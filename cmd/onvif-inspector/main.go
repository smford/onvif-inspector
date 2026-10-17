package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/smford/onvif-inspector/internal/config"
	"github.com/smford/onvif-inspector/internal/discovery"
	"github.com/smford/onvif-inspector/internal/hub"
	"github.com/smford/onvif-inspector/internal/onvif"
	"github.com/smford/onvif-inspector/internal/tui"
	"github.com/smford/onvif-inspector/internal/web"
)

var (
	version = "1.0.0"
	commit  = "none"
	date    = "unknown"
)

type cameraWorker struct {
	dev    discovery.DiscoveredDevice
	client *onvif.Client
	subMgr *onvif.SubscriptionManager
	hub    *hub.Hub
	debug  bool
	logger *log.Logger
}

type appCoordinator struct {
	hub          *hub.Hub
	cfg          *config.Config
	cliCreds     onvif.SecurityCredentials
	cliPassSet   bool
	debug        bool
	logger       *log.Logger
	pushReceiver *onvif.PushReceiver
	mu           sync.Mutex
	workers      map[string]*cameraWorker
	ctx          context.Context
	cancel       context.CancelFunc
}

func newCoordinator(ctx context.Context, h *hub.Hub, cfg *config.Config, creds onvif.SecurityCredentials, cliPassSet bool, debug bool, webhookPort int) (*appCoordinator, context.Context) {
	appCtx, cancel := context.WithCancel(ctx)
	logger := log.New(os.Stderr, "[ONVIF-INSPECTOR] ", log.LstdFlags)

	var pr *onvif.PushReceiver
	pr, err := onvif.NewPushReceiver(webhookPort)
	if err != nil {
		if debug {
			logger.Printf("Warning: failed starting push webhook receiver: %v", err)
		}
	} else if debug {
		logger.Printf("Push notification webhook receiver active on port %d", pr.Port())
	}

	return &appCoordinator{
		hub:          h,
		cfg:          cfg,
		cliCreds:     creds,
		cliPassSet:   cliPassSet,
		debug:        debug,
		logger:       logger,
		pushReceiver: pr,
		workers:      make(map[string]*cameraWorker),
		ctx:          appCtx,
		cancel:       cancel,
	}, appCtx
}

func (c *appCoordinator) resolveCredentials(dev discovery.DiscoveredDevice) onvif.SecurityCredentials {
	// 1. Explicit CLI password flag takes precedence
	if c.cliPassSet {
		return c.cliCreds
	}

	// 2. Check config.yaml (per-camera or default)
	if c.cfg != nil {
		if u, p, matched := c.cfg.GetCredentials(dev.IP, dev.Port, dev.Endpoint, dev.PrimaryXAddr); matched {
			return onvif.SecurityCredentials{
				Username: u,
				Password: p,
			}
		}
	}

	// 3. Fallback to CLI defaults (e.g. username "admin", empty password)
	return c.cliCreds
}

func (c *appCoordinator) AttachCamera(dev discovery.DiscoveredDevice) {
	key := dev.Endpoint
	if key == "" {
		if dev.Port > 0 {
			key = fmt.Sprintf("%s:%d", dev.IP, dev.Port)
		} else {
			key = dev.IP
		}
	}

	c.mu.Lock()
	if _, exists := c.workers[key]; exists {
		c.mu.Unlock()
		return
	}
	c.mu.Unlock()

	if c.cfg != nil {
		if customName := c.cfg.GetCameraName(dev.IP, dev.Port, dev.Endpoint, dev.PrimaryXAddr); customName != "" {
			dev.Name = customName
		}
	}

	// Initial registration in hub
	c.hub.UpdateDevice(hub.DeviceStatus{
		Endpoint:    dev.Endpoint,
		IP:          dev.IP,
		Port:        dev.Port,
		Name:        dev.Name,
		Model:       dev.Model,
		State:       string(onvif.StateReconnecting),
		StateReason: "Connecting...",
		PingLatency: dev.PingLatency,
	})

	go c.runCameraWorker(key, dev)
}

