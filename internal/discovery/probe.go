package discovery

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	MulticastIPv4 = "239.255.255.250"
	DiscoveryPort = 3702
	ActionProbe   = "http://schemas.xmlsoap.org/ws/2005/04/discovery/Probe"
)

// BuildProbeMessage creates a WS-Discovery Probe SOAP 1.2 XML envelope
func BuildProbeMessage() ([]byte, string) {
	msgID := generateUUID()
	body := fmt.Sprintf(`<?xml version="1.0" encoding="utf-8"?>
<Envelope xmlns="http://www.w3.org/2003/05/soap-envelope"
          xmlns:wsa="http://schemas.xmlsoap.org/ws/2004/08/addressing">
  <Header>
    <wsa:Action>%s</wsa:Action>
    <wsa:MessageID>%s</wsa:MessageID>
    <wsa:To>urn:schemas-xmlsoap-org:ws:2005:04:discovery</wsa:To>
  </Header>
  <Body>
    <Probe xmlns="http://schemas.xmlsoap.org/ws/2005/04/discovery"
           xmlns:dn="http://www.onvif.org/ver10/network/wsdl">
      <Types>dn:NetworkVideoTransmitter</Types>
    </Probe>
  </Body>
</Envelope>`, ActionProbe, msgID)

	return []byte(body), msgID
}

func generateUUID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("urn:uuid:%s-%s-%s-%s-%s",
		hex.EncodeToString(b[0:4]),
		hex.EncodeToString(b[4:6]),
		hex.EncodeToString(b[6:8]),
		hex.EncodeToString(b[8:10]),
		hex.EncodeToString(b[10:16]))
}

// Discover probes network interfaces for ONVIF devices.
// If opts.Target is provided, it handles unicast direct discovery.
// Otherwise it scans all active non-loopback NICs (or opts.InterfaceName) via multicast.
func Discover(ctx context.Context, opts ProbeOptions) ([]DiscoveredDevice, error) {
	// If unicast target is explicitly specified
	if opts.Target != "" {
		return discoverTarget(ctx, opts)
	}

	timeout := opts.Timeout
	if timeout == 0 {
		timeout = 3 * time.Second
	}

	interfaces, err := getTargetInterfaces(opts.InterfaceName)
	if err != nil {
		return nil, fmt.Errorf("failed to enumerate network interfaces: %w", err)
	}

	if len(interfaces) == 0 {
		return nil, fmt.Errorf("no suitable network interfaces found for discovery")
	}

	probeData, _ := BuildProbeMessage()
	destAddr := &net.UDPAddr{
		IP:   net.ParseIP(MulticastIPv4),
		Port: DiscoveryPort,
	}

	var (
		mu           sync.Mutex
		foundDevices = make(map[string]DiscoveredDevice)
		wg           sync.WaitGroup
	)

	ctxTimeout, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	for _, iface := range interfaces {
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}

		for _, addr := range addrs {
			ipNet, ok := addr.(*net.IPNet)
			if !ok || ipNet.IP.To4() == nil {
				continue
			}

			localIP := ipNet.IP.To4()
			wg.Add(1)
			go func(srcIP net.IP, ifName string) {
				defer wg.Done()
				scanNIC(ctxTimeout, srcIP, destAddr, probeData, &mu, foundDevices)
			}(localIP, iface.Name)
		}
	}

	wg.Wait()

	devices := make([]DiscoveredDevice, 0, len(foundDevices))
	for _, d := range foundDevices {
		devices = append(devices, d)
	}

	return devices, nil
}

