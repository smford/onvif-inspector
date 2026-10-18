package tui

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/smford/onvif-inspector/internal/config"
	"github.com/smford/onvif-inspector/internal/hub"
)

func TestTUIModelAndViews(t *testing.T) {
	h := hub.NewHub(100)

	h.UpdateDevice(hub.DeviceStatus{
		Endpoint: "urn:uuid:cam-1",
		IP:       "192.168.1.100",
		Port:     80,
		Model:    "TestCam",
		State:    "Active",
	})

	h.Publish(hub.Event{
		ID:            "evt-1",
		Timestamp:     time.Now().UTC(),
		CameraID:      "urn:uuid:cam-1",
		CameraIP:      "192.168.1.100",
		Category:      "Motion",
		Topic:         "tns1:RuleEngine/CellMotionDetector/Motion",
		PropertyState: "true",
		RawXML:        "<SOAP-ENV:Envelope/>",
	})

	m := NewModel(h)

	// Simulate window size
	newM, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	model := newM.(Model)

	view := model.View()
	if !strings.Contains(view, "ONVIF-INSPECTOR") {
		t.Errorf("view missing header: %s", view)
	}
	if !strings.Contains(view, "192.168.1.100") {
		t.Errorf("view missing camera IP: %s", view)
	}
	if !strings.Contains(view, "MOTION") {
		t.Errorf("view missing MOTION badge: %s", view)
	}

	// Test Tab key navigation
	newM, _ = model.Update(tea.KeyMsg{Type: tea.KeyTab})
	model = newM.(Model)
	if model.activePane != PaneInspector {
		t.Errorf("expected PaneInspector, got %d", model.activePane)
	}

	// Test 'x' key for XML toggle
	newM, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	model = newM.(Model)
	if !model.showRawXML {
		t.Error("expected showRawXML to be true")
	}

	viewWithXML := model.View()
	if !strings.Contains(viewWithXML, "<SOAP-ENV:Envelope/>") {
		t.Errorf("expected XML view in inspector: %s", viewWithXML)
	}
}

func TestTUIConfigurationModal(t *testing.T) {
	tmpFile := t.TempDir() + "/test-tui-config.yaml"
	cfg, err := config.LoadOrCreateConfig(tmpFile)
	if err != nil {
		t.Fatalf("failed to create test config: %v", err)
	}

	h := hub.NewHub(100)
	h.UpdateDevice(hub.DeviceStatus{
		Endpoint: "urn:uuid:cam-1",
		IP:       "192.168.1.100",
		Port:     80,
		Model:    "TestCam",
		State:    "Active",
	})

	var updatedTarget, updatedUser, updatedPass string
	onUpdate := func(target, username, password string) {
		updatedTarget = target
		updatedUser = username
		updatedPass = password
	}

	m := NewModel(h, Options{
		AppConfig:      cfg,
		OnConfigUpdate: onUpdate,
	})

	newM, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	model := newM.(Model)

	// Press 'c' to open configuration modal
	newM, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'c'}})
	model = newM.(Model)

	if !model.isConfiguring {
		t.Fatal("expected isConfiguring to be true after pressing 'c'")
	}

	// Verify target was pre-filled with the active camera (input 1)
	if model.configInputs[1].Value() != "192.168.1.100:80" {
		t.Errorf("expected target 192.168.1.100:80, got %s", model.configInputs[1].Value())
	}

	// Set Name and Password
	model.configInputs[0].SetValue("Backyard Camera")
	model.configInputs[3].SetValue("TuiSecretPass")

	// Press Enter to save
	newM, _ = model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = newM.(Model)

	if model.configErr {
		t.Fatalf("unexpected configErr: %s", model.configMessage)
	}
	if !strings.Contains(model.configMessage, "saved to") {
		t.Errorf("expected success message, got: %s", model.configMessage)
	}

	// Verify callback received the update
	if updatedTarget != "192.168.1.100:80" || updatedUser != "admin" || updatedPass != "TuiSecretPass" {
		t.Errorf("callback not fired as expected: target=%s, user=%s, pass=%s", updatedTarget, updatedUser, updatedPass)
	}

	// Verify name was persisted in config
	if camName := cfg.GetCameraName("192.168.1.100", 80, "", ""); camName != "Backyard Camera" {
		t.Errorf("expected saved camera name 'Backyard Camera', got %q", camName)
	}

	// Verify modal renders with title and save button
	modalView := model.View()
	if !strings.Contains(modalView, "CAMERA CONFIGURATION") {
		t.Errorf("modal view missing header: %s", modalView)
	}
	if !strings.Contains(modalView, "SAVE TO CONFIG.YAML") {
		t.Errorf("modal view missing save button: %s", modalView)
	}

	// Press Esc to close
	newM, _ = model.Update(tea.KeyMsg{Type: tea.KeyEsc})
	model = newM.(Model)

	if model.isConfiguring {
		t.Error("expected isConfiguring to be false after Esc")
	}
}

