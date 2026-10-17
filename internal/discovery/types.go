package discovery

import (
	"encoding/xml"
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// DiscoveredDevice represents an ONVIF device found via WS-Discovery or unicast target
type DiscoveredDevice struct {
	Endpoint        string        `json:"endpoint"`
	XAddrs          []string      `json:"xaddrs"`
	PrimaryXAddr    string        `json:"primary_xaddr"`
	IP              string        `json:"ip"`
	Port            int           `json:"port"`
	Types           []string      `json:"types"`
	Scopes          []string      `json:"scopes"`
	MetadataVersion int           `json:"metadata_version"`
	DiscoveredAt    time.Time     `json:"discovered_at"`
	PingLatency     time.Duration `json:"ping_latency_ms"`

	// Extracted Scope Metadata
	Name         string `json:"name,omitempty"`
	Hardware     string `json:"hardware,omitempty"`
	Location     string `json:"location,omitempty"`
	Manufacturer string `json:"manufacturer,omitempty"`
	Model        string `json:"model,omitempty"`

	RawProbeMatch string `json:"raw_probe_match,omitempty"`
}

// ProbeOptions controls the WS-Discovery scanning behavior
type ProbeOptions struct {
	InterfaceName string
	Target        string
	Timeout       time.Duration
}

// WS-Discovery XML structures
type probeEnvelope struct {
	XMLName xml.Name `xml:"http://www.w3.org/2003/05/soap-envelope Envelope"`
	Header  struct {
		Action    string `xml:"http://schemas.xmlsoap.org/ws/2004/08/addressing Action"`
		MessageID string `xml:"http://schemas.xmlsoap.org/ws/2004/08/addressing MessageID"`
		To        string `xml:"http://schemas.xmlsoap.org/ws/2004/08/addressing To"`
	} `xml:"Header"`
	Body struct {
		Probe struct {
			XMLName xml.Name `xml:"http://schemas.xmlsoap.org/ws/2005/04/discovery Probe"`
			Types   string   `xml:"Types,omitempty"`
		} `xml:"Probe"`
	} `xml:"Body"`
}

type probeMatchesEnvelope struct {
	XMLName xml.Name `xml:"Envelope"`
	Body    struct {
		ProbeMatches struct {
			ProbeMatch []struct {
				EndpointReference struct {
					Address string `xml:"Address"`
				} `xml:"EndpointReference"`
				Types           string `xml:"Types"`
				Scopes          string `xml:"Scopes"`
				XAddrs          string `xml:"XAddrs"`
				MetadataVersion int    `xml:"MetadataVersion"`
			} `xml:"ProbeMatch"`
		} `xml:"ProbeMatches"`
	} `xml:"Body"`
}

// ParseProbeMatch parses a WS-Discovery SOAP response envelope and extracts discovered devices
func ParseProbeMatch(data []byte) ([]DiscoveredDevice, error) {
	var env probeMatchesEnvelope
	if err := xml.Unmarshal(data, &env); err != nil {
		return nil, fmt.Errorf("failed to parse ProbeMatches XML: %w", err)
	}

	matches := env.Body.ProbeMatches.ProbeMatch
	if len(matches) == 0 {
		return nil, nil
	}

	var devices []DiscoveredDevice
	for _, m := range matches {
		xaddrs := strings.Fields(m.XAddrs)
		types := strings.Fields(m.Types)
		scopes := strings.Fields(m.Scopes)

		primaryXAddr := ""
		ip := ""
		port := 80

		if len(xaddrs) > 0 {
			primaryXAddr = xaddrs[0]
			if u, err := url.Parse(primaryXAddr); err == nil {
				host, pStr, err := net.SplitHostPort(u.Host)
				if err == nil {
					ip = host
					if p, err := strconv.Atoi(pStr); err == nil {
						port = p
					}
				} else {
					ip = u.Host
					if u.Scheme == "https" {
						port = 443
					} else {
						port = 80
					}
				}
			}
		}

		dev := DiscoveredDevice{
			Endpoint:        strings.TrimSpace(m.EndpointReference.Address),
			XAddrs:          xaddrs,
			PrimaryXAddr:    primaryXAddr,
			IP:              ip,
			Port:            port,
			Types:           types,
			Scopes:          scopes,
			MetadataVersion: m.MetadataVersion,
			DiscoveredAt:    time.Now().UTC(),
			RawProbeMatch:   string(data),
		}

		// Extract metadata from scopes
		// Common ONVIF scopes:
		// onvif://www.onvif.org/name/CameraName
		// onvif://www.onvif.org/hardware/ModelName
		// onvif://www.onvif.org/location/LocationName
		// onvif://www.onvif.org/type/NetworkVideoTransmitter
		for _, s := range scopes {
			decoded, err := url.QueryUnescape(s)
			if err != nil {
				decoded = s
			}
			lower := strings.ToLower(decoded)
			switch {
			case strings.Contains(lower, "/name/"):
				dev.Name = extractScopeValue(decoded, "/name/")
			case strings.Contains(lower, "/hardware/"):
				dev.Hardware = extractScopeValue(decoded, "/hardware/")
				dev.Model = dev.Hardware
			case strings.Contains(lower, "/location/"):
				dev.Location = extractScopeValue(decoded, "/location/")
			case strings.Contains(lower, "/manufacturer/"):
				dev.Manufacturer = extractScopeValue(decoded, "/manufacturer/")
			}
		}

		if dev.Name == "" && dev.Model != "" {
			dev.Name = dev.Model
		}

		devices = append(devices, dev)
	}

	return devices, nil
}

func extractScopeValue(scope string, delimiter string) string {
	idx := strings.Index(strings.ToLower(scope), delimiter)
	if idx != -1 {
		val := scope[idx+len(delimiter):]
		// trim any trailing slash or query
		if slashIdx := strings.Index(val, "/"); slashIdx != -1 {
			val = val[:slashIdx]
		}
		return strings.TrimSpace(val)
	}
	return ""
}