func (c *appCoordinator) runCameraWorker(key string, dev discovery.DiscoveredDevice) {
	deviceURL := dev.PrimaryXAddr
	if deviceURL == "" {
		deviceURL = fmt.Sprintf("http://%s:%d/onvif/device_service", dev.IP, dev.Port)
	}

	client, err := onvif.NewClient(onvif.ClientOptions{
		DeviceURL:   deviceURL,
		Credentials: c.resolveCredentials(dev),
		HTTPTimeout: 15 * time.Second,
		Debug:       c.debug,
		DebugLogger: c.logger,
	})
	if err != nil {
		c.hub.UpdateDevice(hub.DeviceStatus{
			Endpoint:    dev.Endpoint,
			IP:          dev.IP,
			Port:        dev.Port,
			State:       string(onvif.StateFailed),
			StateReason: fmt.Sprintf("Init failed: %v", err),
		})
		return
	}

	// Step 1: Clock skew synchronization
	driftCtx, cancelDrift := context.WithTimeout(c.ctx, 10*time.Second)
	drift, err := client.SyncSystemDateAndTime(driftCtx)
	cancelDrift()
	if err != nil && c.debug {
		c.logger.Printf("[%s] Clock sync warning: %v (continuing with local time)", dev.IP, err)
	} else if c.debug {
		c.logger.Printf("[%s] Clock synchronized. Drift: %v", dev.IP, drift)
	}

	// Step 2: Fetch device information
	infoCtx, cancelInfo := context.WithTimeout(c.ctx, 10*time.Second)
	info, err := client.GetDeviceInformation(infoCtx)
	cancelInfo()
	if err == nil && info != nil {
		dev.Manufacturer = info.Manufacturer
		dev.Model = info.Model
	}
	if c.cfg != nil {
		if customName := c.cfg.GetCameraName(dev.IP, dev.Port, dev.Endpoint, dev.PrimaryXAddr); customName != "" {
			dev.Name = customName
		}
	}

	// Step 3: Resolve capabilities and Events service XAddr
	capCtx, cancelCap := context.WithTimeout(c.ctx, 10*time.Second)
	_, _ = client.GetCapabilities(capCtx)
	cancelCap()

	// Handler bridge to Hub
	handler := &cameraEventHandler{
		camID: key,
		camIP: dev.IP,
		dev:   dev,
		hub:   c.hub,
		info:  info,
	}

	subMgr := onvif.NewSubscriptionManager(client, handler)
	if c.pushReceiver != nil {
		subMgr.EnablePushFallback(c.pushReceiver, key, dev.IP)
	}

	worker := &cameraWorker{
		dev:    dev,
		client: client,
		subMgr: subMgr,
		hub:    c.hub,
		debug:  c.debug,
		logger: c.logger,
	}

	c.mu.Lock()
	c.workers[key] = worker
	c.mu.Unlock()

	// Update device with rich metadata
	c.hub.UpdateDevice(hub.DeviceStatus{
		Endpoint:        dev.Endpoint,
		IP:              dev.IP,
		Port:            dev.Port,
		Name:            dev.Name,
		Manufacturer:    dev.Manufacturer,
		Model:           dev.Model,
		FirmwareVersion: getFirmware(info),
		SerialNumber:    getSerial(info),
		State:           string(onvif.StateActive),
		StateReason:     "Subscription running",
		PingLatency:     dev.PingLatency,
	})

	// Start long-polling PullPoint subscription
	subMgr.Start(c.ctx)
}

