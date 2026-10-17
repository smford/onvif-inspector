package config

import (
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"strings"
	"sync"

	"gopkg.in/yaml.v3"
)

// Config represents the application configuration file structure
type Config struct {
	mu sync.RWMutex `yaml:"-"`

	filePath string `yaml:"-"`

	// Top-level default credentials (shorthand)
	Username string `yaml:"username,omitempty"`
	Password string `yaml:"password,omitempty"`

	// Structured default credentials
	Default Credentials `yaml:"default,omitempty"`

	// Per-camera specific credentials
	Cameras []CameraConfig `yaml:"cameras,omitempty"`

	// Optional defaults for networking & web
	Interface string `yaml:"interface,omitempty"`
	Target    string `yaml:"target,omitempty"`
	WebPort   int    `yaml:"web_port,omitempty"`
}

// Credentials holds username and password
type Credentials struct {
	Username string `yaml:"username,omitempty"`
	Password string `yaml:"password,omitempty"`
}

// CameraConfig specifies credentials for a specific camera matched by IP, target, or endpoint
type CameraConfig struct {
	Name     string `yaml:"name,omitempty" json:"name,omitempty"`
	IP       string `yaml:"ip,omitempty" json:"ip,omitempty"`
	Target   string `yaml:"target,omitempty" json:"target,omitempty"`
	Endpoint string `yaml:"endpoint,omitempty" json:"endpoint,omitempty"`
	Username string `yaml:"username,omitempty" json:"username,omitempty"`
	Password string `yaml:"password,omitempty" json:"password,omitempty"`
}

// LoadConfig loads configuration from the given file path.
// If path is empty, it checks for "./config.yaml". If neither exists, returns a new Config with default path "config.yaml".
func LoadConfig(path string) (*Config, error) {
	resolvedPath := path
	if resolvedPath == "" {
		if _, err := os.Stat("config.yaml"); err == nil {
			resolvedPath = "config.yaml"
		} else {
			return &Config{filePath: "config.yaml"}, nil
		}
	}

	data, err := os.ReadFile(resolvedPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file %q: %w", resolvedPath, err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse yaml config %q: %w", resolvedPath, err)
	}

	cfg.filePath = resolvedPath
	return &cfg, nil
}

// LoadOrCreateConfig loads configuration from path, or returns an empty Config targeting path if it does not exist yet.
func LoadOrCreateConfig(path string) (*Config, error) {
	if path == "" {
		return LoadConfig("")
	}
	cfg, err := LoadConfig(path)
	if err != nil {
		if os.IsNotExist(err) || errors.Is(err, os.ErrNotExist) {
			return &Config{filePath: path}, nil
		}
		var pathErr *os.PathError
		if errors.As(err, &pathErr) && (errors.Is(pathErr.Err, os.ErrNotExist) || os.IsNotExist(pathErr.Err)) {
			return &Config{filePath: path}, nil
		}
		return nil, err
	}
	return cfg, nil
}

// FilePath returns the underlying file path of the configuration
func (c *Config) FilePath() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.filePath == "" {
		return "config.yaml"
	}
	return c.filePath
}

// SetFilePath updates the target file path for saving
func (c *Config) SetFilePath(path string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.filePath = path
}

// Save writes the current configuration back to its source file path
func (c *Config) Save() error {
	c.mu.RLock()
	path := c.filePath
	if path == "" {
		path = "config.yaml"
	}
	c.mu.RUnlock()
	return c.SaveToFile(path)
}

// SaveToFile marshals and saves configuration to a specific file
func (c *Config) SaveToFile(path string) error {
	c.mu.RLock()
	defer c.mu.RUnlock()

	data, err := yaml.Marshal(c)
	if err != nil {
		return fmt.Errorf("failed to marshal config to yaml: %w", err)
	}

	if err := os.WriteFile(path, data, 0600); err != nil {
		return fmt.Errorf("failed to write config file %q: %w", path, err)
	}

	return nil
}

// SetDefaultCredentials updates default credentials and saves to disk
func (c *Config) SetDefaultCredentials(username, password string) error {
	c.mu.Lock()
	c.Default.Username = username
	c.Default.Password = password
	c.Username = ""
	c.Password = ""
	c.mu.Unlock()

	return c.Save()
}

// SetCameraCredentials adds or updates credentials for a camera and saves to disk
func (c *Config) SetCameraCredentials(cam CameraConfig) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	found := false
	for i, existing := range c.Cameras {
		match := false
		if cam.Endpoint != "" && existing.Endpoint != "" && strings.EqualFold(cam.Endpoint, existing.Endpoint) {
			match = true
		} else if cam.IP != "" && existing.IP != "" && strings.EqualFold(cam.IP, existing.IP) {
			match = true
		} else if cam.Target != "" && existing.Target != "" && strings.EqualFold(cam.Target, existing.Target) {
			match = true
		}

		if match {
			if cam.Name != "" {
				c.Cameras[i].Name = cam.Name
			}
			if cam.Username != "" {
				c.Cameras[i].Username = cam.Username
			}
			if cam.Password != "" {
				c.Cameras[i].Password = cam.Password
			}
			if cam.IP != "" {
				c.Cameras[i].IP = cam.IP
			}
			if cam.Target != "" {
				c.Cameras[i].Target = cam.Target
			}
			if cam.Endpoint != "" {
				c.Cameras[i].Endpoint = cam.Endpoint
			}
			found = true
			break
		}
	}

	if !found {
		c.Cameras = append(c.Cameras, cam)
	}

	// Save within lock
	path := c.filePath
	if path == "" {
		path = "config.yaml"
	}
	data, err := yaml.Marshal(c)
	if err != nil {
		return fmt.Errorf("failed to marshal config to yaml: %w", err)
	}
	return os.WriteFile(path, data, 0600)
}

