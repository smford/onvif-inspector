package web

import (
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/smford/onvif-inspector/internal/config"
	"github.com/smford/onvif-inspector/internal/hub"
	webassets "github.com/smford/onvif-inspector/web"
)

// ConfigUpdateHandler is called when camera or default credentials are changed via Web
type ConfigUpdateHandler func(target string, username string, password string)

// Server provides the embedded web interface and REST/SSE APIs
type Server struct {
	hub            *hub.Hub
	appConfig      *config.Config
	onConfigUpdate ConfigUpdateHandler
	port           int
	version        string
	broker         *SSEBroker
	httpServer     *http.Server
	listener       net.Listener
	mu             sync.Mutex
}

// Config configures web server options
type Config struct {
	Hub            *hub.Hub
	AppConfig      *config.Config
	OnConfigUpdate ConfigUpdateHandler
	Port           int
	Version        string
}

// AboutInfo represents application metadata
type AboutInfo struct {
	Name       string `json:"name"`
	Version    string `json:"version"`
	DocsURL    string `json:"docs_url"`
	RepoURL    string `json:"repo_url"`
	Copyright  string `json:"copyright"`
	License    string `json:"license"`
	LicenseURL string `json:"license_url"`
}

// NewServer initializes a web server instance
func NewServer(cfg Config) *Server {
	if cfg.Port <= 0 {
		cfg.Port = 8080
	}
	ver := cfg.Version
	if ver == "" {
		ver = "1.0.0"
	}

	broker := NewSSEBroker(cfg.Hub)

	return &Server{
		hub:            cfg.Hub,
		appConfig:      cfg.AppConfig,
		onConfigUpdate: cfg.OnConfigUpdate,
		port:           cfg.Port,
		version:        ver,
		broker:         broker,
	}
}

// Handler returns the http.Handler for routing
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()

	// Healthcheck endpoint
	mux.HandleFunc("/healthz", s.handleHealthz)

	// REST endpoints
	mux.HandleFunc("/api/about", s.handleAbout)
	mux.HandleFunc("/api/devices", s.handleGetDevices)
	mux.HandleFunc("/api/events", s.handleGetEvents)

	// Config endpoints
	mux.HandleFunc("/api/config", s.handleConfig)
	mux.HandleFunc("/api/config/camera", s.handleConfigCamera)
	mux.HandleFunc("/api/config/default", s.handleConfigDefault)

	// SSE endpoint
	mux.Handle("/api/events/stream", s.broker)

	// Static assets from embedded FS
	subFS, err := fs.Sub(webassets.Assets, ".")
	if err == nil {
		mux.Handle("/", http.FileServer(http.FS(subFS)))
	}

	return mux
}

// Start binds to the configured port and starts serving HTTP requests
func (s *Server) Start() error {
	addr := fmt.Sprintf(":%d", s.port)
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("failed to bind web server to %s: %w", addr, err)
	}

	s.mu.Lock()
	s.listener = ln
	s.httpServer = &http.Server{
		Handler:      s.Handler(),
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 0, // 0 for streaming SSE
		IdleTimeout:  60 * time.Second,
	}
	s.mu.Unlock()

	go func() {
		_ = s.httpServer.Serve(ln)
	}()

	return nil
}

// Addr returns the actual listening address (useful when testing with port 0)
func (s *Server) Addr() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.listener != nil {
		return s.listener.Addr().String()
	}
	return fmt.Sprintf(":%d", s.port)
}

// Stop cleanly shuts down the HTTP server and SSE connections
func (s *Server) Stop(ctx context.Context) error {
	s.broker.Close()

	s.mu.Lock()
	defer s.mu.Unlock()
	if s.httpServer != nil {
		return s.httpServer.Shutdown(ctx)
	}
	return nil
}

func (s *Server) handleGetDevices(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}

	devices := s.hub.GetDevices()
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(devices)
}

func (s *Server) handleGetEvents(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}

	events := s.hub.GetHistory(0) // all buffered events
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(events)
}

