package onvif

import (
	"testing"
	"time"
)

func TestParseSystemDateAndTimeResponse(t *testing.T) {
	xmlSample := `<?xml version="1.0" encoding="UTF-8"?>
<SOAP-ENV:Envelope xmlns:SOAP-ENV="http://www.w3.org/2003/05/soap-envelope"
                   xmlns:tds="http://www.onvif.org/ver10/device/wsdl"
                   xmlns:tt="http://www.onvif.org/ver10/schema">
  <SOAP-ENV:Header/>
  <SOAP-ENV:Body>
    <tds:GetSystemDateAndTimeResponse>
      <tds:SystemDateAndTime>
        <tt:DateTimeType>NTP</tt:DateTimeType>
        <tt:DaylightSavings>false</tt:DaylightSavings>
        <tt:TimeZone>
          <tt:TZ>GMT</tt:TZ>
        </tt:TimeZone>
        <tt:UTCDateTime>
          <tt:Time>
            <tt:Hour>14</tt:Hour>
            <tt:Minute>25</tt:Minute>
            <tt:Second>30</tt:Second>
          </tt:Time>
          <tt:Date>
            <tt:Year>2026</tt:Year>
            <tt:Month>10</tt:Month>
            <tt:Day>17</tt:Day>
          </tt:Date>
        </tt:UTCDateTime>
      </tds:SystemDateAndTime>
    </tds:GetSystemDateAndTimeResponse>
  </SOAP-ENV:Body>
</SOAP-ENV:Envelope>`

	dt, err := ParseSystemDateAndTimeResponse([]byte(xmlSample))
	if err != nil {
		t.Fatalf("ParseSystemDateAndTimeResponse failed: %v", err)
	}

	expected := time.Date(2026, 10, 17, 14, 25, 30, 0, time.UTC)
	if !dt.Equal(expected) {
		t.Errorf("expected %v, got %v", expected, dt)
	}
}

func TestClockManagerDrift(t *testing.T) {
	cm := NewClockManager()
	if cm.IsSynced() {
		t.Error("expected IsSynced false initially")
	}

	drift := 5 * time.Minute
	cm.SetDrift(drift)

	if !cm.IsSynced() {
		t.Error("expected IsSynced true after SetDrift")
	}
	if cm.GetDrift() != drift {
		t.Errorf("expected drift %v, got %v", drift, cm.GetDrift())
	}

	before := time.Now().UTC().Add(drift)
	adjusted := cm.Now()
	diff := adjusted.Sub(before)
	if diff < -time.Second || diff > time.Second {
		t.Errorf("adjusted time %v differs from expected %v", adjusted, before)
	}
}
