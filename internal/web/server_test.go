package web

import (
	"bufio"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/smford/onvif-inspector/internal/config"
	"github.com/smford/onvif-inspector/internal/hub"
)

func TestWebServerEndpoints(t *testing.T) {
	h := hub.NewHub(100)

	h.UpdateDevice(hub.DeviceStatus{
		Endpoint: "urn:uuid:cam-web-1",
		IP:       "192.168.1.150",
		Port:     80,
		Model:    "WebTestModel",
		State:    "Active",
	})

	h.Publish(hub.Event{
		ID:            "evt-web-1",
		Timestamp:     time.Now().UTC(),
		CameraID:      "urn:uuid:cam-web-1",
		CameraIP:      "192.168.1.150",
		Category:      "Motion",
		Topic:         "tns1:RuleEngine/CellMotionDetector/Motion",
		PropertyState: "true",
	})

	srv := NewServer(Config{
		Hub:  h,
		Port: 0,
	})
	defer srv.Stop(context.Background())

	handler := srv.Handler()

	// 1. Test Static files
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for /, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "ONVIF-Inspector") {
		t.Errorf("missing title in index.html response: %s", rec.Body.String()[:100])
	}

	// 2. Test /api/devices
	req = httptest.NewRequest(http.MethodGet, "/api/devices", nil)
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for /api/devices, got %d", rec.Code)
	}
	var devs []hub.DeviceStatus
	if err := json.Unmarshal(rec.Body.Bytes(), &devs); err != nil {
		t.Fatalf("failed to decode devices JSON: %v", err)
	}
	if len(devs) != 1 || devs[0].Model != "WebTestModel" {
		t.Errorf("unexpected devices: %+v", devs)
	}

	// 3. Test /api/events
	req = httptest.NewRequest(http.MethodGet, "/api/events", nil)
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for /api/events, got %d", rec.Code)
	}
	var evs []hub.Event
	if err := json.Unmarshal(rec.Body.Bytes(), &evs); err != nil {
		t.Fatalf("failed to decode events JSON: %v", err)
	}
	if len(evs) != 1 || evs[0].ID != "evt-web-1" {
		t.Errorf("unexpected events: %+v", evs)
	}
}

func TestSSEStream(t *testing.T) {
	h := hub.NewHub(100)
	srv := NewServer(Config{
		Hub:  h,
		Port: 0,
	})
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()
	defer srv.Stop(context.Background())

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, ts.URL+"/api/events/stream", nil)
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("failed to connect to SSE stream: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected status 200, got %d", resp.StatusCode)
	}

	reader := bufio.NewReader(resp.Body)

	// Read initial connected comment
	line, err := reader.ReadString('\n')
	if err != nil {
		t.Fatalf("failed to read initial line: %v", err)
	}
	if !strings.Contains(line, ": connected") {
		t.Errorf("expected ': connected', got %q", line)
	}

	// Now publish an event in background
	go func() {
		time.Sleep(50 * time.Millisecond)
		h.Publish(hub.Event{
			ID:            "evt-sse-broadcast",
			CameraIP:      "192.168.1.200",
			Category:      "Motion",
			Topic:         "tns1:RuleEngine/Motion",
			PropertyState: "true",
		})
	}()

	// Expect to receive the SSE event
	foundEvent := false
	for i := 0; i < 10; i++ {
		line, err = reader.ReadString('\n')
		if err != nil {
			break
		}
		if strings.Contains(line, "evt-sse-broadcast") {
			foundEvent = true
			break
		}
	}

	if !foundEvent {
		t.Error("did not receive published event over SSE stream")
	}
}

func TestWebServerConfigEndpoints(t *testing.T) {
	h := hub.NewHub(100)
	tmpFile, err := os.CreateTemp("", "web-config-*.yaml")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())
	tmpFile.Close()

	appCfg := &config.Config{}
	appCfg.SetFilePath(tmpFile.Name())

	var lastUpdatedTarget string
	srv := NewServer(Config{
		Hub:       h,
		AppConfig: appCfg,
		OnConfigUpdate: func(target, username, password string) {
			lastUpdatedTarget = target
		},
		Port: 0,
	})
	defer srv.Stop(context.Background())
	handler := srv.Handler()

	// 1. POST /api/config/default
	defaultPayload := `{"username": "admin_test", "password": "pass_test"}`
	req := httptest.NewRequest(http.MethodPost, "/api/config/default", strings.NewReader(defaultPayload))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for /api/config/default, got %d: %s", rec.Code, rec.Body.String())
	}

	// 2. POST /api/config/camera
	camPayload := `{"ip": "192.168.1.88", "target": "192.168.1.88:80", "username": "cam_user", "password": "cam_pass"}`
	req = httptest.NewRequest(http.MethodPost, "/api/config/camera", strings.NewReader(camPayload))
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for /api/config/camera, got %d: %s", rec.Code, rec.Body.String())
	}
	if lastUpdatedTarget != "192.168.1.88:80" {
		t.Errorf("expected callback with 192.168.1.88:80, got %s", lastUpdatedTarget)
	}

	// 3. GET /api/config
	req = httptest.NewRequest(http.MethodGet, "/api/config", nil)
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for /api/config, got %d", rec.Code)
	}

	var resp struct {
		FilePath string `json:"file_path"`
		Default  struct {
			Username    string `json:"username"`
			HasPassword bool   `json:"has_password"`
		} `json:"default"`
		Cameras []struct {
			IP          string `json:"ip"`
			Target      string `json:"target"`
			Username    string `json:"username"`
			HasPassword bool   `json:"has_password"`
		} `json:"cameras"`
	}

	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode /api/config JSON: %v", err)
	}
	if resp.Default.Username != "admin_test" || !resp.Default.HasPassword {
		t.Errorf("unexpected default config: %+v", resp.Default)
	}
	if len(resp.Cameras) != 1 || resp.Cameras[0].IP != "192.168.1.88" {
		t.Errorf("unexpected cameras config: %+v", resp.Cameras)
	}

	// 4. DELETE /api/config/camera
	req = httptest.NewRequest(http.MethodDelete, "/api/config/camera?target=192.168.1.88:80", nil)
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for DELETE /api/config/camera, got %d", rec.Code)
	}

	// Verify removal in GET
	req = httptest.NewRequest(http.MethodGet, "/api/config", nil)
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	if len(resp.Cameras) != 0 {
		t.Errorf("expected 0 cameras after delete, got %d", len(resp.Cameras))
	}
}
