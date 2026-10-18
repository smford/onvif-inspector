package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/smford/onvif-inspector/internal/config"
	"github.com/smford/onvif-inspector/internal/hub"
	"github.com/smford/onvif-inspector/internal/tui"
	"github.com/smford/onvif-inspector/internal/web"
)

func main() {
	h := hub.NewHub(500)

	// Populate realistic cameras
	cameras := []hub.DeviceStatus{
		{
			Endpoint:        "uuid:3fa1fe68-b915-4053-a3e1-6c4cbc5d0b0e",
			IP:              "192.168.1.160",
			Port:            2020,
			Name:            "camera1",
			Manufacturer:    "tp-link",
			Model:           "Tapo C216",
			FirmwareVersion: "1.3.2 Build 260820",
			SerialNumber:    "74610b0e",
			State:           "Active",
			StateReason:     "Subscription active (PullPoint)",
			PingLatency:     18 * time.Millisecond,
			EventCount:      42,
			LastSeen:        time.Now().UTC(),
		},
		{
			Endpoint:        "uuid:3fa1fe68-b915-4053-a3e1-7cf17e4ca028",
			IP:              "192.168.1.113",
			Port:            2020,
			Name:            "garden1",
			Manufacturer:    "tp-link",
			Model:           "Tapo C720",
			FirmwareVersion: "1.5.2 Build 260528",
			SerialNumber:    "7461a028",
			State:           "Active",
			StateReason:     "Subscription active (Push webhook)",
			PingLatency:     24 * time.Millisecond,
			EventCount:      18,
			LastSeen:        time.Now().UTC(),
		},
		{
			Endpoint:        "uuid:3fa1fe68-b915-4053-a3e1-6c4cbc5cf923",
			IP:              "192.168.1.12",
			Port:            2020,
			Name:            "tort1",
			Manufacturer:    "tp-link",
			Model:           "Tapo C216",
			FirmwareVersion: "1.3.2 Build 260820",
			SerialNumber:    "7461f923",
			State:           "Active",
			StateReason:     "Subscription active (PullPoint)",
			PingLatency:     22 * time.Millisecond,
			EventCount:      7,
			LastSeen:        time.Now().UTC(),
		},
		{
			Endpoint:        "uuid:hik-001-driveway",
			IP:              "192.168.1.105",
			Port:            80,
			Name:            "driveway-cam",
			Manufacturer:    "Hikvision",
			Model:           "DS-2CD2386G2-ISU/SL",
			FirmwareVersion: "V5.7.13",
			SerialNumber:    "HIK998241",
			State:           "Active",
			StateReason:     "Subscription active (PullPoint)",
			PingLatency:     4 * time.Millisecond,
			EventCount:      89,
			LastSeen:        time.Now().UTC(),
		},
		{
			Endpoint:        "uuid:dahua-002-door",
			IP:              "192.168.1.108",
			Port:            80,
			Name:            "front-door",
			Manufacturer:    "Dahua",
			Model:           "IPC-HDW5849H-ASE-LED",
			FirmwareVersion: "V2.840",
			SerialNumber:    "DAH441209",
			State:           "Active",
			StateReason:     "Subscription active (PullPoint)",
			PingLatency:     3 * time.Millisecond,
			EventCount:      124,
			LastSeen:        time.Now().UTC(),
		},
	}

	for _, cam := range cameras {
		h.UpdateDevice(cam)
	}

	baseTime := time.Now().UTC().Add(-10 * time.Minute)

	// Publish realistic events
	rawSOAP := `<?xml version="1.0" encoding="UTF-8"?>
<SOAP-ENV:Envelope xmlns:SOAP-ENV="http://www.w3.org/2003/05/soap-envelope"
  xmlns:wsa="http://www.w3.org/2005/08/addressing"
  xmlns:wsnt="http://docs.oasis-open.org/wsn/b-2"
  xmlns:tt="http://www.onvif.org/ver10/schema"
  xmlns:tns1="http://www.onvif.org/ver10/topics">
  <SOAP-ENV:Body>
    <wsnt:NotificationMessage>
      <wsnt:Topic Dialect="http://www.onvif.org/ver10/tev/topicExpression/ConcreteSet">
        tns1:RuleEngine/PeopleDetector/People
      </wsnt:Topic>
      <wsnt:Message>
        <tt:Message UtcTime="2026-10-17T23:58:00Z" PropertyOperation="Changed">
          <tt:Source>
            <tt:SimpleItem Name="VideoSourceConfigurationToken" Value="vsconf"/>
            <tt:SimpleItem Name="Rule" Value="MyPeopleDetectorRule"/>
          </tt:Source>
          <tt:Data>
            <tt:SimpleItem Name="IsPeople" Value="true"/>
          </tt:Data>
        </tt:Message>
      </wsnt:Message>
    </wsnt:NotificationMessage>
  </SOAP-ENV:Body>
</SOAP-ENV:Envelope>`

	events := []hub.Event{
		{
			ID:            "evt-001",
			Timestamp:     baseTime.Add(1 * time.Minute),
			CameraID:      "uuid:3fa1fe68-b915-4053-a3e1-6c4cbc5d0b0e",
			CameraIP:      "192.168.1.160",
			CameraName:    "camera1",
			Category:      "Motion",
			Topic:         "tns1:RuleEngine/CellMotionDetector/Motion",
			PropertyState: "IsMotion=true",
			Data:          map[string]string{"IsMotion": "true"},
			Source:        map[string]string{"VideoSourceConfigurationToken": "vsconf"},
			RawXML:        rawSOAP,
		},
		{
			ID:            "evt-002",
			Timestamp:     baseTime.Add(2 * time.Minute),
			CameraID:      "uuid:3fa1fe68-b915-4053-a3e1-7cf17e4ca028",
			CameraIP:      "192.168.1.113",
			CameraName:    "garden1",
			Category:      "Motion",
			Topic:         "tns1:RuleEngine/CellMotionDetector/Motion",
			PropertyState: "IsMotion=true",
			Data:          map[string]string{"IsMotion": "true"},
			Source:        map[string]string{"VideoSourceConfigurationToken": "vsconf_garden"},
			RawXML:        rawSOAP,
		},
		{
			ID:            "evt-003",
			Timestamp:     baseTime.Add(4 * time.Minute),
			CameraID:      "uuid:hik-001-driveway",
			CameraIP:      "192.168.1.105",
			CameraName:    "driveway-cam",
			Category:      "RuleEngine",
			Topic:         "tns1:RuleEngine/LineDetector/Crossed",
			PropertyState: "IsCrossing=true",
			Data:          map[string]string{"IsCrossing": "true", "Direction": "Inbound"},
			Source:        map[string]string{"Rule": "DrivewayPerimeterLine"},
			RawXML:        rawSOAP,
		},
		{
			ID:            "evt-004",
			Timestamp:     baseTime.Add(6 * time.Minute),
			CameraID:      "uuid:dahua-002-door",
			CameraIP:      "192.168.1.108",
			CameraName:    "front-door",
			Category:      "Audio",
			Topic:         "tns1:AudioAnalytics/SoundDetection/Detected",
			PropertyState: "SoundDetected=true",
			Data:          map[string]string{"SoundDetected": "true", "Decibels": "78dB"},
			Source:        map[string]string{"AudioSourceToken": "audio_ch0"},
			RawXML:        rawSOAP,
		},
		{
			ID:            "evt-005",
			Timestamp:     baseTime.Add(8 * time.Minute),
			CameraID:      "uuid:3fa1fe68-b915-4053-a3e1-6c4cbc5d0b0e",
			CameraIP:      "192.168.1.160",
			CameraName:    "camera1",
			Category:      "Motion",
			Topic:         "tns1:RuleEngine/PeopleDetector/People",
			PropertyState: "IsPeople=true",
			Data:          map[string]string{"IsPeople": "true", "Confidence": "94%"},
			Source:        map[string]string{"Rule": "MyPeopleDetectorRule", "VideoSourceConfigurationToken": "vsconf"},
			RawXML:        rawSOAP,
		},
	}

	for _, ev := range events {
		h.Publish(ev)
	}

	// 1. Generate TUI SVG screenshot
	m := tui.NewModel(h)
	newM, _ := m.Update(tea.WindowSizeMsg{Width: 140, Height: 35})
	renderedView := newM.(tui.Model).View()

	// Write raw ANSI output to temporary file
	ansiPath := "/tmp/tui_sample.ansi"
	if err := os.WriteFile(ansiPath, []byte(renderedView), 0644); err != nil {
		log.Fatalf("failed writing ansi: %v", err)
	}

	// Run term2svg
	cmd := exec.Command("/tmp/term2svg",
		"--title", "onvif-inspector - Interactive Terminal UI",
		"--columns", "140",
		"--rows", "35",
		"--theme", "dark",
		"--output", "docs/images/tui-screenshot.svg",
	)
	cmd.Stdin, _ = os.Open(ansiPath)
	if out, err := cmd.CombinedOutput(); err != nil {
		log.Fatalf("term2svg failed: %v, out: %s", err, string(out))
	}
	fmt.Println("Generated docs/images/tui-screenshot.svg")

	// 2. Start Web Server and capture screenshot
	cfg := &config.Config{}
	server := web.NewServer(web.Config{
		Hub:       h,
		Port:      8099,
		AppConfig: cfg,
	})
	go func() {
		if err := server.Start(); err != nil {
			log.Printf("web server err: %v", err)
		}
	}()
	time.Sleep(500 * time.Millisecond)

	// Screenshot Web UI
	chromeCmd := exec.Command(
		"/Applications/Google Chrome.app/Contents/MacOS/Google Chrome",
		"--headless=new",
		"--screenshot=docs/images/web-screenshot.png",
		"--window-size=1440,900",
		"--disable-gpu",
		"--no-sandbox",
		"http://127.0.0.1:8099",
	)
	if out, err := chromeCmd.CombinedOutput(); err != nil {
		log.Printf("chrome web screenshot warning: %v, out: %s", err, string(out))
	} else {
		fmt.Println("Generated docs/images/web-screenshot.png")
	}

	// Render TUI SVG to PNG
	chromeTuiCmd := exec.Command(
		"/Applications/Google Chrome.app/Contents/MacOS/Google Chrome",
		"--headless=new",
		"--screenshot=docs/images/tui-screenshot.png",
		"--window-size=1400,850",
		"--disable-gpu",
		"--no-sandbox",
		fmt.Sprintf("file://%s/docs/images/tui-screenshot.svg", os.Getenv("PWD")),
	)
	if out, err := chromeTuiCmd.CombinedOutput(); err != nil {
		log.Printf("chrome tui screenshot warning: %v, out: %s", err, string(out))
	} else {
		fmt.Println("Generated docs/images/tui-screenshot.png")
	}

	_ = server.Stop(context.Background())
}