func (c *appCoordinator) RestartWorkerWithCredentials(targetOrIP, username, password string) {
	c.mu.Lock()
	var matchedWorker *cameraWorker
	var matchedKey string

	targetOrIP = strings.TrimSpace(targetOrIP)

	for k, w := range c.workers {
		if targetOrIP == "" {
			// If target is empty, we updated default credentials; restart failed or reconnecting workers
			if w.subMgr.GetState() == onvif.StateFailed || w.subMgr.GetState() == onvif.StateReconnecting {
				go c.restartWorker(k, w)
			}
			continue
		}

		if strings.EqualFold(w.dev.IP, targetOrIP) ||
			strings.EqualFold(fmt.Sprintf("%s:%d", w.dev.IP, w.dev.Port), targetOrIP) ||
			strings.EqualFold(w.dev.Endpoint, targetOrIP) ||
			strings.Contains(w.dev.PrimaryXAddr, targetOrIP) {
			matchedWorker = w
			matchedKey = k
			break
		}
	}
	c.mu.Unlock()

	if matchedWorker != nil {
		if c.cfg != nil {
			if customName := c.cfg.GetCameraName(matchedWorker.dev.IP, matchedWorker.dev.Port, matchedWorker.dev.Endpoint, matchedWorker.dev.PrimaryXAddr); customName != "" {
				matchedWorker.dev.Name = customName
				c.hub.UpdateDevice(hub.DeviceStatus{
					Endpoint: matchedWorker.dev.Endpoint,
					IP:       matchedWorker.dev.IP,
					Port:     matchedWorker.dev.Port,
					Name:     matchedWorker.dev.Name,
				})
			}
		}
		c.restartWorker(matchedKey, matchedWorker)
		return
	}

	// If no active worker matched and targetOrIP is provided, try direct discovery and attach
	if targetOrIP != "" {
		go func() {
			devs, err := discovery.Discover(c.ctx, discovery.ProbeOptions{
				Target:  targetOrIP,
				Timeout: 3 * time.Second,
			})
			if err == nil && len(devs) > 0 {
				c.AttachCamera(devs[0])
			}
		}()
	}
}

func (c *appCoordinator) restartWorker(key string, w *cameraWorker) {
	c.mu.Lock()
	delete(c.workers, key)
	c.mu.Unlock()

	// Stop old subscription cleanly
	w.subMgr.Stop()

	// Re-run worker with updated credentials
	go c.runCameraWorker(key, w.dev)
}

func (c *appCoordinator) StopAll() {
	c.cancel()

	c.mu.Lock()
	workers := make([]*cameraWorker, 0, len(c.workers))
	for _, w := range c.workers {
		workers = append(workers, w)
	}
	c.workers = make(map[string]*cameraWorker)
	c.mu.Unlock()

	var wg sync.WaitGroup
	for _, w := range workers {
		wg.Add(1)
		go func(worker *cameraWorker) {
			defer wg.Done()
			worker.subMgr.Stop()
		}(w)
	}
	wg.Wait()
	if c.pushReceiver != nil {
		_ = c.pushReceiver.Close()
	}
}

type cameraEventHandler struct {
	camID string
	camIP string
	dev   discovery.DiscoveredDevice
	hub   *hub.Hub
	info  *onvif.DeviceInformationResponse
}

func (h *cameraEventHandler) OnStatusChange(state onvif.SubscriptionState, reason string) {
	h.hub.UpdateDevice(hub.DeviceStatus{
		Endpoint:        h.dev.Endpoint,
		IP:              h.dev.IP,
		Port:            h.dev.Port,
		Name:            h.dev.Name,
		Manufacturer:    h.dev.Manufacturer,
		Model:           h.dev.Model,
		FirmwareVersion: getFirmware(h.info),
		SerialNumber:    getSerial(h.info),
		State:           string(state),
		StateReason:     reason,
		PingLatency:     h.dev.PingLatency,
	})
}

func (h *cameraEventHandler) OnNotification(msg onvif.NotificationMessage, rawRequest, rawResponse string) {
	ev := hub.NormalizeEvent(h.camID, h.camIP, msg, rawRequest, rawResponse)
	ev.CameraName = h.dev.Name
	h.hub.Publish(ev)
}

func getFirmware(info *onvif.DeviceInformationResponse) string {
	if info != nil {
		return info.FirmwareVersion
	}
	return ""
}

