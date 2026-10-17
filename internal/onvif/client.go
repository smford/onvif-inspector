package onvif

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/xml"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// Client represents an ONVIF SOAP client for a specific camera
type Client struct {
	DeviceURL   string
	TargetHost  string // Original host:port used to connect (for URL rewriting/sanitization)
	Credentials SecurityCredentials
	Clock       *ClockManager
	HTTPClient  *http.Client
	Debug       bool
	DebugLogger *log.Logger

	mu          sync.RWMutex
	EventsXAddr string
	DeviceInfo  *DeviceInformationResponse
}

// ClientOptions configures client creation
type ClientOptions struct {
	DeviceURL   string
	Credentials SecurityCredentials
	HTTPTimeout time.Duration
	Debug       bool
	DebugLogger *log.Logger
}

// NewClient initializes a new ONVIF client
func NewClient(opts ClientOptions) (*Client, error) {
	if opts.DeviceURL == "" {
		return nil, fmt.Errorf("device URL is required")
	}

	parsed, err := url.Parse(opts.DeviceURL)
	if err != nil {
		return nil, fmt.Errorf("invalid device URL %q: %w", opts.DeviceURL, err)
	}

	timeout := opts.HTTPTimeout
	if timeout == 0 {
		timeout = 15 * time.Second
	}

	httpClient := &http.Client{
		Timeout: timeout,
		Transport: &http.Transport{
			Proxy: http.ProxyFromEnvironment,
			DialContext: (&net.Dialer{
				Timeout:   10 * time.Second,
				KeepAlive: 30 * time.Second,
			}).DialContext,
			MaxIdleConns:        10,
			IdleConnTimeout:     60 * time.Second,
			TLSHandshakeTimeout: 10 * time.Second,
		},
	}

	logger := opts.DebugLogger
	if logger == nil && opts.Debug {
		logger = log.Default()
	}

	return &Client{
		DeviceURL:   opts.DeviceURL,
		TargetHost:  parsed.Host,
		Credentials: opts.Credentials,
		Clock:       NewClockManager(),
		HTTPClient:  httpClient,
		Debug:       opts.Debug,
		DebugLogger: logger,
	}, nil
}

// GenerateUUID generates a random urn:uuid string
func GenerateUUID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	// Set version 4 and variant RFC4122
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("urn:uuid:%s-%s-%s-%s-%s",
		hex.EncodeToString(b[0:4]),
		hex.EncodeToString(b[4:6]),
		hex.EncodeToString(b[6:8]),
		hex.EncodeToString(b[8:10]),
		hex.EncodeToString(b[10:16]))
}

// SanitizeURL adjusts a URL returned by a camera to use the reachable host/port of the camera.
// Many camera firmwares report internal/private IPs (e.g. 192.168.1.1:80) even when queried
// from a different subnet or forwarded port.
func (c *Client) SanitizeURL(rawURL string) string {
	if rawURL == "" {
		return rawURL
	}

	cleanURL := rawURL
	if !strings.HasPrefix(cleanURL, "http://") && !strings.HasPrefix(cleanURL, "https://") {
		if strings.HasPrefix(cleanURL, "//") {
			cleanURL = "http:" + cleanURL
		} else if strings.HasPrefix(cleanURL, "/") {
			if c.TargetHost != "" {
				cleanURL = "http://" + c.TargetHost + cleanURL
			} else {
				cleanURL = "http://localhost" + cleanURL
			}
		} else {
			cleanURL = "http://" + cleanURL
		}
	}

	u, err := url.Parse(cleanURL)
	if err != nil {
		return rawURL
	}

	if c.TargetHost == "" {
		return u.String()
	}

	// If scheme is empty or host is empty/loopback/mismatched, harmonize host:port with TargetHost
	if u.Host == "" || u.Host != c.TargetHost {
		targetHostOnly, targetPort, _ := net.SplitHostPort(c.TargetHost)
		if targetHostOnly == "" {
			targetHostOnly = c.TargetHost
		}

		reportedHostOnly, reportedPort, _ := net.SplitHostPort(u.Host)
		if reportedHostOnly == "" {
			reportedHostOnly = u.Host
		}

		// Rewrite host to targetHostOnly
		newHost := targetHostOnly
		if reportedPort != "" && reportedPort != "80" && reportedPort != "443" {
			newHost = net.JoinHostPort(targetHostOnly, reportedPort)
		} else if targetPort != "" {
			newHost = net.JoinHostPort(targetHostOnly, targetPort)
		}

		u.Host = newHost
	}

	if u.Scheme == "" {
		u.Scheme = "http"
	}

	return u.String()
}

