package onvif

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type testHandler struct {
	mu            sync.Mutex
	statusEvents  []SubscriptionState
	notifications []NotificationMessage
}

func (h *testHandler) OnStatusChange(state SubscriptionState, reason string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.statusEvents = append(h.statusEvents, state)
}

func (h *testHandler) OnNotification(msg NotificationMessage, req string, resp string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.notifications = append(h.notifications, msg)
}

func TestSubscriptionManagerLifecycleAndUnsubscribe(t *testing.T) {
	var unsubscribed int32

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/soap+xml; charset=utf-8")

		action := r.Header.Get("SOAPAction")
		if action == "" {
			ct := r.Header.Get("Content-Type")
			if idx := strings.Index(ct, `action="`); idx != -1 {
				action = ct[idx+8:]
				action = strings.TrimSuffix(action, `"`)
			}
		}

		switch {
		case strings.Contains(action, "CreatePullPointSubscriptionRequest"):
			w.WriteHeader(http.StatusOK)
			_, _ = fmt.Fprintf(w, `<?xml version="1.0" encoding="utf-8"?>
<s:Envelope xmlns:s="http://www.w3.org/2003/05/soap-envelope" xmlns:tev="http://www.onvif.org/ver10/events/wsdl" xmlns:wsa="http://www.w3.org/2005/08/addressing">
  <s:Body>
    <tev:CreatePullPointSubscriptionResponse>
      <tev:SubscriptionReference>
        <wsa:Address>http://%s/subscription/test</wsa:Address>
      </tev:SubscriptionReference>
      <tev:CurrentTime>2026-10-17T12:00:00Z</tev:CurrentTime>
      <tev:TerminationTime>2026-10-17T12:01:00Z</tev:TerminationTime>
    </tev:CreatePullPointSubscriptionResponse>
  </s:Body>
</s:Envelope>`, r.Host)

		case strings.Contains(action, "PullMessagesRequest"):
			w.WriteHeader(http.StatusOK)
			_, _ = fmt.Fprint(w, `<?xml version="1.0" encoding="utf-8"?>
<s:Envelope xmlns:s="http://www.w3.org/2003/05/soap-envelope" xmlns:tev="http://www.onvif.org/ver10/events/wsdl">
  <s:Body>
    <tev:PullMessagesResponse>
      <tev:NotificationMessage>
        <tev:Topic>tns1:VideoSource/MotionAlarm</tev:Topic>
        <tev:Message>
          <tev:Data>
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
    <wsnt:RenewResponse><wsnt:TerminationTime>2026-10-17T12:02:00Z</wsnt:TerminationTime></wsnt:RenewResponse>
  </s:Body>
</s:Envelope>`)

		case strings.Contains(action, "UnsubscribeRequest"):
			atomic.AddInt32(&unsubscribed, 1)
			w.WriteHeader(http.StatusOK)
			_, _ = fmt.Fprint(w, `<?xml version="1.0" encoding="utf-8"?>
<s:Envelope xmlns:s="http://www.w3.org/2003/05/soap-envelope" xmlns:wsnt="http://docs.oasis-open.org/wsn/b-2">
  <s:Body><wsnt:UnsubscribeResponse/></s:Body>
</s:Envelope>`)

		default:
			w.WriteHeader(http.StatusOK)
		}
	}))
	defer ts.Close()

	client, err := NewClient(ClientOptions{
		DeviceURL: ts.URL + "/onvif/device_service",
		Credentials: SecurityCredentials{
			Username: "admin",
			Password: "pass",
		},
		HTTPTimeout: 2 * time.Second,
	})
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	client.EventsXAddr = ts.URL + "/onvif/events"

	handler := &testHandler{}
	sm := NewSubscriptionManager(client, handler)
	sm.pullTimeout = "PT1S"

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sm.Start(ctx)

	// Wait for at least 1 notification
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		handler.mu.Lock()
		count := len(handler.notifications)
		handler.mu.Unlock()
		if count > 0 {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}

	handler.mu.Lock()
	count := len(handler.notifications)
	handler.mu.Unlock()
	if count == 0 {
		t.Error("expected at least 1 notification")
	}

	// Stop subscription manager
	sm.Stop()

	if atomic.LoadInt32(&unsubscribed) == 0 {
		t.Error("expected UnsubscribeRequest to be sent on Stop()")
	}

	if sm.GetState() != StateClosed {
		t.Errorf("expected StateClosed, got %s", sm.GetState())
	}
}