func getSerial(info *onvif.DeviceInformationResponse) string {
	if info != nil {
		return info.SerialNumber
	}
	return ""
}

func main() {
	if len(os.Args) > 1 && os.Args[1] == "config" {
		handleConfigCommand(os.Args[2:])
		return
	}

	var (
		configFlag     string
		ifaceFlag      string
		targetFlag     string
		userFlag       string
		passFlag       string
		saveConfigFlag bool
		webFlag        bool
		webPortFlag    int
		headlessFlag   bool
		formatFlag     string
		debugFlag      bool
		versionFlag    bool
	)

	flag.StringVar(&configFlag, "config", "", "Path to YAML configuration file (default: config.yaml if present)")
	flag.StringVar(&configFlag, "c", "", "Path to YAML configuration file (shorthand)")

	flag.StringVar(&ifaceFlag, "interface", "", "Network interface for WS-Discovery (e.g. eth0, en0)")
	flag.StringVar(&ifaceFlag, "i", "", "Network interface for WS-Discovery (shorthand)")

	flag.StringVar(&targetFlag, "target", "", "Direct camera address bypass discovery (e.g. 192.168.1.100:80)")
	flag.StringVar(&targetFlag, "t", "", "Direct camera address bypass discovery (shorthand)")

	flag.StringVar(&userFlag, "username", "admin", "ONVIF username")
	flag.StringVar(&userFlag, "u", "admin", "ONVIF username (shorthand)")

	flag.StringVar(&passFlag, "password", "", "ONVIF password")
	flag.StringVar(&passFlag, "p", "", "ONVIF password (shorthand)")

	flag.BoolVar(&saveConfigFlag, "save-config", false, "Save specified target/credentials to config.yaml on launch")

	flag.BoolVar(&webFlag, "web", false, "Enable embedded web dashboard concurrently with TUI")
	flag.IntVar(&webPortFlag, "web-port", 8080, "Port for embedded web dashboard")
	var webhookPortFlag int
	flag.IntVar(&webhookPortFlag, "webhook-port", 0, "Local port for ONVIF push webhook fallback (0 = automatic free port)")

	flag.BoolVar(&headlessFlag, "headless", false, "Run without TUI (stdout streaming or web-only mode)")
	flag.StringVar(&formatFlag, "format", "text", "Output format for headless stdout: text, json")

	flag.BoolVar(&debugFlag, "debug", false, "Log raw SOAP requests and responses to stderr")
	flag.BoolVar(&versionFlag, "v", false, "Print version and exit")

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "onvif-inspector %s (commit: %s, date: %s)\n", version, commit, date)
		fmt.Fprintf(os.Stderr, "ONVIF IP Camera Discovery & Event Diagnostic Utility\n\nUsage:\n")
		flag.PrintDefaults()
		fmt.Fprintf(os.Stderr, "\nCommands:\n  config    Manage camera credentials and settings in config.yaml\n")
	}

	flag.Parse()

	if versionFlag {
		fmt.Printf("onvif-inspector version %s (commit: %s, date: %s)\n", version, commit, date)
		os.Exit(0)
	}

	cfg, err := config.LoadConfig(configFlag)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading configuration: %v\n", err)
		os.Exit(1)
	}

	// If --save-config requested on launch, persist CLI flags into config.yaml
	if saveConfigFlag {
		if targetFlag != "" {
			_ = cfg.SetCameraCredentials(config.CameraConfig{
				Target:   targetFlag,
				Username: userFlag,
				Password: passFlag,
			})
			fmt.Printf("Saved camera %q credentials to %s\n", targetFlag, cfg.FilePath())
		} else if passFlag != "" {
			_ = cfg.SetDefaultCredentials(userFlag, passFlag)
			fmt.Printf("Saved default credentials to %s\n", cfg.FilePath())
		}
	}

	// Apply config fallbacks for CLI flags if not explicitly set
	if targetFlag == "" && cfg.Target != "" {
		targetFlag = cfg.Target
	}
	if ifaceFlag == "" && cfg.Interface != "" {
		ifaceFlag = cfg.Interface
	}
	if webPortFlag == 8080 && cfg.WebPort > 0 {
		webPortFlag = cfg.WebPort
	}

	cliPassSet := passFlag != ""

	// If CLI user/password was provided, ensure cfg default reflects it if config.yaml had no default set
	defU, _ := cfg.GetDefaultCredentials()
	if defU == "" && userFlag != "" {
		cfg.Username = userFlag
		cfg.Password = passFlag
	}

	eventHub := hub.NewHub(1000)
	creds := onvif.SecurityCredentials{
		Username: userFlag,
		Password: passFlag,
	}

	rootCtx, rootCancel := context.WithCancel(context.Background())
	defer rootCancel()

	coord, appCtx := newCoordinator(rootCtx, eventHub, cfg, creds, cliPassSet, debugFlag, webhookPortFlag)

	// Intercept signals for clean unsubscriptions
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	go func() {
		<-sigChan
		if debugFlag {
			fmt.Fprintf(os.Stderr, "\nReceived shutdown signal. Draining PullPoint subscriptions...\n")
		}
		coord.StopAll()
		rootCancel()
	}()

	// Start web dashboard if requested
	var webServer *web.Server
	if webFlag {
		webServer = web.NewServer(web.Config{
			Hub:       eventHub,
			AppConfig: cfg,
			OnConfigUpdate: func(target, username, password string) {
				coord.RestartWorkerWithCredentials(target, username, password)
			},
			Port: webPortFlag,
		})
		if err := webServer.Start(); err != nil {
			fmt.Fprintf(os.Stderr, "Failed to start web dashboard: %v\n", err)
		} else {
			fmt.Fprintf(os.Stderr, "Embedded Web Dashboard running at: http://localhost:%d\n", webPortFlag)
		}
	}

	// Trigger camera discovery in background
	go func() {
		probeOpts := discovery.ProbeOptions{
			InterfaceName: ifaceFlag,
			Target:        targetFlag,
			Timeout:       3 * time.Second,
		}

		// Initial scan
		devices, err := discovery.Discover(appCtx, probeOpts)
		if err == nil {
			for _, dev := range devices {
				coord.AttachCamera(dev)
			}
		} else if debugFlag {
			fmt.Fprintf(os.Stderr, "Discovery probe warning: %v\n", err)
		}

		// If not a single target, periodically rescan to pick up newly connected cameras
		if targetFlag == "" {
			ticker := time.NewTicker(30 * time.Second)
			defer ticker.Stop()
			for {
				select {
				case <-appCtx.Done():
					return
				case <-ticker.C:
					devs, err := discovery.Discover(appCtx, probeOpts)
					if err == nil {
						for _, dev := range devs {
							coord.AttachCamera(dev)
						}
					}
				}
			}
		}
	}()

	// Mode Selection: Headless vs Interactive TUI
	if headlessFlag {
		runHeadless(appCtx, eventHub, formatFlag, webServer)
	} else {
		runTUI(eventHub, cfg, func(target, username, password string) {
			coord.RestartWorkerWithCredentials(target, username, password)
		})
		// When TUI exits, ensure clean coordinator stop
		coord.StopAll()
		if webServer != nil {
			_ = webServer.Stop(context.Background())
		}
	}
}

