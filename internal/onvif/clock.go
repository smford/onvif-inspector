package onvif

import (
	"encoding/xml"
	"fmt"
	"sync"
	"time"
)

// ClockManager manages the clock drift compensation between the local host and an ONVIF device.
type ClockManager struct {
	mu     sync.RWMutex
	drift  time.Duration
	synced bool
}

// NewClockManager creates a new un-synchronized ClockManager
func NewClockManager() *ClockManager {
	return &ClockManager{}
}

// SetDrift sets the known clock drift (cameraTime - localTime)
func (cm *ClockManager) SetDrift(d time.Duration) {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	cm.drift = d
	cm.synced = true
}

// GetDrift returns current recorded drift
func (cm *ClockManager) GetDrift() time.Duration {
	cm.mu.RLock()
	defer cm.mu.RUnlock()
	return cm.drift
}

// IsSynced returns whether a drift calculation has succeeded
func (cm *ClockManager) IsSynced() bool {
	cm.mu.RLock()
	defer cm.mu.RUnlock()
	return cm.synced
}

// Now returns the current time adjusted by the clock drift
func (cm *ClockManager) Now() time.Time {
	cm.mu.RLock()
	d := cm.drift
	cm.mu.RUnlock()
	return time.Now().UTC().Add(d)
}

// ParseSystemDateAndTimeResponse parses the XML response from GetSystemDateAndTime
// and returns the camera's system time in UTC.
func ParseSystemDateAndTimeResponse(xmlData []byte) (time.Time, error) {
	var resp struct {
		XMLName xml.Name `xml:"Envelope"`
		Body    struct {
			Fault *struct {
				Code   struct{ Value string } `xml:"Code"`
				Reason struct{ Text string }  `xml:"Reason"`
			} `xml:"Fault"`
			GetSystemDateAndTimeResponse struct {
				SystemDateAndTime struct {
					DateTimeType    string `xml:"DateTimeType"`
					DaylightSavings bool   `xml:"DaylightSavings"`
					TimeZone        struct {
						TZ string `xml:"TZ"`
					} `xml:"TimeZone"`
					UTCDateTime struct {
						Time struct {
							Hour   int `xml:"Hour"`
							Minute int `xml:"Minute"`
							Second int `xml:"Second"`
						} `xml:"Time"`
						Date struct {
							Year  int `xml:"Year"`
							Month int `xml:"Month"`
							Day   int `xml:"Day"`
						} `xml:"Date"`
					} `xml:"UTCDateTime"`
					LocalDateTime struct {
						Time struct {
							Hour   int `xml:"Hour"`
							Minute int `xml:"Minute"`
							Second int `xml:"Second"`
						} `xml:"Time"`
						Date struct {
							Year  int `xml:"Year"`
							Month int `xml:"Month"`
							Day   int `xml:"Day"`
						} `xml:"Date"`
					} `xml:"LocalDateTime"`
				} `xml:"SystemDateAndTime"`
			} `xml:"GetSystemDateAndTimeResponse"`
		} `xml:"Body"`
	}

	if err := xml.Unmarshal(xmlData, &resp); err != nil {
		return time.Time{}, fmt.Errorf("failed to parse GetSystemDateAndTime response: %w", err)
	}

	if resp.Body.Fault != nil {
		return time.Time{}, fmt.Errorf("SOAP fault: %s - %s", resp.Body.Fault.Code.Value, resp.Body.Fault.Reason.Text)
	}

	sdt := resp.Body.GetSystemDateAndTimeResponse.SystemDateAndTime
	utcDate := sdt.UTCDateTime.Date
	utcTime := sdt.UTCDateTime.Time

	if utcDate.Year > 1970 && utcDate.Month >= 1 && utcDate.Month <= 12 && utcDate.Day >= 1 && utcDate.Day <= 31 {
		return time.Date(utcDate.Year, time.Month(utcDate.Month), utcDate.Day,
			utcTime.Hour, utcTime.Minute, utcTime.Second, 0, time.UTC), nil
	}

	// Fallback to LocalDateTime if UTCDateTime is missing/invalid
	locDate := sdt.LocalDateTime.Date
	locTime := sdt.LocalDateTime.Time
	if locDate.Year > 1970 && locDate.Month >= 1 && locDate.Month <= 12 && locDate.Day >= 1 && locDate.Day <= 31 {
		return time.Date(locDate.Year, time.Month(locDate.Month), locDate.Day,
			locTime.Hour, locTime.Minute, locTime.Second, 0, time.UTC), nil
	}

	return time.Time{}, fmt.Errorf("response contained invalid date/time values")
}

// BuildGetSystemDateAndTimeRequest constructs a SOAP envelope to query camera system date and time (no auth needed).
func BuildGetSystemDateAndTimeRequest() string {
	return `<?xml version="1.0" encoding="utf-8"?>
<s:Envelope xmlns:s="http://www.w3.org/2003/05/soap-envelope"
            xmlns:tds="http://www.onvif.org/ver10/device/wsdl">
  <s:Body>
    <tds:GetSystemDateAndTime/>
  </s:Body>
</s:Envelope>`
}