// RoundTripResult captures SOAP request and response details for diagnostics
type RoundTripResult struct {
	StatusCode   int
	RawRequest   string
	RawResponse  string
	Duration     time.Duration
	FaultCode    string
	FaultReason  string
	ResponseBody []byte
}

// SendSOAP sends a SOAP 1.2 request to endpointURL with optional action and body XML.
func (c *Client) SendSOAP(ctx context.Context, endpointURL string, action string, bodyXML string, withAuth bool) (*RoundTripResult, error) {
	msgID := GenerateUUID()
	var secHeader *SecurityHeader
	var err error

	if withAuth && c.Credentials.Username != "" {
		secHeader, err = GenerateSecurityHeader(c.Credentials, c.Clock.Now())
		if err != nil {
			return nil, fmt.Errorf("failed to generate security header: %w", err)
		}
	}

	var secXML strings.Builder
	if secHeader != nil {
		token := secHeader.UsernameToken
		secXML.WriteString(fmt.Sprintf(`
    <wsse:Security xmlns:wsse="%s" xmlns:wsu="%s" s:mustUnderstand="1">
      <wsse:UsernameToken>
        <wsse:Username>%s</wsse:Username>
        <wsse:Password Type="%s">%s</wsse:Password>
        <wsse:Nonce EncodingType="%s">%s</wsse:Nonce>
        <wsu:Created>%s</wsu:Created>
      </wsse:UsernameToken>
    </wsse:Security>`,
			NSWSSecurity, NSWSUtility,
			xmlEscape(token.Username),
			xmlEscape(token.Password.Type), xmlEscape(token.Password.Value),
			xmlEscape(token.Nonce.EncodingType), xmlEscape(token.Nonce.Value),
			xmlEscape(token.Created)))
	}

	var wsaHeaders strings.Builder
	if action != "" {
		wsaHeaders.WriteString(fmt.Sprintf("\n    <wsa:Action>%s</wsa:Action>", xmlEscape(action)))
	}
	wsaHeaders.WriteString(fmt.Sprintf("\n    <wsa:MessageID>%s</wsa:MessageID>", xmlEscape(msgID)))
	if endpointURL != "" {
		wsaHeaders.WriteString(fmt.Sprintf("\n    <wsa:To>%s</wsa:To>", xmlEscape(endpointURL)))
	}

	soapReq := fmt.Sprintf(`<?xml version="1.0" encoding="utf-8"?>
<s:Envelope xmlns:s="%s"
            xmlns:wsa="%s"
            xmlns:wsse="%s"
            xmlns:wsu="%s">
  <s:Header>%s%s
  </s:Header>
  <s:Body>
%s
  </s:Body>
</s:Envelope>`,
		SOAPEnv12, NSWSAddressing, NSWSSecurity, NSWSUtility,
		secXML.String(), wsaHeaders.String(),
		bodyXML)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpointURL, strings.NewReader(soapReq))
	if err != nil {
		return nil, fmt.Errorf("failed to create http request: %w", err)
	}

	// SOAP 1.2 content type
	contentType := fmt.Sprintf("application/soap+xml; charset=utf-8; action=\"%s\"", action)
	req.Header.Set("Content-Type", contentType)
	req.Header.Set("User-Agent", "onvif-inspector/1.0")

	startTime := time.Now()
	resp, err := c.HTTPClient.Do(req)
	duration := time.Since(startTime)

	result := &RoundTripResult{
		RawRequest: soapReq,
		Duration:   duration,
	}

	if err != nil {
		if c.Debug && c.DebugLogger != nil {
			c.DebugLogger.Printf("[SOAP ERROR] %s -> %v\nRequest:\n%s\n", endpointURL, err, soapReq)
		}
		return result, fmt.Errorf("http request error: %w", err)
	}
	defer resp.Body.Close()

	result.StatusCode = resp.StatusCode
	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return result, fmt.Errorf("failed reading response body: %w", err)
	}
	result.RawResponse = string(respBytes)
	result.ResponseBody = respBytes

	if c.Debug && c.DebugLogger != nil {
		c.DebugLogger.Printf("[SOAP] %s %d (%v)\nRequest:\n%s\nResponse:\n%s\n",
			endpointURL, resp.StatusCode, duration, soapReq, string(respBytes))
	}

	// Check for SOAP fault
	var faultCheck struct {
		XMLName xml.Name `xml:"Envelope"`
		Body    struct {
			Fault *struct {
				Code struct {
					Value   string `xml:"Value"`
					Subcode *struct {
						Value string `xml:"Value"`
					} `xml:"Subcode"`
				} `xml:"Code"`
				Reason struct {
					Text string `xml:"Text"`
				} `xml:"Reason"`
				// SOAP 1.1 fallback
				FaultCode   string `xml:"faultcode"`
				FaultString string `xml:"faultstring"`
			} `xml:"Fault"`
		} `xml:"Body"`
	}

	if unmarshalErr := xml.Unmarshal(respBytes, &faultCheck); unmarshalErr == nil && faultCheck.Body.Fault != nil {
		fault := faultCheck.Body.Fault
		code := fault.Code.Value
		if fault.Code.Subcode != nil {
			code = code + "/" + fault.Code.Subcode.Value
		}
		if code == "" {
			code = fault.FaultCode
		}
		reason := fault.Reason.Text
		if reason == "" {
			reason = fault.FaultString
		}
		result.FaultCode = code
		result.FaultReason = reason
		return result, fmt.Errorf("SOAP fault [%s]: %s", code, reason)
	}

	if resp.StatusCode >= 400 {
		return result, fmt.Errorf("HTTP error %d: %s", resp.StatusCode, resp.Status)
	}

	return result, nil
}