func isStdoutStreamingOnly(headless bool, format string, webEnabled bool) bool {
	return headless && !webEnabled && format == "json"
}

func runHeadless(ctx context.Context, h *hub.Hub, format string, webServer *web.Server) {
	evCh, unsub := h.SubscribeEvents(500)
	defer unsub()

	formatJSON := strings.ToLower(format) == "json"

	for {
		select {
		case <-ctx.Done():
			if webServer != nil {
				_ = webServer.Stop(context.Background())
			}
			return
		case ev, ok := <-evCh:
			if !ok {
				return
			}
			if formatJSON {
				data, err := json.Marshal(ev)
				if err == nil {
					fmt.Println(string(data))
				}
			} else {
				ts := ev.Timestamp.Format("2006-01-02 15:04:05")
				fmt.Printf("[%s] [%-15s] [%-6s] %s (State=%s)\n",
					ts, ev.CameraIP, ev.Category, ev.Topic, ev.PropertyState)
			}
		}
	}
}

func runTUI(h *hub.Hub, cfg *config.Config, onConfigUpdate func(target, username, password string)) {
	model := tui.NewModel(h, tui.Options{
		AppConfig:      cfg,
		OnConfigUpdate: onConfigUpdate,
	})
	p := tea.NewProgram(model, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error running TUI: %v\n", err)
	}
}

