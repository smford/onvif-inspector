package hub

import (
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/smford/onvif-inspector/internal/onvif"
)

// Category constants for normalized ONVIF events
const (
	CategoryMotion       = "Motion"
	CategoryDigitalInput = "Input"
	CategoryTamper       = "Tamper"
	CategoryAudio        = "Audio"
	CategorySystem       = "System"
	CategoryOther        = "Other"
)

// Event represents a parsed, normalized ONVIF event
type Event struct {
	ID            string            `json:"id"`
	Timestamp     time.Time         `json:"timestamp"`
	CameraID      string            `json:"camera_id"`
	CameraIP      string            `json:"camera_ip"`
	CameraName    string            `json:"camera_name,omitempty"`
	Topic         string            `json:"topic"`
	Category      string            `json:"category"`
	PropertyState string            `json:"property_state"`
	Data          map[string]string `json:"data"`
	Source        map[string]string `json:"source"`
	RawXML        string            `json:"raw_xml,omitempty"`
	RawRequest    string            `json:"raw_request,omitempty"`
}

// DeviceStatus tracks an ONVIF device and its subscription state
type DeviceStatus struct {
	Endpoint        string        `json:"endpoint"`
	IP              string        `json:"ip"`
	Port            int           `json:"port"`
	Name            string        `json:"name"`
	Manufacturer    string        `json:"manufacturer"`
	Model           string        `json:"model"`
	FirmwareVersion string        `json:"firmware_version"`
	SerialNumber    string        `json:"serial_number"`
	State           string        `json:"state"` // "Active", "Reconnecting", "Failed", "Closed"
	StateReason     string        `json:"state_reason"`
	PingLatency     time.Duration `json:"ping_latency_ms"`
	EventCount      int64         `json:"event_count"`
	LastSeen        time.Time     `json:"last_seen"`
}

// Hub manages pub/sub distribution of events and device states, with a bounded ring buffer
type Hub struct {
	buffer *RingBuffer

	mu           sync.RWMutex
	devices      map[string]*DeviceStatus
	eventSubs    map[uint64]chan Event
	deviceSubs   map[uint64]chan DeviceStatus
	nextSubID    uint64
	eventCounter uint64
}

// NewHub creates a new central Hub with the default ring buffer capacity of 1,000
func NewHub(bufferCap int) *Hub {
	if bufferCap <= 0 {
		bufferCap = 1000
	}
	return &Hub{
		buffer:     NewRingBuffer(bufferCap),
		devices:    make(map[string]*DeviceStatus),
		eventSubs:  make(map[uint64]chan Event),
		deviceSubs: make(map[uint64]chan DeviceStatus),
	}
}

// NormalizeEvent transforms an ONVIF NotificationMessage into a structured Event
func NormalizeEvent(camID, camIP string, msg onvif.NotificationMessage, rawRequest, rawResponse string) Event {
	sourceMap := make(map[string]string)
	for _, s := range msg.GetSourceSimpleItems() {
		sourceMap[s.Name] = s.Value
	}

	dataMap := make(map[string]string)
	for _, d := range msg.GetDataSimpleItems() {
		dataMap[d.Name] = d.Value
	}
	for _, elem := range msg.GetDataElementItems() {
		dataMap[elem.Name] = elem.Content
	}

	topic := msg.Topic.Value
	category := CategorizeTopic(topic)
	propState := ExtractPropertyState(dataMap)

	ts := msg.GetUtcTime()
	if ts.IsZero() {
		ts = time.Now().UTC()
	}

	return Event{
		ID:            fmt.Sprintf("evt-%d", time.Now().UnixNano()),
		Timestamp:     ts,
		CameraID:      camID,
		CameraIP:      camIP,
		Topic:         topic,
		Category:      category,
		PropertyState: propState,
		Data:          dataMap,
		Source:        sourceMap,
		RawXML:        rawResponse,
		RawRequest:    rawRequest,
	}
}

// CategorizeTopic analyzes an ONVIF topic string and returns a normalized category
func CategorizeTopic(topic string) string {
	lower := strings.ToLower(topic)
	switch {
	case strings.Contains(lower, "tamper") || strings.Contains(lower, "defocus") || strings.Contains(lower, "scenechange"):
		return CategoryTamper
	case strings.Contains(lower, "audio") || strings.Contains(lower, "sound") || strings.Contains(lower, "noise"):
		return CategoryAudio
	case strings.Contains(lower, "motion") || strings.Contains(lower, "cellmotion") || strings.Contains(lower, "motiondetector"):
		return CategoryMotion
	case strings.Contains(lower, "digitalinput") || strings.Contains(lower, "relay") || strings.Contains(lower, "trigger/input"):
		return CategoryDigitalInput
	case strings.Contains(lower, "reboot") || strings.Contains(lower, "shutdown") || strings.Contains(lower, "system") || strings.Contains(lower, "storage") || strings.Contains(lower, "network"):
		return CategorySystem
	case strings.Contains(lower, "ruleengine"):
		return CategoryMotion
	default:
		return CategoryOther
	}
}