func TestTUIManyCamerasAndFrameAlignment(t *testing.T) {
	h := hub.NewHub(100)

	// Add 7 cameras, including 4 on 127.0.0.1 with different ports
	h.UpdateDevice(hub.DeviceStatus{Endpoint: "urn:uuid:cam-1", IP: "192.168.1.113", Port: 2020, Name: "garden1", State: "Active"})
	h.UpdateDevice(hub.DeviceStatus{Endpoint: "urn:uuid:cam-2", IP: "192.168.1.160", Port: 2020, Name: "camera1", State: "Active"})
	h.UpdateDevice(hub.DeviceStatus{Endpoint: "urn:uuid:cam-3", IP: "192.168.1.12", Port: 2020, Name: "tort1", State: "Active"})
	h.UpdateDevice(hub.DeviceStatus{Endpoint: "urn:uuid:cam-4", IP: "127.0.0.1", Port: 8000, Name: "virtcam0", State: "Active"})
	h.UpdateDevice(hub.DeviceStatus{Endpoint: "urn:uuid:cam-5", IP: "127.0.0.1", Port: 8001, Name: "virtcam1", State: "Active"})
	h.UpdateDevice(hub.DeviceStatus{Endpoint: "urn:uuid:cam-6", IP: "127.0.0.1", Port: 8002, Name: "virtcam2", State: "Active"})
	h.UpdateDevice(hub.DeviceStatus{Endpoint: "urn:uuid:cam-7", IP: "127.0.0.1", Port: 8003, Name: "virtcam3", State: "Active"})

	m := NewModel(h)
	newM, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	model := newM.(Model)

	// Verify all 7 devices are preserved in model
	if len(model.devices) != 7 {
		t.Fatalf("expected 7 devices in model, got %d", len(model.devices))
	}

	// Test DeviceMsg updates: updating virtcam1 must not clobber virtcam0
	updatedDev := hub.DeviceStatus{Endpoint: "urn:uuid:cam-5", IP: "127.0.0.1", Port: 8001, Name: "virtcam1-renamed", State: "Active"}
	newM, _ = model.Update(DeviceMsg(updatedDev))
	model = newM.(Model)

	if len(model.devices) != 7 {
		t.Fatalf("expected 7 devices after DeviceMsg update, got %d", len(model.devices))
	}

	view := model.View()
	if !strings.Contains(view, "DEVICES (7)") {
		t.Errorf("view title should indicate 7 devices, got: %s", view)
	}

	// Set active pane to PaneDevices for navigation
	model.activePane = PaneDevices

	// Verify navigation down reaches device 6 (virtcam3)
	for i := 0; i < 6; i++ {
		newM, _ = model.Update(tea.KeyMsg{Type: tea.KeyDown})
		model = newM.(Model)
	}
	if model.deviceIndex != 6 {
		t.Errorf("expected deviceIndex 6 after scrolling down, got %d", model.deviceIndex)
	}

	scrollDownView := model.View()
	if !strings.Contains(scrollDownView, "virtcam3") {
		t.Errorf("view after scrolling down should contain virtcam3: %s", scrollDownView)
	}
}

