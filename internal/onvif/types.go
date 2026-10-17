package onvif

import (
	"encoding/xml"
	"time"
)

// SOAP Namespaces commonly used in ONVIF
const (
	SOAPEnv12 = "http://www.w3.org/2003/05/soap-envelope"
	SOAPEnv11 = "http://schemas.xmlsoap.org/soap/envelope/"

	NSWSAddressing = "http://www.w3.org/2005/08/addressing"
	NSWSSecurity   = "http://docs.oasis-open.org/wss/2004/01/oasis-200401-wss-wssecurity-secext-1.0.xsd"
	NSWSUtility    = "http://docs.oasis-open.org/wss/2004/01/oasis-200401-wss-wssecurity-utility-1.0.xsd"
	NSTopicDialect = "http://www.onvif.org/ver10/tev/topicExpression/ConcreteSet"

	NSTDevice = "http://www.onvif.org/ver10/device/wsdl"
	NSTEvents = "http://www.onvif.org/ver10/events/wsdl"
	NSTSchema = "http://www.onvif.org/ver10/schema"
	NSWSNT    = "http://docs.oasis-open.org/wsn/b-2"
	NSWSNTW   = "http://docs.oasis-open.org/wsn/bw-2"

	PasswordDigestType = "http://docs.oasis-open.org/wss/2004/01/oasis-200401-wss-username-token-profile-1.0#PasswordDigest"
	NonceEncodingType  = "http://docs.oasis-open.org/wss/2004/01/oasis-200401-wss-soap-message-security-1.0#Base64Binary"
)

// SecurityHeader represents the WS-Security header structure
type SecurityHeader struct {
	XMLName       xml.Name      `xml:"http://docs.oasis-open.org/wss/2004/01/oasis-200401-wss-wssecurity-secext-1.0.xsd Security"`
	MustUnderstand string       `xml:"http://www.w3.org/2003/05/soap-envelope mustUnderstand,attr,omitempty"`
	UsernameToken UsernameToken `xml:"UsernameToken"`
}

// UsernameToken represents the WS-Security UsernameToken element
type UsernameToken struct {
	Username string          `xml:"Username"`
	Password PasswordElement `xml:"Password"`
	Nonce    NonceElement    `xml:"Nonce"`
	Created  string          `xml:"http://docs.oasis-open.org/wss/2004/01/oasis-200401-wss-wssecurity-utility-1.0.xsd Created"`
}

// PasswordElement represents the WS-Security Password element with type attribute
type PasswordElement struct {
	Type  string `xml:"Type,attr"`
	Value string `xml:",chardata"`
}

// NonceElement represents the WS-Security Nonce element with encoding attribute
type NonceElement struct {
	EncodingType string `xml:"EncodingType,attr"`
	Value        string `xml:",chardata"`
}

// Header represents the SOAP 1.2 Header
type Header struct {
	XMLName   xml.Name        `xml:"http://www.w3.org/2003/05/soap-envelope Header"`
	Action    string          `xml:"http://www.w3.org/2005/08/addressing Action,omitempty"`
	MessageID string          `xml:"http://www.w3.org/2005/08/addressing MessageID,omitempty"`
	To        string          `xml:"http://www.w3.org/2005/08/addressing To,omitempty"`
	Security  *SecurityHeader `xml:"http://docs.oasis-open.org/wss/2004/01/oasis-200401-wss-wssecurity-secext-1.0.xsd Security,omitempty"`
}

// Envelope12 represents a SOAP 1.2 request or response envelope
type Envelope12 struct {
	XMLName xml.Name `xml:"http://www.w3.org/2003/05/soap-envelope Envelope"`
	Header  Header   `xml:"Header"`
	Body    Body     `xml:"Body"`
}

// Body represents generic SOAP Body
type Body struct {
	Content []byte `xml:",innerxml"`
}

// --- GetSystemDateAndTime types ---

// SystemDateAndTimeResponse holds the parsed response of GetSystemDateAndTime
type SystemDateAndTimeResponse struct {
	SystemDateAndTime SystemDateAndTime `xml:"GetSystemDateAndTimeResponse>SystemDateAndTime"`
}