func scanNIC(ctx context.Context, localIP net.IP, destAddr *net.UDPAddr, probeData []byte, mu *sync.Mutex, results map[string]DiscoveredDevice) {
	localAddr := &net.UDPAddr{
		IP:   localIP,
		Port: 0,
	}

	conn, err := net.ListenUDP("udp4", localAddr)
	if err != nil {
		return
	}
	defer conn.Close()

	// Send probe
	sendStart := time.Now()
	if _, err := conn.WriteToUDP(probeData, destAddr); err != nil {
		return
	}

	// Read responses until context canceled or timeout
	buf := make([]byte, 8192)
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		_ = conn.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
		n, remoteAddr, err := conn.ReadFrom(buf)
		if err != nil {
			if nerr, ok := err.(net.Error); ok && nerr.Timeout() {
				if ctx.Err() != nil {
					return
				}
				continue
			}
			return
		}

		rtt := time.Since(sendStart)
		data := make([]byte, n)
		copy(data, buf[:n])

		// Parse ProbeMatch
		devs, err := ParseProbeMatch(data)
		if err != nil || len(devs) == 0 {
			continue
		}

		for _, d := range devs {
			d.PingLatency = rtt
			if d.IP == "" && remoteAddr != nil {
				if udpRemote, ok := remoteAddr.(*net.UDPAddr); ok {
					d.IP = udpRemote.IP.String()
				}
			}

			key := d.Endpoint
			if key == "" {
				key = d.PrimaryXAddr
			}
			if key == "" {
				key = d.IP
			}

			mu.Lock()
			if _, exists := results[key]; !exists {
				results[key] = d
			}
			mu.Unlock()
		}
	}
}

// discoverTarget handles unicast direct target
func discoverTarget(ctx context.Context, opts ProbeOptions) ([]DiscoveredDevice, error) {
	target := strings.TrimSpace(opts.Target)
	if !strings.HasPrefix(target, "http://") && !strings.HasPrefix(target, "https://") {
		target = "http://" + target
	}

	u, err := url.Parse(target)
	if err != nil {
		return nil, fmt.Errorf("invalid target address %q: %w", opts.Target, err)
	}

	host := u.Hostname()
	port := 80
	if u.Port() != "" {
		if p, err := strconv.Atoi(u.Port()); err == nil {
			port = p
		}
	} else if u.Scheme == "https" {
		port = 443
	}

	path := u.Path
	if path == "" || path == "/" {
		path = "/onvif/device_service"
	}
	u.Path = path
	primaryXAddr := u.String()

	// Also attempt unicast UDP probe to target:3702
	dev := DiscoveredDevice{
		Endpoint:     fmt.Sprintf("urn:uuid:target-%s-%d", host, port),
		XAddrs:       []string{primaryXAddr},
		PrimaryXAddr: primaryXAddr,
		IP:           host,
		Port:         port,
		Types:        []string{"dn:NetworkVideoTransmitter"},
		DiscoveredAt: time.Now().UTC(),
	}

	// Try quick unicast UDP probe to target:3702 to enrich scopes if possible
	probeData, _ := BuildProbeMessage()
	remoteUDP, err := net.ResolveUDPAddr("udp4", net.JoinHostPort(host, "3702"))
	if err == nil {
		localConn, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4zero, Port: 0})
		if err == nil {
			defer localConn.Close()
			start := time.Now()
			_, _ = localConn.WriteToUDP(probeData, remoteUDP)
			_ = localConn.SetReadDeadline(time.Now().Add(600 * time.Millisecond))
			buf := make([]byte, 8192)
			n, _, err := localConn.ReadFrom(buf)
			if err == nil && n > 0 {
				rtt := time.Since(start)
				if parsed, err := ParseProbeMatch(buf[:n]); err == nil && len(parsed) > 0 {
					dev = parsed[0]
					dev.PingLatency = rtt
					if dev.PrimaryXAddr == "" {
						dev.PrimaryXAddr = primaryXAddr
					}
				}
			}
		}
	}

	return []DiscoveredDevice{dev}, nil
}

func getTargetInterfaces(interfaceName string) ([]net.Interface, error) {
	ifaces, err := net.Interfaces()
	if err != nil {
		return nil, err
	}

	var results []net.Interface
	for _, iface := range ifaces {
		// Filter down or loopback interfaces
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}

		if interfaceName != "" {
			if strings.EqualFold(iface.Name, interfaceName) {
				return []net.Interface{iface}, nil
			}
			continue
		}

		results = append(results, iface)
	}

	if interfaceName != "" && len(results) == 0 {
		return nil, fmt.Errorf("interface %q not found or not active", interfaceName)
	}

	return results, nil
}