func TestTUICameraFilteringAndNavigation(t *testing.T) {
	h := hub.NewHub(100)

	h.UpdateDevice(hub.DeviceStatus{Endpoint: "urn:uuid:cam-1", IP: "192.168.1.113", Port: 2020, Name: "garden1", State: "Active"})
	h.UpdateDevice(hub.DeviceStatus{Endpoint: "urn:uuid:cam-2", IP: "192.168.1.160", Port: 2020, Name: "camera1", State: "Active"})

	h.Publish(hub.Event{
		ID:         "evt-garden-1",
		Timestamp:  time.Now().UTC(),
		CameraID:   "urn:uuid:cam-1",
		CameraIP:   "192.168.1.113",
		CameraName: "garden1",
		Category:   "Motion",
		Topic:      "tns1:RuleEngine/CellMotionDetector/Motion",
	})
	h.Publish(hub.Event{
		ID:         "evt-camera1-1",
		Timestamp:  time.Now().UTC(),
		CameraID:   "urn:uuid:cam-2",
		CameraIP:   "192.168.1.160",
		CameraName: "camera1",
		Category:   "Input",
		Topic:      "tns1:Device/Trigger/DigitalInput",
	})

	m := NewModel(h)
	newM, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	model := newM.(Model)

	// Initially, both events are visible
	if len(model.filteredEvents) != 2 {
		t.Fatalf("expected 2 filteredEvents initially, got %d", len(model.filteredEvents))
	}

	// Switch to PaneDevices and select first device (garden1)
	model.activePane = PaneDevices
	model.deviceIndex = 0

	// View should contain garden1 and active badge together on the row
	view := model.View()
	if !strings.Contains(view, "garden1") || !strings.Contains(view, "Active") {
		t.Errorf("devices pane missing garden1 or Active badge: %s", view)
	}

	// Press Enter to filter by garden1
	newM, _ = model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = newM.(Model)

	if model.selectedCamFilter == "" {
		t.Fatal("expected selectedCamFilter to be set after Enter on device")
	}
	if len(model.filteredEvents) != 1 {
		t.Fatalf("expected 1 filtered event for garden1, got %d", len(model.filteredEvents))
	}
	if model.filteredEvents[0].CameraName != "garden1" {
		t.Errorf("expected filtered event for garden1, got %s", model.filteredEvents[0].CameraName)
	}

	// Verify view shows filter indicators
	filteredView := model.View()
	if !strings.Contains(filteredView, "Filter: garden1") {
		t.Errorf("header missing Filter badge for garden1: %s", filteredView)
	}
	if !strings.Contains(filteredView, "LIVE FEED [garden1]") {
		t.Errorf("live feed title missing [garden1] indicator: %s", filteredView)
	}
	if !strings.Contains(filteredView, "[FILTER]") {
		t.Errorf("devices list missing [FILTER] tag on selected device: %s", filteredView)
	}

	// Press Esc to clear filter and return to all cameras
	newM, _ = model.Update(tea.KeyMsg{Type: tea.KeyEsc})
	model = newM.(Model)

	if model.selectedCamFilter != "" {
		t.Errorf("expected selectedCamFilter to be cleared after Esc, got %s", model.selectedCamFilter)
	}
	if len(model.filteredEvents) != 2 {
		t.Fatalf("expected 2 filteredEvents after Esc, got %d", len(model.filteredEvents))
	}

	// Press Enter again on garden1 to re-enable filter
	newM, _ = model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = newM.(Model)
	if model.selectedCamFilter == "" {
		t.Fatal("expected selectedCamFilter to be set again")
	}

	// Press Enter a second time on garden1 to toggle filter off
	newM, _ = model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = newM.(Model)
	if model.selectedCamFilter != "" {
		t.Errorf("expected selectedCamFilter to be toggled off after second Enter, got %s", model.selectedCamFilter)
	}
	if len(model.filteredEvents) != 2 {
		t.Fatalf("expected 2 filteredEvents after toggling off filter, got %d", len(model.filteredEvents))
	}
}