// SystemDateAndTime structure
type SystemDateAndTime struct {
	DateTimeType    string        `xml:"DateTimeType"`
	DaylightSavings bool          `xml:"DaylightSavings"`
	TimeZone        TimeZoneInfo  `xml:"TimeZone"`
	UTCDateTime     *DateTimeInfo `xml:"UTCDateTime"`
	LocalDateTime   *DateTimeInfo `xml:"LocalDateTime"`
}

// TimeZoneInfo represents the time zone
type TimeZoneInfo struct {
	TZ string `xml:"TZ"`
}

// DateTimeInfo holds Date and Time components
type DateTimeInfo struct {
	Time TimeComponents `xml:"Time"`
	Date DateComponents `xml:"Date"`
}

// TimeComponents represents hour, minute, second
type TimeComponents struct {
	Hour   int `xml:"Hour"`
	Minute int `xml:"Minute"`
	Second int `xml:"Second"`
}

// DateComponents represents year, month, day
type DateComponents struct {
	Year  int `xml:"Year"`
	Month int `xml:"Month"`
	Day   int `xml:"Day"`
}

// ToTime converts DateTimeInfo to time.Time in UTC
func (d *DateTimeInfo) ToTime() time.Time {
	if d == nil {
		return time.Time{}
	}
	return time.Date(d.Date.Year, time.Month(d.Date.Month), d.Date.Day,
		d.Time.Hour, d.Time.Minute, d.Time.Second, 0, time.UTC)
}

// --- GetDeviceInformation types ---

// DeviceInformationEnvelope wraps GetDeviceInformationResponse
type DeviceInformationEnvelope struct {
	XMLName xml.Name `xml:"Envelope"`
	Body    struct {
		Response DeviceInformationResponse `xml:"GetDeviceInformationResponse"`
	} `xml:"Body"`
}

// DeviceInformationResponse holds camera metadata
type DeviceInformationResponse struct {
	Manufacturer    string `xml:"Manufacturer"`
	Model           string `xml:"Model"`
	FirmwareVersion string `xml:"FirmwareVersion"`
	SerialNumber    string `xml:"SerialNumber"`
	HardwareID      string `xml:"HardwareId"`
}

// --- GetCapabilities types ---

// CapabilitiesEnvelope wraps GetCapabilitiesResponse
type CapabilitiesEnvelope struct {
	XMLName xml.Name `xml:"Envelope"`
	Body    struct {
		Response CapabilitiesResponse `xml:"GetCapabilitiesResponse"`
	} `xml:"Body"`
}

// CapabilitiesResponse holds service endpoints
type CapabilitiesResponse struct {
	Capabilities struct {
		Device struct {
			XAddr string `xml:"XAddr"`
		} `xml:"Device"`
		Events struct {
			XAddr               string `xml:"XAddr"`
			WSSubscriptionPolicySupport bool   `xml:"WSSubscriptionPolicySupport"`
			WSPullPointSupport          bool   `xml:"WSPullPointSupport"`
		} `xml:"Events"`
		Media struct {
			XAddr string `xml:"XAddr"`
		} `xml:"Media"`
	} `xml:"Capabilities"`
}

// --- PullPoint types ---

// CreatePullPointSubscriptionEnvelope wraps CreatePullPointSubscriptionResponse
type CreatePullPointSubscriptionEnvelope struct {
	XMLName xml.Name `xml:"Envelope"`
	Body    struct {
		Response CreatePullPointSubscriptionResponse `xml:"CreatePullPointSubscriptionResponse"`
	} `xml:"Body"`
}

// CreatePullPointSubscriptionResponse holds the subscription endpoint
type CreatePullPointSubscriptionResponse struct {
	SubscriptionReference struct {
		Address string `xml:"Address"`
	} `xml:"SubscriptionReference"`
	CurrentTime     string `xml:"CurrentTime"`
	TerminationTime string `xml:"TerminationTime"`
}

// PullMessagesEnvelope wraps PullMessagesResponse
type PullMessagesEnvelope struct {
	XMLName xml.Name `xml:"Envelope"`
	Body    struct {
		Response PullMessagesResponse `xml:"PullMessagesResponse"`
	} `xml:"Body"`
}