func (s *Server) handleConfig(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}

	if s.appConfig == nil {
		http.Error(w, "Configuration not initialized", http.StatusInternalServerError)
		return
	}

	type CameraResponse struct {
		IP          string `json:"ip,omitempty"`
		Target      string `json:"target,omitempty"`
		Endpoint    string `json:"endpoint,omitempty"`
		Username    string `json:"username,omitempty"`
		HasPassword bool   `json:"has_password"`
	}

	type ConfigResponse struct {
		FilePath string `json:"file_path"`
		Default  struct {
			Username    string `json:"username"`
			HasPassword bool   `json:"has_password"`
		} `json:"default"`
		Cameras []CameraResponse `json:"cameras"`
	}

	resp := ConfigResponse{
		FilePath: s.appConfig.FilePath(),
	}
	du, dp := s.appConfig.GetDefaultCredentials()
	resp.Default.Username = du
	resp.Default.HasPassword = (dp != "")

	for _, c := range s.appConfig.ListCameras() {
		resp.Cameras = append(resp.Cameras, CameraResponse{
			IP:          c.IP,
			Target:      c.Target,
			Endpoint:    c.Endpoint,
			Username:    c.Username,
			HasPassword: (c.Password != ""),
		})
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

func (s *Server) handleConfigCamera(w http.ResponseWriter, r *http.Request) {
	if s.appConfig == nil {
		http.Error(w, "Configuration not initialized", http.StatusInternalServerError)
		return
	}

	switch r.Method {
	case http.MethodPost:
		var req struct {
			Name     string `json:"name"`
			IP       string `json:"ip"`
			Target   string `json:"target"`
			Endpoint string `json:"endpoint"`
			Username string `json:"username"`
			Password string `json:"password"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid JSON payload: "+err.Error(), http.StatusBadRequest)
			return
		}

		if req.IP == "" && req.Target == "" && req.Endpoint == "" {
			http.Error(w, "At least one of IP, Target, or Endpoint must be provided", http.StatusBadRequest)
			return
		}

		camCfg := config.CameraConfig{
			Name:     strings.TrimSpace(req.Name),
			IP:       strings.TrimSpace(req.IP),
			Target:   strings.TrimSpace(req.Target),
			Endpoint: strings.TrimSpace(req.Endpoint),
			Username: strings.TrimSpace(req.Username),
			Password: req.Password,
		}

		if err := s.appConfig.SetCameraCredentials(camCfg); err != nil {
			http.Error(w, "Failed to save configuration: "+err.Error(), http.StatusInternalServerError)
			return
		}

		if s.onConfigUpdate != nil {
			ident := camCfg.Target
			if ident == "" {
				ident = camCfg.IP
			}
			if ident == "" {
				ident = camCfg.Endpoint
			}
			s.onConfigUpdate(ident, camCfg.Username, camCfg.Password)
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"status":  "ok",
			"message": fmt.Sprintf("Camera settings saved to %s", s.appConfig.FilePath()),
		})

	case http.MethodDelete:
		target := r.URL.Query().Get("target")
		if target == "" {
			target = r.URL.Query().Get("ip")
		}
		if target == "" {
			http.Error(w, "Missing target or ip query parameter", http.StatusBadRequest)
			return
		}

		removed, err := s.appConfig.RemoveCamera(target)
		if err != nil {
			http.Error(w, "Failed to remove camera: "+err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"status":  "ok",
			"removed": removed,
			"message": fmt.Sprintf("Configuration updated in %s", s.appConfig.FilePath()),
		})

	default:
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleConfigDefault(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}

	if s.appConfig == nil {
		http.Error(w, "Configuration not initialized", http.StatusInternalServerError)
		return
	}

	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid JSON payload: "+err.Error(), http.StatusBadRequest)
		return
	}

	if err := s.appConfig.SetDefaultCredentials(req.Username, req.Password); err != nil {
		http.Error(w, "Failed to save configuration: "+err.Error(), http.StatusInternalServerError)
		return
	}

	if s.onConfigUpdate != nil {
		s.onConfigUpdate("", req.Username, req.Password)
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"status":  "ok",
		"message": fmt.Sprintf("Default credentials saved to %s", s.appConfig.FilePath()),
	})
}

func (s *Server) handleHealthz(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]string{
		"status": "ok",
	})
}

func (s *Server) handleAbout(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}

	info := AboutInfo{
		Name:       "onvif-inspector",
		Version:    s.version,
		DocsURL:    "http://stephenford.org/onvif-inspector/",
		RepoURL:    "https://github.com/smford/onvif-inspector",
		Copyright:  "Copyright (c) 2026 Stephen Ford",
		License:    "MIT License",
		LicenseURL: "https://github.com/smford/onvif-inspector/blob/main/LICENSE",
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(info)
}