func TestTUIDevicesPaneRowLayoutAndAlignment(t *testing.T) {
	h := hub.NewHub(100)
	h.UpdateDevice(hub.DeviceStatus{Endpoint: "urn:uuid:cam-1", IP: "127.0.0.1", Port: 8000, Model: "ONVIF-Go-SRE-Enterprise-Fleet", State: "Active"})
	h.UpdateDevice(hub.DeviceStatus{Endpoint: "urn:uuid:cam-2", IP: "192.168.1.113", Port: 2020, Name: "garden1", Model: "Tapo C720", State: "Active"})
	h.UpdateDevice(hub.DeviceStatus{Endpoint: "urn:uuid:cam-3", IP: "192.168.1.160", Port: 2020, Name: "camera1", Model: "Tapo C216", State: "Active"})
	h.UpdateDevice(hub.DeviceStatus{Endpoint: "urn:uuid:cam-4", IP: "192.168.1.12", Port: 2020, Name: "tort1", Model: "Tapo C216", State: "Active"})

	m := NewModel(h)
	newM, _ := m.Update(tea.WindowSizeMsg{Width: 184, Height: 30})
	model := newM.(Model)
	model.activePane = PaneDevices
	model.deviceIndex = 1 // garden1

	view := model.View()

	// Verify that each camera row displays the name and status on the exact same line without wrapping
	lines := strings.Split(view, "\n")
	for _, l := range lines {
		if strings.Contains(l, "ONVIF-Go-SRE-Enterprise-Fleet") && !strings.Contains(l, "Active") {
			t.Errorf("line with ONVIF camera model wrapped status to next line: %s", l)
		}
		if strings.Contains(l, "garden1") && !strings.Contains(l, "Active") {
			t.Errorf("line with garden1 wrapped status to next line: %s", l)
		}
		if strings.Contains(l, "camera1") && !strings.Contains(l, "Active") {
			t.Errorf("line with camera1 wrapped status to next line: %s", l)
		}
		if strings.Contains(l, "tort1") && !strings.Contains(l, "Active") {
			t.Errorf("line with tort1 wrapped status to next line: %s", l)
		}
		// Also ensure 'Active' is never alone on an orphaned line
		trimmed := strings.Trim(l, " │")
		if trimmed == "Active" {
			t.Errorf("found orphaned Active on line: %s", l)
		}
	}
}

func TestTUIAboutModal(t *testing.T) {
	h := hub.NewHub(100)
	m := NewModel(h, Options{
		Version: "1.2.3",
	})

	newM, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	model := newM.(Model)

	// Normal view should have ?:About in help
	view := model.View()
	if !strings.Contains(view, "?:About") {
		t.Errorf("help bar missing '?:About': %s", view)
	}

	// Press '?' to open About modal
	newM, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'?'}})
	model = newM.(Model)

	if !model.isAbout {
		t.Fatal("expected isAbout to be true after '?'")
	}

	aboutView := model.View()
	if !strings.Contains(aboutView, "ABOUT ONVIF-INSPECTOR") {
		t.Errorf("about modal missing title: %s", aboutView)
	}
	if !strings.Contains(aboutView, "onvif-inspector") {
		t.Errorf("about modal missing app name: %s", aboutView)
	}
	if !strings.Contains(aboutView, "v1.2.3") {
		t.Errorf("about modal missing version v1.2.3: %s", aboutView)
	}
	if !strings.Contains(aboutView, "http://stephenford.org/onvif-inspector/") {
		t.Errorf("about modal missing project site: %s", aboutView)
	}
	if !strings.Contains(aboutView, "https://github.com/smford/onvif-inspector") {
		t.Errorf("about modal missing github repo: %s", aboutView)
	}
	if !strings.Contains(aboutView, "Copyright (c) 2026 Stephen Ford") {
		t.Errorf("about modal missing copyright: %s", aboutView)
	}
	if !strings.Contains(aboutView, "MIT License") {
		t.Errorf("about modal missing license: %s", aboutView)
	}

	// Press Esc to close
	newM, _ = model.Update(tea.KeyMsg{Type: tea.KeyEsc})
	model = newM.(Model)

	if model.isAbout {
		t.Error("expected isAbout to be false after Esc")
	}

	// Test 'i' key also opens About modal
	newM, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'i'}})
	model = newM.(Model)
	if !model.isAbout {
		t.Error("expected isAbout to be true after 'i'")
	}

	// Test Enter closes About modal
	newM, _ = model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = newM.(Model)
	if model.isAbout {
		t.Error("expected isAbout to be false after Enter")
	}
}