// SyncSystemDateAndTime calls GetSystemDateAndTime without authentication to synchronize clock drift
func (c *Client) SyncSystemDateAndTime(ctx context.Context) (time.Duration, error) {
	body := `<tds:GetSystemDateAndTime xmlns:tds="http://www.onvif.org/ver10/device/wsdl"/>`
	action := "http://www.onvif.org/ver10/device/wsdl/GetSystemDateAndTime"

	startLocal := time.Now()
	res, err := c.SendSOAP(ctx, c.DeviceURL, action, body, false)
	if err != nil {
		return 0, fmt.Errorf("GetSystemDateAndTime failed: %w", err)
	}
	endLocal := time.Now()
	estLocal := startLocal.Add(endLocal.Sub(startLocal) / 2)

	camTime, err := ParseSystemDateAndTimeResponse(res.ResponseBody)
	if err != nil {
		return 0, fmt.Errorf("failed to parse system date/time: %w", err)
	}

	drift := camTime.Sub(estLocal)
	c.Clock.SetDrift(drift)
	return drift, nil
}

// GetDeviceInformation queries the device metadata
func (c *Client) GetDeviceInformation(ctx context.Context) (*DeviceInformationResponse, error) {
	body := `<tds:GetDeviceInformation xmlns:tds="http://www.onvif.org/ver10/device/wsdl"/>`
	action := "http://www.onvif.org/ver10/device/wsdl/GetDeviceInformation"

	res, err := c.SendSOAP(ctx, c.DeviceURL, action, body, true)
	if err != nil {
		return nil, fmt.Errorf("GetDeviceInformation failed: %w", err)
	}

	var env DeviceInformationEnvelope
	if err := xml.Unmarshal(res.ResponseBody, &env); err != nil {
		return nil, fmt.Errorf("failed to parse GetDeviceInformation response: %w", err)
	}

	devInfo := env.Body.Response
	c.mu.Lock()
	c.DeviceInfo = &devInfo
	c.mu.Unlock()

	return &devInfo, nil
}

// GetCapabilities resolves service endpoints, notably the Events XAddr
func (c *Client) GetCapabilities(ctx context.Context) (*CapabilitiesResponse, error) {
	body := `<tds:GetCapabilities xmlns:tds="http://www.onvif.org/ver10/device/wsdl">
    <tds:Category>All</tds:Category>
  </tds:GetCapabilities>`
	action := "http://www.onvif.org/ver10/device/wsdl/GetCapabilities"

	res, err := c.SendSOAP(ctx, c.DeviceURL, action, body, true)
	if err != nil {
		return nil, fmt.Errorf("GetCapabilities failed: %w", err)
	}

	var env CapabilitiesEnvelope
	if err := xml.Unmarshal(res.ResponseBody, &env); err != nil {
		return nil, fmt.Errorf("failed to parse GetCapabilities response: %w", err)
	}

	caps := env.Body.Response
	rawEventsXAddr := caps.Capabilities.Events.XAddr
	sanitizedEventsXAddr := c.SanitizeURL(rawEventsXAddr)

	c.mu.Lock()
	c.EventsXAddr = sanitizedEventsXAddr
	c.mu.Unlock()

	return &caps, nil
}

// GetCachedEventsXAddr returns the resolved Events XAddr or falls back to common paths
func (c *Client) GetCachedEventsXAddr() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.EventsXAddr != "" {
		return c.EventsXAddr
	}
	// Fallback guess: replace /onvif/device_service with /onvif/event_service or /onvif/Events
	if strings.Contains(c.DeviceURL, "device_service") {
		return strings.Replace(c.DeviceURL, "device_service", "event_service", 1)
	}
	return c.DeviceURL
}

func xmlEscape(s string) string {
	var b bytes.Buffer
	_ = xml.EscapeText(&b, []byte(s))
	return b.String()
}
