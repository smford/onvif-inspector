package discovery

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestParseProbeMatch(t *testing.T) {
	sample := `<?xml version="1.0" encoding="UTF-8"?>
<SOAP-ENV:Envelope xmlns:SOAP-ENV="http://www.w3.org/2003/05/soap-envelope"
                   xmlns:wsa="http://schemas.xmlsoap.org/ws/2004/08/addressing"
                   xmlns:d="http://schemas.xmlsoap.org/ws/2005/04/discovery"
                   xmlns:dn="http://www.onvif.org/ver10/network/wsdl">
  <SOAP-ENV:Header>
    <wsa:Action>http://schemas.xmlsoap.org/ws/2005/04/discovery/ProbeMatches</wsa:Action>
    <wsa:RelatesTo>urn:uuid:12345</wsa:RelatesTo>
  </SOAP-ENV:Header>
  <SOAP-ENV:Body>
    <d:ProbeMatches>
      <d:ProbeMatch>
        <wsa:EndpointReference>
          <wsa:Address>urn:uuid:52044813-0982-41f2-8d77-a89a01c349a1</wsa:Address>
        </wsa:EndpointReference>
        <d:Types>dn:NetworkVideoTransmitter tds:Device</d:Types>
        <d:Scopes>
          onvif://www.onvif.org/type/video_encoder
          onvif://www.onvif.org/type/audio_encoder
          onvif://www.onvif.org/hardware/LensCam-4K
          onvif://www.onvif.org/name/FrontDoorCamera
          onvif://www.onvif.org/location/Entrance
        </d:Scopes>
        <d:XAddrs>http://192.168.1.120:8080/onvif/device_service</d:XAddrs>
        <d:MetadataVersion>1</d:MetadataVersion>
      </d:ProbeMatch>
    </d:ProbeMatches>
  </SOAP-ENV:Body>
</SOAP-ENV:Envelope>`

	devices, err := ParseProbeMatch([]byte(sample))
	if err != nil {
		t.Fatalf("ParseProbeMatch failed: %v", err)
	}

	if len(devices) != 1 {
		t.Fatalf("expected 1 device, got %d", len(devices))
	}

	d := devices[0]
	if d.Endpoint != "urn:uuid:52044813-0982-41f2-8d77-a89a01c349a1" {
		t.Errorf("unexpected endpoint: %s", d.Endpoint)
	}
	if d.IP != "192.168.1.120" {
		t.Errorf("expected IP 192.168.1.120, got %s", d.IP)
	}
	if d.Port != 8080 {
		t.Errorf("expected port 8080, got %d", d.Port)
	}
	if d.PrimaryXAddr != "http://192.168.1.120:8080/onvif/device_service" {
		t.Errorf("unexpected primary xaddr: %s", d.PrimaryXAddr)
	}
	if d.Name != "FrontDoorCamera" {
		t.Errorf("expected Name 'FrontDoorCamera', got %s", d.Name)
	}
	if d.Hardware != "LensCam-4K" {
		t.Errorf("expected Hardware 'LensCam-4K', got %s", d.Hardware)
	}
	if d.Location != "Entrance" {
		t.Errorf("expected Location 'Entrance', got %s", d.Location)
	}
}

func TestBuildProbeMessage(t *testing.T) {
	data, msgID := BuildProbeMessage()
	str := string(data)

	if !strings.Contains(str, ActionProbe) {
		t.Errorf("probe missing action %s", ActionProbe)
	}
	if !strings.Contains(str, msgID) {
		t.Errorf("probe missing messageID %s", msgID)
	}
	if !strings.Contains(str, "NetworkVideoTransmitter") {
		t.Errorf("probe missing NetworkVideoTransmitter type")
	}
}

func TestDiscoverTarget(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	devices, err := Discover(ctx, ProbeOptions{
		Target: "192.168.1.50:8000",
	})
	if err != nil {
		t.Fatalf("Discover target failed: %v", err)
	}
	if len(devices) != 1 {
		t.Fatalf("expected 1 device, got %d", len(devices))
	}
	d := devices[0]
	if d.IP != "192.168.1.50" || d.Port != 8000 {
		t.Errorf("unexpected target ip/port: %s:%d", d.IP, d.Port)
	}
	if d.PrimaryXAddr != "http://192.168.1.50:8000/onvif/device_service" {
		t.Errorf("unexpected primary xaddr: %s", d.PrimaryXAddr)
	}
}
