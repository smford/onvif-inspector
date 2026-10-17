package onvif

import (
	"bytes"
	"fmt"
	"net/http"
	"sync"
	"testing"
	"time"
)

type mockEventHandler struct {
	mu            sync.Mutex
	notifications []NotificationMessage
	statusChanges []SubscriptionState
}

func (m *mockEventHandler) OnStatusChange(state SubscriptionState, reason string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.statusChanges = append(m.statusChanges, state)
}

func (m *mockEventHandler) OnNotification(msg NotificationMessage, rawReq, rawResp string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.notifications = append(m.notifications, msg)
}

func TestPushReceiverLifecycleAndNotificationDelivery(t *testing.T) {
	pr, err := NewPushReceiver(0)
	if err != nil {
		t.Fatalf("failed to create PushReceiver: %v", err)
	}
	defer pr.Close()

	if pr.Port() <= 0 {
		t.Errorf("expected positive port, got %d", pr.Port())
	}

	handler := &mockEventHandler{}
	pr.RegisterHandler("garden1", handler)

	// Simulate incoming ONVIF notification HTTP POST
	notifyXML := `<?xml version="1.0" encoding="utf-8"?>
<s:Envelope xmlns:s="http://www.w3.org/2003/05/soap-envelope"
            xmlns:wsnt="http://docs.oasis-open.org/wsn/b-2"
            xmlns:tt="http://www.onvif.org/ver10/schema">
  <s:Body>
    <wsnt:Notify>
      <wsnt:NotificationMessage>
        <wsnt:Topic Dialect="http://www.onvif.org/ver10/tev/topicExpression/ConcreteSet">tns1:RuleEngine/CellMotionDetector/Motion</wsnt:Topic>
        <wsnt:Message>
          <tt:Message UtcTime="2026-10-17T21:00:00Z">
            <tt:Data>
              <tt:SimpleItem Name="IsMotion" Value="true"/>
            </tt:Data>
          </tt:Message>
        </wsnt:Message>
      </wsnt:NotificationMessage>
    </wsnt:Notify>
  </s:Body>
</s:Envelope>`

	url := fmt.Sprintf("http://127.0.0.1:%d/notify/garden1", pr.Port())
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewBufferString(notifyXML))
	if err != nil {
		t.Fatalf("failed to build request: %v", err)
	}
	req.Header.Set("Content-Type", "application/soap+xml; charset=utf-8")

	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("failed to post notification: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected HTTP 200, got %d", resp.StatusCode)
	}

	handler.mu.Lock()
	defer handler.mu.Unlock()
	if len(handler.notifications) != 1 {
		t.Fatalf("expected 1 notification, got %d", len(handler.notifications))
	}
	msg := handler.notifications[0]
	if msg.Topic.Value != "tns1:RuleEngine/CellMotionDetector/Motion" {
		t.Errorf("unexpected topic: %s", msg.Topic.Value)
	}
	dataItems := msg.GetDataSimpleItems()
	if len(dataItems) != 1 || dataItems[0].Value != "true" {
		t.Errorf("unexpected data items: %+v", dataItems)
	}
}