// ExtractPropertyState finds the primary state value from event data
func ExtractPropertyState(data map[string]string) string {
	for _, key := range []string{"State", "state", "IsInside", "LogicalState", "Value", "Active", "active"} {
		if val, ok := data[key]; ok {
			return val
		}
	}
	if len(data) > 0 {
		for k, v := range data {
			return fmt.Sprintf("%s=%s", k, v)
		}
	}
	return "true"
}

// Publish stores an event in the ring buffer and broadcasts to all subscribers
func (h *Hub) Publish(e Event) {
	if e.ID == "" {
		id := atomic.AddUint64(&h.eventCounter, 1)
		e.ID = fmt.Sprintf("evt-%d-%d", time.Now().Unix(), id)
	}

	// Update device event count and last seen
	h.mu.Lock()
	if dev, exists := h.devices[e.CameraID]; exists {
		dev.EventCount++
		dev.LastSeen = e.Timestamp
	} else if dev, exists := h.devices[e.CameraIP]; exists {
		dev.EventCount++
		dev.LastSeen = e.Timestamp
	}
	h.mu.Unlock()

	// Push to bounded ring buffer
	h.buffer.Push(e)

	// Broadcast to subscribers
	h.mu.RLock()
	defer h.mu.RUnlock()

	for _, ch := range h.eventSubs {
		select {
		case ch <- e:
		default:
			// Non-blocking drop if subscriber is too slow to avoid blocking other workers
		}
	}
}

// UpdateDevice updates or creates a device status entry and notifies device subscribers
func (h *Hub) UpdateDevice(status DeviceStatus) {
	status.LastSeen = time.Now().UTC()

	h.mu.Lock()
	key := status.Endpoint
	if key == "" {
		if status.Port > 0 {
			key = fmt.Sprintf("%s:%d", status.IP, status.Port)
		} else {
			key = status.IP
		}
	}

	existing, exists := h.devices[key]
	if exists {
		if status.Name == "" {
			status.Name = existing.Name
		}
		if status.Manufacturer == "" {
			status.Manufacturer = existing.Manufacturer
		}
		if status.Model == "" {
			status.Model = existing.Model
		}
		if status.FirmwareVersion == "" {
			status.FirmwareVersion = existing.FirmwareVersion
		}
		if status.SerialNumber == "" {
			status.SerialNumber = existing.SerialNumber
		}
		if status.EventCount == 0 {
			status.EventCount = existing.EventCount
		}
		if status.PingLatency == 0 {
			status.PingLatency = existing.PingLatency
		}
	}
	h.devices[key] = &status
	h.mu.Unlock()

	// Broadcast update
	h.mu.RLock()
	defer h.mu.RUnlock()

	for _, ch := range h.deviceSubs {
		select {
		case ch <- status:
		default:
		}
	}
}

// SubscribeEvents registers a channel to receive live events
func (h *Hub) SubscribeEvents(bufSize int) (<-chan Event, func()) {
	if bufSize <= 0 {
		bufSize = 100
	}
	ch := make(chan Event, bufSize)

	h.mu.Lock()
	subID := h.nextSubID
	h.nextSubID++
	h.eventSubs[subID] = ch
	h.mu.Unlock()

	unsub := func() {
		h.mu.Lock()
		delete(h.eventSubs, subID)
		close(ch)
		h.mu.Unlock()
	}

	return ch, unsub
}

// SubscribeDevices registers a channel to receive device status changes
func (h *Hub) SubscribeDevices(bufSize int) (<-chan DeviceStatus, func()) {
	if bufSize <= 0 {
		bufSize = 50
	}
	ch := make(chan DeviceStatus, bufSize)

	h.mu.Lock()
	subID := h.nextSubID
	h.nextSubID++
	h.deviceSubs[subID] = ch
	h.mu.Unlock()

	unsub := func() {
		h.mu.Lock()
		delete(h.deviceSubs, subID)
		close(ch)
		h.mu.Unlock()
	}

	return ch, unsub
}

// GetDevices returns snapshot of all known devices
func (h *Hub) GetDevices() []DeviceStatus {
	h.mu.RLock()
	defer h.mu.RUnlock()

	res := make([]DeviceStatus, 0, len(h.devices))
	for _, d := range h.devices {
		res = append(res, *d)
	}
	return res
}

// GetHistory returns historical events up to maxCount
func (h *Hub) GetHistory(maxCount int) []Event {
	if maxCount <= 0 {
		return h.buffer.GetAll()
	}
	return h.buffer.GetLast(maxCount)
}
