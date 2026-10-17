package onvif

import (
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

// PushReceiver manages an embedded HTTP webhook listener for camera event pushes
type PushReceiver struct {
	listener   net.Listener
	httpServer *http.Server
	port       int
	handlers   map[string]EventHandler
	mu         sync.RWMutex
}

// NewPushReceiver starts an HTTP server listening for ONVIF WS-BaseNotification push events
func NewPushReceiver(port int) (*PushReceiver, error) {
	addr := fmt.Sprintf(":%d", port)
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("failed to listen on %s for push webhook: %w", addr, err)
	}

	actualPort := ln.Addr().(*net.TCPAddr).Port

	pr := &PushReceiver{
		listener: ln,
		port:     actualPort,
		handlers: make(map[string]EventHandler),
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/", pr.handleNotify)

	pr.httpServer = &http.Server{
		Handler:      mux,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
	}

	go func() {
		_ = pr.httpServer.Serve(ln)
	}()

	return pr, nil
}

// Port returns the TCP port the push receiver is listening on
func (pr *PushReceiver) Port() int {
	return pr.port
}

// RegisterHandler registers an event handler for a camera key (e.g. IP or endpoint)
func (pr *PushReceiver) RegisterHandler(camKey string, handler EventHandler) {
	pr.mu.Lock()
	defer pr.mu.Unlock()
	pr.handlers[normalizeKey(camKey)] = handler
}

// UnregisterHandler removes an event handler
func (pr *PushReceiver) UnregisterHandler(camKey string) {
	pr.mu.Lock()
	defer pr.mu.Unlock()
	delete(pr.handlers, normalizeKey(camKey))
}

// GetConsumerURL calculates the reachable IP towards targetIP and builds the consumer webhook URL
func (pr *PushReceiver) GetConsumerURL(targetIP string, camKey string) (string, error) {
	outboundIP, err := getOutboundIP(targetIP)
	if err != nil {
		return "", fmt.Errorf("failed to determine outbound IP for target %s: %w", targetIP, err)
	}

	normKey := normalizeKey(camKey)
	return fmt.Sprintf("http://%s:%d/notify/%s", outboundIP, pr.port, normKey), nil
}

// Close stops the HTTP server and listener
func (pr *PushReceiver) Close() error {
	if pr.httpServer != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		return pr.httpServer.Shutdown(ctx)
	}
	return nil
}

func (pr *PushReceiver) handleNotify(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	bodyBytes, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Failed to read body", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	// Parse incoming Notify envelope
	var env NotifyEnvelope
	if err := xml.Unmarshal(bodyBytes, &env); err != nil {
		// Try parsing generic SOAP envelope or ignore unparseable
		http.Error(w, "Failed to parse SOAP XML", http.StatusBadRequest)
		return
	}

	messages := env.Body.Notify.NotificationMessages

	// Determine matching handler from URL path (/notify/<key>) or RemoteAddr IP
	path := strings.TrimPrefix(r.URL.Path, "/")
	pathParts := strings.Split(path, "/")
	var matchedKey string
	if len(pathParts) >= 2 && pathParts[0] == "notify" {
		matchedKey = normalizeKey(pathParts[1])
	}

	pr.mu.RLock()
	var handler EventHandler
	if matchedKey != "" {
		handler = pr.handlers[matchedKey]
	}

	// Fallback to match by remote IP if URL path didn't match directly
	if handler == nil {
		hostOnly, _, _ := net.SplitHostPort(r.RemoteAddr)
		if hostOnly != "" {
			handler = pr.handlers[normalizeKey(hostOnly)]
		}
	}

	// Fallback to single registered handler if only 1 camera registered
	if handler == nil && len(pr.handlers) == 1 {
		for _, h := range pr.handlers {
			handler = h
			break
		}
	}
	pr.mu.RUnlock()

	rawReq := string(bodyBytes)
	respBody := `<?xml version="1.0" encoding="utf-8"?><s:Envelope xmlns:s="http://www.w3.org/2003/05/soap-envelope"><s:Body/></s:Envelope>`

	if handler != nil && len(messages) > 0 {
		for _, msg := range messages {
			handler.OnNotification(msg, rawReq, respBody)
		}
	}

	w.Header().Set("Content-Type", "application/soap+xml; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(respBody))
}

func normalizeKey(k string) string {
	k = strings.TrimSpace(k)
	k = strings.ReplaceAll(k, ":", "_")
	k = strings.ReplaceAll(k, "/", "_")
	return strings.ToLower(k)
}

func getOutboundIP(targetIP string) (string, error) {
	// Strip port if present
	host := targetIP
	if h, _, err := net.SplitHostPort(targetIP); err == nil && h != "" {
		host = h
	}

	conn, err := net.DialTimeout("udp", host+":80", 2*time.Second)
	if err != nil {
		return "", err
	}
	defer conn.Close()

	localAddr := conn.LocalAddr().(*net.UDPAddr)
	return localAddr.IP.String(), nil
}
