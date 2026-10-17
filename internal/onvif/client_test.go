package onvif

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestClientFullLifecycleWithMockServer(t *testing.T) {
	var pullCount int

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body := r.Header.Get("Content-Type")
		_ = body

		action := r.Header.Get("SOAPAction")
		if action == "" {
			// Extract from Content-Type if present
			ct := r.Header.Get("Content-Type")
			if idx := strings.Index(ct, `action="`); idx != -1 {
				action = ct[idx+8:]
				action = strings.TrimSuffix(action, `"`)
			}
		}

		w.Header().Set("Content-Type", "application/soap+xml; charset=utf-8")

		switch {
		case strings.Contains(action, "GetSystemDateAndTime"):
			w.WriteHeader(http.StatusOK)
			_, _ = fmt.Fprint(w, `<?xml version="1.0" encoding="utf-8"?>
<s:Envelope xmlns:s="http://www.w3.org/2003/05/soap-envelope" xmlns:tds="http://www.onvif.org/ver10/device/wsdl">
  <s:Body>
    <tds:GetSystemDateAndTimeResponse>
      <tds:SystemDateAndTime>
        <DateTimeType>NTP</DateTimeType>
        <DaylightSavings>false</DaylightSavings>
        <UTCDateTime>
          <Time><Hour>12</Hour><Minute>0</Minute><Second>0</Second></Time>
          <Date><Year>2026</Year><Month>10</Month><Day>17</Day></Date>
        </UTCDateTime>
      </tds:SystemDateAndTime>
    </tds:GetSystemDateAndTimeResponse>
  </s:Body>
</s:Envelope>`)

		case strings.Contains(action, "GetDeviceInformation"):
			w.WriteHeader(http.StatusOK)
			_, _ = fmt.Fprint(w, `<?xml version="1.0" encoding="utf-8"?>
<s:Envelope xmlns:s="http://www.w3.org/2003/05/soap-envelope" xmlns:tds="http://www.onvif.org/ver10/device/wsdl">
  <s:Body>
    <tds:GetDeviceInformationResponse>
      <tds:Manufacturer>Acme Corp</tds:Manufacturer>
      <tds:Model>LensCam-4K</tds:Model>
      <tds:FirmwareVersion>v2.4.1</tds:FirmwareVersion>
      <tds:SerialNumber>ACME-987654</tds:SerialNumber>
      <tds:HardwareId>HW-001</tds:HardwareId>
    </tds:GetDeviceInformationResponse>
  </s:Body>
</s:Envelope>`)

		case strings.Contains(action, "GetCapabilities"):
			w.WriteHeader(http.StatusOK)
			_, _ = fmt.Fprint(w, `<?xml version="1.0" encoding="utf-8"?>
<s:Envelope xmlns:s="http://www.w3.org/2003/05/soap-envelope" xmlns:tds="http://www.onvif.org/ver10/device/wsdl">
  <s:Body>
    <tds:GetCapabilitiesResponse>
      <tds:Capabilities>
        <tds:Events>
          <tds:XAddr>`+r.Host+`/onvif/events</tds:XAddr>
        </tds:Events>
      </tds:Capabilities>
    </tds:GetCapabilitiesResponse>
  </s:Body>
</s:Envelope>`)

		case strings.Contains(action, "CreatePullPointSubscriptionRequest"):
			w.WriteHeader(http.StatusOK)
			_, _ = fmt.Fprintf(w, `<?xml version="1.0" encoding="utf-8"?>
<s:Envelope xmlns:s="http://www.w3.org/2003/05/soap-envelope" xmlns:tev="http://www.onvif.org/ver10/events/wsdl" xmlns:wsa="http://www.w3.org/2005/08/addressing">
  <s:Body>
    <tev:CreatePullPointSubscriptionResponse>
      <tev:SubscriptionReference>
        <wsa:Address>http://%s/onvif/subscription/123</wsa:Address>
      </tev:SubscriptionReference>
      <tev:CurrentTime>2026-10-17T12:00:00Z</tev:CurrentTime>
      <tev:TerminationTime>2026-10-17T12:01:00Z</tev:TerminationTime>
    </tev:CreatePullPointSubscriptionResponse>
  </s:Body>
</s:Envelope>`, r.Host)

		case strings.Contains(action, "PullMessagesRequest"):
			pullCount++
			w.WriteHeader(http.StatusOK)
			_, _ = fmt.Fprint(w, `<?xml version="1.0" encoding="utf-8"?>
<s:Envelope xmlns:s="http://www.w3.org/2003/05/soap-envelope" xmlns:tev="http://www.onvif.org/ver10/events/wsdl">
  <s:Body>
    <tev:PullMessagesResponse>
      <tev:NotificationMessage>
        <tev:Topic Dialect="http://www.onvif.org/ver10/tev/topicExpression/ConcreteSet">tns1:RuleEngine/CellMotionDetector/Motion</tev:Topic>
        <tev:Message UtcTime="2026-10-17T12:00:05Z">
          <tev:Data>
            <tev:SimpleItem Name="IsInside" Value="true"/>
            <tev:SimpleItem Name="State" Value="true"/>
          </tev:Data>
        </tev:Message>
      </tev:NotificationMessage>
    </tev:PullMessagesResponse>
  </s:Body>
</s:Envelope>`)

		case strings.Contains(action, "RenewRequest"):
			w.WriteHeader(http.StatusOK)
			_, _ = fmt.Fprint(w, `<?xml version="1.0" encoding="utf-8"?>
<s:Envelope xmlns:s="http://www.w3.org/2003/05/soap-envelope" xmlns:wsnt="http://docs.oasis-open.org/wsn/b-2">
  <s:Body>
    <wsnt:RenewResponse>
      <wsnt:TerminationTime>2026-10-17T12:02:00Z</wsnt:TerminationTime>
    </wsnt:RenewResponse>
  </s:Body>
</s:Envelope>`)

		case strings.Contains(action, "UnsubscribeRequest"):
			w.WriteHeader(http.StatusOK)
			_, _ = fmt.Fprint(w, `<?xml version="1.0" encoding="utf-8"?>
<s:Envelope xmlns:s="http://www.w3.org/2003/05/soap-envelope" xmlns:wsnt="http://docs.oasis-open.org/wsn/b-2">
  <s:Body>
    <wsnt:UnsubscribeResponse/>
  </s:Body>
</s:Envelope>`)

		default:
			w.WriteHeader(http.StatusNotFound)
			_, _ = fmt.Fprintf(w, "Unknown action: %s", action)
		}
	}))
	defer ts.Close()

	client, err := NewClient(ClientOptions{
		DeviceURL: ts.URL + "/onvif/device_service",
		Credentials: SecurityCredentials{
			Username: "admin",
			Password: "pass",
		},
		HTTPTimeout: 5 * time.Second,
	})
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	ctx := context.Background()

	// 1. Sync clock
	drift, err := client.SyncSystemDateAndTime(ctx)
	if err != nil {
		t.Fatalf("SyncSystemDateAndTime failed: %v", err)
	}
	_ = drift
	if !client.Clock.IsSynced() {
		t.Error("expected clock to be synced")
	}

	// 2. Get device info
	devInfo, err := client.GetDeviceInformation(ctx)
	if err != nil {
		t.Fatalf("GetDeviceInformation failed: %v", err)
	}
	if devInfo.Manufacturer != "Acme Corp" || devInfo.Model != "LensCam-4K" {
		t.Errorf("unexpected devInfo: %+v", devInfo)
	}

	// 3. Get capabilities
	caps, err := client.GetCapabilities(ctx)
	if err != nil {
		t.Fatalf("GetCapabilities failed: %v", err)
	}
	if !strings.Contains(caps.Capabilities.Events.XAddr, "/onvif/events") {
		t.Errorf("unexpected events xaddr: %s", caps.Capabilities.Events.XAddr)
	}

	// 4. Create PullPoint
	sub, err := client.CreatePullPointSubscription(ctx, "PT60S")
	if err != nil {
		t.Fatalf("CreatePullPointSubscription failed: %v", err)
	}
	if !strings.Contains(sub.SubscriptionURL, "/onvif/subscription/123") {
		t.Errorf("unexpected subscription URL: %s", sub.SubscriptionURL)
	}

	// 5. Pull messages
	pullRes, err := client.PullMessages(ctx, sub.SubscriptionURL, "PT10S", 10)
	if err != nil {
		t.Fatalf("PullMessages failed: %v", err)
	}
	if len(pullRes.Messages) != 1 {
		t.Fatalf("expected 1 message, got %d", len(pullRes.Messages))
	}
	msg := pullRes.Messages[0]
	if !strings.Contains(msg.Topic.Value, "CellMotionDetector") {
		t.Errorf("unexpected topic: %s", msg.Topic.Value)
	}

	// 6. Renew
	if err := client.Renew(ctx, sub.SubscriptionURL, "PT60S"); err != nil {
		t.Fatalf("Renew failed: %v", err)
	}

	// 7. Unsubscribe
	if err := client.Unsubscribe(ctx, sub.SubscriptionURL); err != nil {
		t.Fatalf("Unsubscribe failed: %v", err)
	}
}

func TestSanitizeURL(t *testing.T) {
	c := &Client{
		TargetHost: "192.168.1.113:2020",
	}

	tests := []struct {
		input    string
		expected string
	}{
		// Dynamic event port 1024 returned by camera should be preserved on target IP
		{"http://192.168.1.113:1024/event-1024_1024", "http://192.168.1.113:1024/event-1024_1024"},
		// Private or loopback IP with default HTTP port 80 rewritten to target IP and port 2020
		{"http://10.0.0.5:80/onvif/service", "http://192.168.1.113:2020/onvif/service"},
		{"http://10.0.0.5/onvif/service", "http://192.168.1.113:2020/onvif/service"},
		// Relative path
		{"/onvif/service", "http://192.168.1.113:2020/onvif/service"},
		// Non-standard port on different host should preserve the port but rewrite host
		{"http://10.0.0.5:1025/event-1025", "http://192.168.1.113:1025/event-1025"},
	}

	for _, tt := range tests {
		got := c.SanitizeURL(tt.input)
		if got != tt.expected {
			t.Errorf("SanitizeURL(%q) = %q, expected %q", tt.input, got, tt.expected)
		}
	}
}