// PullMessagesResponse contains notifications from PullMessages
type PullMessagesResponse struct {
	CurrentTime          string                `xml:"CurrentTime"`
	TerminationTime      string                `xml:"TerminationTime"`
	NotificationMessages []NotificationMessage `xml:"NotificationMessage"`
}

// NotificationMessage represents a single event message
type NotificationMessage struct {
	Topic struct {
		Dialect string `xml:"Dialect,attr"`
		Value   string `xml:",chardata"`
	} `xml:"Topic"`
	Message struct {
		UtcTime time.Time `xml:"UtcTime,attr"`
		Source  struct {
			SimpleItems []SimpleItem `xml:"SimpleItem"`
		} `xml:"Source"`
		Data struct {
			SimpleItems  []SimpleItem  `xml:"SimpleItem"`
			ElementItems []ElementItem `xml:"ElementItem"`
		} `xml:"Data"`
		InnerMessage *struct {
			UtcTime time.Time `xml:"UtcTime,attr"`
			Source  struct {
				SimpleItems []SimpleItem `xml:"SimpleItem"`
			} `xml:"Source"`
			Data struct {
				SimpleItems  []SimpleItem  `xml:"SimpleItem"`
				ElementItems []ElementItem `xml:"ElementItem"`
			} `xml:"Data"`
		} `xml:"Message"`
	} `xml:"Message"`
}

// GetDataSimpleItems returns simple items from Data or InnerMessage.Data
func (m *NotificationMessage) GetDataSimpleItems() []SimpleItem {
	if m.Message.InnerMessage != nil && len(m.Message.InnerMessage.Data.SimpleItems) > 0 {
		return m.Message.InnerMessage.Data.SimpleItems
	}
	return m.Message.Data.SimpleItems
}

// GetDataElementItems returns element items from Data or InnerMessage.Data
func (m *NotificationMessage) GetDataElementItems() []ElementItem {
	if m.Message.InnerMessage != nil && len(m.Message.InnerMessage.Data.ElementItems) > 0 {
		return m.Message.InnerMessage.Data.ElementItems
	}
	return m.Message.Data.ElementItems
}

// GetSourceSimpleItems returns source items from Source or InnerMessage.Source
func (m *NotificationMessage) GetSourceSimpleItems() []SimpleItem {
	if m.Message.InnerMessage != nil && len(m.Message.InnerMessage.Source.SimpleItems) > 0 {
		return m.Message.InnerMessage.Source.SimpleItems
	}
	return m.Message.Source.SimpleItems
}

// GetUtcTime returns timestamp from Message or InnerMessage
func (m *NotificationMessage) GetUtcTime() time.Time {
	if m.Message.InnerMessage != nil && !m.Message.InnerMessage.UtcTime.IsZero() {
		return m.Message.InnerMessage.UtcTime
	}
	return m.Message.UtcTime
}

// SimpleItem represents a key-value attribute
type SimpleItem struct {
	Name  string `xml:"Name,attr"`
	Value string `xml:"Value,attr"`
}

// ElementItem represents complex xml nodes inside Data
type ElementItem struct {
	Name    string `xml:"Name,attr"`
	Content string `xml:",innerxml"`
}

// PushSubscription represents an active WS-BaseNotification push subscription
type PushSubscription struct {
	SubscriptionURL string
	CurrentTime     string
	TerminationTime string
}

// SubscribeEnvelope wraps SubscribeResponse from WS-BaseNotification
type SubscribeEnvelope struct {
	XMLName xml.Name `xml:"Envelope"`
	Body    struct {
		Response SubscribeResponse `xml:"SubscribeResponse"`
	} `xml:"Body"`
}

// SubscribeResponse holds the subscription endpoint for push notifications
type SubscribeResponse struct {
	SubscriptionReference struct {
		Address string `xml:"Address"`
	} `xml:"SubscriptionReference"`
	CurrentTime     string `xml:"CurrentTime"`
	TerminationTime string `xml:"TerminationTime"`
}

// NotifyEnvelope wraps an incoming WS-BaseNotification Notify message
type NotifyEnvelope struct {
	XMLName xml.Name `xml:"Envelope"`
	Body    struct {
		Notify struct {
			NotificationMessages []NotificationMessage `xml:"NotificationMessage"`
		} `xml:"Notify"`
	} `xml:"Body"`
}