func handleConfigCommand(args []string) {
	if len(args) == 0 || args[0] == "help" || args[0] == "-h" || args[0] == "--help" {
		printConfigUsage()
		return
	}

	sub := args[0]
	subArgs := args[1:]

	switch sub {
	case "list":
		var cfgPath string
		fs := flag.NewFlagSet("config list", flag.ExitOnError)
		fs.StringVar(&cfgPath, "config", "", "Path to YAML configuration file")
		fs.StringVar(&cfgPath, "c", "", "Path to YAML configuration file (shorthand)")
		_ = fs.Parse(subArgs)

		cfg, err := config.LoadOrCreateConfig(cfgPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}

		fmt.Printf("Configuration File: %s\n\n", cfg.FilePath())
		du, dp := cfg.GetDefaultCredentials()
		fmt.Printf("Default Credentials:\n  Username: %s\n  Password: %s\n\n", du, maskPassword(dp))

		cams := cfg.ListCameras()
		fmt.Printf("Configured Cameras (%d):\n", len(cams))
		if len(cams) == 0 {
			fmt.Println("  (No specific cameras configured)")
		} else {
			for i, cam := range cams {
				ident := cam.Target
				if ident == "" {
					ident = cam.IP
				}
				if ident == "" {
					ident = cam.Endpoint
				}
				fmt.Printf("  [%d] Target:   %s\n", i+1, ident)
				if cam.Name != "" {
					fmt.Printf("      Name:     %s\n", cam.Name)
				}
				fmt.Printf("      Username: %s\n", cam.Username)
				fmt.Printf("      Password: %s\n", maskPassword(cam.Password))
				if cam.Endpoint != "" && cam.Endpoint != ident {
					fmt.Printf("      Endpoint: %s\n", cam.Endpoint)
				}
				fmt.Println()
			}
		}

	case "set-default":
		var cfgPath, user, pass string
		fs := flag.NewFlagSet("config set-default", flag.ExitOnError)
		fs.StringVar(&user, "username", "admin", "Default username")
		fs.StringVar(&user, "u", "admin", "Default username (shorthand)")
		fs.StringVar(&pass, "password", "", "Default password")
		fs.StringVar(&pass, "p", "", "Default password (shorthand)")
		fs.StringVar(&cfgPath, "config", "", "Path to YAML configuration file")
		fs.StringVar(&cfgPath, "c", "", "Path to YAML configuration file (shorthand)")
		_ = fs.Parse(subArgs)

		cfg, err := config.LoadOrCreateConfig(cfgPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		if cfgPath != "" {
			cfg.SetFilePath(cfgPath)
		}

		if err := cfg.SetDefaultCredentials(user, pass); err != nil {
			fmt.Fprintf(os.Stderr, "Error saving default credentials: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("Successfully saved default credentials (user: %s) to %s\n", user, cfg.FilePath())

	case "set":
		var cfgPath, name, target, ip, endpoint, user, pass string
		fs := flag.NewFlagSet("config set", flag.ExitOnError)
		fs.StringVar(&name, "name", "", "Friendly camera name/alias (e.g. Garden)")
		fs.StringVar(&name, "n", "", "Friendly camera name/alias (shorthand)")
		fs.StringVar(&target, "target", "", "Camera target address (e.g. 192.168.1.100:80)")
		fs.StringVar(&target, "t", "", "Camera target address (shorthand)")
		fs.StringVar(&ip, "ip", "", "Camera IP address")
		fs.StringVar(&endpoint, "endpoint", "", "Camera ONVIF endpoint UUID")
		fs.StringVar(&user, "username", "admin", "Camera username")
		fs.StringVar(&user, "u", "admin", "Camera username (shorthand)")
		fs.StringVar(&pass, "password", "", "Camera password")
		fs.StringVar(&pass, "p", "", "Camera password (shorthand)")
		fs.StringVar(&cfgPath, "config", "", "Path to YAML configuration file")
		fs.StringVar(&cfgPath, "c", "", "Path to YAML configuration file (shorthand)")
		_ = fs.Parse(subArgs)

		if target == "" && ip == "" && endpoint == "" {
			fmt.Fprintf(os.Stderr, "Error: please specify -t/--target or --ip for the camera\n")
			os.Exit(1)
		}

		cfg, err := config.LoadOrCreateConfig(cfgPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		if cfgPath != "" {
			cfg.SetFilePath(cfgPath)
		}

		camCfg := config.CameraConfig{
			Name:     name,
			Target:   target,
			IP:       ip,
			Endpoint: endpoint,
			Username: user,
			Password: pass,
		}
		if err := cfg.SetCameraCredentials(camCfg); err != nil {
			fmt.Fprintf(os.Stderr, "Error saving camera configuration: %v\n", err)
			os.Exit(1)
		}

		ident := target
		if ident == "" {
			ident = ip
		}
		if ident == "" {
			ident = endpoint
		}
		displayName := ident
		if name != "" {
			displayName = fmt.Sprintf("%s (%s)", name, ident)
		}
		fmt.Printf("Successfully configured camera %q (user: %s) in %s\n", displayName, user, cfg.FilePath())

	case "remove":
		var cfgPath, target string
		fs := flag.NewFlagSet("config remove", flag.ExitOnError)
		fs.StringVar(&target, "target", "", "Camera target, IP, or endpoint to remove")
		fs.StringVar(&target, "t", "", "Camera target, IP, or endpoint to remove (shorthand)")
		fs.StringVar(&cfgPath, "config", "", "Path to YAML configuration file")
		fs.StringVar(&cfgPath, "c", "", "Path to YAML configuration file (shorthand)")
		_ = fs.Parse(subArgs)

		if target == "" {
			fmt.Fprintf(os.Stderr, "Error: please specify -t/--target to remove\n")
			os.Exit(1)
		}

		cfg, err := config.LoadOrCreateConfig(cfgPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		if cfgPath != "" {
			cfg.SetFilePath(cfgPath)
		}

		removed, err := cfg.RemoveCamera(target)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error removing camera: %v\n", err)
			os.Exit(1)
		}
		if removed {
			fmt.Printf("Removed camera %q from %s\n", target, cfg.FilePath())
		} else {
			fmt.Printf("Camera %q was not found in %s\n", target, cfg.FilePath())
		}

	default:
		fmt.Fprintf(os.Stderr, "Unknown config subcommand %q\n\n", sub)
		printConfigUsage()
		os.Exit(1)
	}
}

func printConfigUsage() {
	fmt.Printf(`onvif-inspector config - Manage camera credentials and config.yaml settings

Usage:
  onvif-inspector config list [-c <path>]
      View configured cameras and default credentials

  onvif-inspector config set -t <target|ip> [-n <name>] -u <username> -p <password> [-c <path>]
      Add or update camera name and credentials in config.yaml

  onvif-inspector config set-default -u <username> -p <password> [-c <path>]
      Set default camera credentials in config.yaml

  onvif-inspector config remove -t <target|ip> [-c <path>]
      Remove a camera configuration from config.yaml
`)
}

func maskPassword(p string) string {
	if p == "" {
		return "(none)"
	}
	return "******** (configured)"
}