// RemoveCamera deletes a camera entry matching target, IP, or endpoint
func (c *Config) RemoveCamera(ident string) (bool, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	ident = strings.TrimSpace(ident)
	foundIdx := -1
	for i, cam := range c.Cameras {
		if strings.EqualFold(cam.IP, ident) ||
			strings.EqualFold(cam.Target, ident) ||
			strings.EqualFold(cam.Endpoint, ident) {
			foundIdx = i
			break
		}
	}

	if foundIdx == -1 {
		return false, nil
	}

	c.Cameras = append(c.Cameras[:foundIdx], c.Cameras[foundIdx+1:]...)

	path := c.filePath
	if path == "" {
		path = "config.yaml"
	}
	data, err := yaml.Marshal(c)
	if err != nil {
		return false, fmt.Errorf("failed to marshal config to yaml: %w", err)
	}
	if err := os.WriteFile(path, data, 0600); err != nil {
		return false, err
	}

	return true, nil
}

// ListCameras returns a copy of configured cameras
func (c *Config) ListCameras() []CameraConfig {
	c.mu.RLock()
	defer c.mu.RUnlock()

	res := make([]CameraConfig, len(c.Cameras))
	copy(res, c.Cameras)
	return res
}

// GetDefaultCredentials returns the configured default credentials
func (c *Config) GetDefaultCredentials() (string, string) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if c.Default.Username != "" || c.Default.Password != "" {
		return c.Default.Username, c.Default.Password
	}
	if c.Username != "" || c.Password != "" {
		return c.Username, c.Password
	}
	return "admin", ""
}

// GetCredentials resolves credentials for a specific camera.
// It checks camera-specific entries first, then falls back to default credentials.
func (c *Config) GetCredentials(ip string, port int, endpoint string, xaddr string) (username string, password string, matched bool) {
	if c == nil {
		return "", "", false
	}

	c.mu.RLock()
	defer c.mu.RUnlock()

	// 1. Check camera-specific entries
	for _, cam := range c.Cameras {
		if cam.Endpoint != "" && strings.EqualFold(cam.Endpoint, endpoint) {
			return cam.Username, cam.Password, true
		}

		if cam.IP != "" {
			camHost := cam.IP
			if h, _, err := net.SplitHostPort(cam.IP); err == nil {
				camHost = h
			}
			if camHost == ip {
				return cam.Username, cam.Password, true
			}
		}

		if cam.Target != "" {
			target := cam.Target
			if strings.EqualFold(target, ip) ||
				strings.EqualFold(target, fmt.Sprintf("%s:%d", ip, port)) ||
				strings.Contains(xaddr, target) {
				return cam.Username, cam.Password, true
			}

			// URL match check
			if u, err := url.Parse(target); err == nil && u.Host != "" {
				if strings.Contains(xaddr, u.Host) || strings.EqualFold(u.Hostname(), ip) {
					return cam.Username, cam.Password, true
				}
			}
		}
	}

	// 2. Structured default section
	if c.Default.Username != "" || c.Default.Password != "" {
		return c.Default.Username, c.Default.Password, true
	}

	// 3. Root-level shorthand credentials
	if c.Username != "" || c.Password != "" {
		return c.Username, c.Password, true
	}

	return "", "", false
}

// GetCameraName returns the configured name/alias for a camera, if one exists
func (c *Config) GetCameraName(ip string, port int, endpoint string, xaddr string) string {
	if c == nil {
		return ""
	}

	c.mu.RLock()
	defer c.mu.RUnlock()

	for _, cam := range c.Cameras {
		if cam.Name == "" {
			continue
		}

		if cam.Endpoint != "" && strings.EqualFold(cam.Endpoint, endpoint) {
			return cam.Name
		}

		if cam.IP != "" {
			camHost := cam.IP
			if h, _, err := net.SplitHostPort(cam.IP); err == nil {
				camHost = h
			}
			if camHost == ip {
				return cam.Name
			}
		}

		if cam.Target != "" {
			target := cam.Target
			if strings.EqualFold(target, ip) ||
				strings.EqualFold(target, fmt.Sprintf("%s:%d", ip, port)) ||
				strings.Contains(xaddr, target) {
				return cam.Name
			}

			if u, err := url.Parse(target); err == nil && u.Host != "" {
				if strings.Contains(xaddr, u.Host) || strings.EqualFold(u.Hostname(), ip) {
					return cam.Name
				}
			}
		}
	}

	return ""
}
