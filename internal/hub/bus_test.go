package hub

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/smford/onvif-inspector/internal/onvif"
)

func TestRingBufferCapacityAndOrdering(t *testing.T) {
	rb := NewRingBuffer(5)

	if rb.Len() != 0 {
		t.Errorf("expected 0 items, got %d", rb.Len())
	}

	for i := 1; i <= 5; i++ {
		rb.Push(Event{ID: fmt.Sprintf("e%d", i)})
	}

	if rb.Len() != 5 {
		t.Errorf("expected 5 items, got %d", rb.Len())
	}

	all := rb.GetAll()
	if len(all) != 5 {
		t.Fatalf("expected 5 items, got %d", len(all))
	}
	if all[0].ID != "e1" || all[4].ID != "e5" {
		t.Errorf("unexpected order: first=%s, last=%s", all[0].ID, all[4].ID)
	}

	// Push 3 more to cause wraparound
	rb.Push(Event{ID: "e6"})
	rb.Push(Event{ID: "e7"})
	rb.Push(Event{ID: "e8"})

	if rb.Len() != 5 {
		t.Errorf("expected 5 items after overflow, got %d", rb.Len())
	}

	all = rb.GetAll()
	expectedIDs := []string{"e4", "e5", "e6", "e7", "e8"}
	for i, exp := range expectedIDs {
		if all[i].ID != exp {
			t.Errorf("at index %d: expected %s, got %s", i, exp, all[i].ID)
		}
	}

	last2 := rb.GetLast(2)
	if len(last2) != 2 || last2[0].ID != "e7" || last2[1].ID != "e8" {
		t.Errorf("unexpected GetLast(2): %+v", last2)
	}
}

func TestCategorizeTopic(t *testing.T) {
	tests := []struct {
		topic    string
		expected string
	}{
		{"tns1:RuleEngine/CellMotionDetector/Motion", CategoryMotion},
		{"tns1:VideoSource/MotionAlarm", CategoryMotion},
		{"tns1:Device/Trigger/DigitalInput", CategoryDigitalInput},
		{"tns1:Device/HardwareFailure/StorageFailure", CategorySystem},
		{"tns1:RuleEngine/TamperDetector/Tamper", CategoryTamper},
		{"tns1:Audio/AudioDetected", CategoryAudio},
		{"tns1:SomeCustomVendor/SpecialTopic", CategoryOther},
	}

	for _, tt := range tests {
		cat := CategorizeTopic(tt.topic)
		if cat != tt.expected {
			t.Errorf("for topic %q: expected %q, got %q", tt.topic, tt.expected, cat)
		}
	}
}

func TestNormalizeEvent(t *testing.T) {
	msg := onvif.NotificationMessage{}
	msg.Topic.Value = "tns1:RuleEngine/CellMotionDetector/Motion"
	msg.Message.UtcTime = time.Date(2026, 10, 17, 12, 0, 0, 0, time.UTC)
	msg.Message.Source.SimpleItems = []onvif.SimpleItem{
		{Name: "VideoSourceToken", Value: "token0"},
	}
	msg.Message.Data.SimpleItems = []onvif.SimpleItem{
		{Name: "State", Value: "true"},
		{Name: "IsInside", Value: "true"},
	}

	ev := NormalizeEvent("cam-1", "192.168.1.50", msg, "<req/>", "<resp/>")

	if ev.CameraID != "cam-1" || ev.CameraIP != "192.168.1.50" {
		t.Errorf("unexpected camera ID/IP: %s / %s", ev.CameraID, ev.CameraIP)
	}
	if ev.Category != CategoryMotion {
		t.Errorf("expected CategoryMotion, got %s", ev.Category)
	}
	if ev.PropertyState != "true" {
		t.Errorf("expected PropertyState 'true', got %s", ev.PropertyState)
	}
	if ev.Data["State"] != "true" || ev.Data["IsInside"] != "true" {
		t.Errorf("missing data keys: %+v", ev.Data)
	}
	if ev.Source["VideoSourceToken"] != "token0" {
		t.Errorf("missing source key: %+v", ev.Source)
	}
}

func TestHubPubSubAndDeviceUpdate(t *testing.T) {
	hub := NewHub(100)

	evCh, unsubEv := hub.SubscribeEvents(10)
	defer unsubEv()

	devCh, unsubDev := hub.SubscribeDevices(10)
	defer unsubDev()

	var wg sync.WaitGroup
	wg.Add(2)

	var receivedEv Event
	go func() {
		defer wg.Done()
		receivedEv = <-evCh
	}()

	var receivedDev DeviceStatus
	go func() {
		defer wg.Done()
		receivedDev = <-devCh
	}()

	hub.UpdateDevice(DeviceStatus{
		Endpoint: "urn:uuid:cam-101",
		IP:       "10.0.0.10",
		Model:    "ProCam",
		State:    "Active",
	})

	hub.Publish(Event{
		CameraID:      "urn:uuid:cam-101",
		CameraIP:      "10.0.0.10",
		Topic:         "tns1:RuleEngine/Motion",
		Category:      CategoryMotion,
		PropertyState: "true",
	})

	wg.Wait()

	if receivedEv.CameraID != "urn:uuid:cam-101" {
		t.Errorf("unexpected event received: %+v", receivedEv)
	}
	if receivedDev.Model != "ProCam" || receivedDev.State != "Active" {
		t.Errorf("unexpected device received: %+v", receivedDev)
	}

	history := hub.GetHistory(10)
	if len(history) != 1 {
		t.Fatalf("expected 1 history event, got %d", len(history))
	}
	if history[0].CameraID != "urn:uuid:cam-101" {
		t.Errorf("unexpected history item: %+v", history[0])
	}
}
