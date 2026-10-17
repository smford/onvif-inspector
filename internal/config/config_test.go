package config

import (
	"os"
	"testing"
)

func TestLoadConfigAndGetCredentials(t *testing.T) {
	yamlContent := `
default:
  username: "default_admin"
  password: "default_password"

cameras:
  - ip: "192.168.1.100"
    username: "cam1_user"
    password: "cam1_password"
  - target: "10.0.0.50:8000"
    username: "cam2_user"
    password: "cam2_password"
  - endpoint: "urn:uuid:special-camera"
    username: "cam3_user"
    password: "cam3_password"
`
	tmpFile, err := os.CreateTemp("", "config-*.yaml")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())

	if _, err := tmpFile.Write([]byte(yamlContent)); err != nil {
		t.Fatalf("failed to write temp file: %v", err)
	}
	tmpFile.Close()

	cfg, err := LoadConfig(tmpFile.Name())
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}

	// 1. Match by IP
	u, p, matched := cfg.GetCredentials("192.168.1.100", 80, "urn:uuid:other", "http://192.168.1.100/onvif")
	if !matched || u != "cam1_user" || p != "cam1_password" {
		t.Errorf("expected cam1_user / cam1_password, got %s / %s (matched: %v)", u, p, matched)
	}

	// 2. Match by target
	u, p, matched = cfg.GetCredentials("10.0.0.50", 8000, "urn:uuid:other", "http://10.0.0.50:8000/onvif")
	if !matched || u != "cam2_user" || p != "cam2_password" {
		t.Errorf("expected cam2_user / cam2_password, got %s / %s (matched: %v)", u, p, matched)
	}

	// 3. Match by endpoint
	u, p, matched = cfg.GetCredentials("172.16.0.2", 80, "urn:uuid:special-camera", "http://172.16.0.2/onvif")
	if !matched || u != "cam3_user" || p != "cam3_password" {
		t.Errorf("expected cam3_user / cam3_password, got %s / %s (matched: %v)", u, p, matched)
	}

	// 4. Fallback to default
	u, p, matched = cfg.GetCredentials("192.168.1.222", 80, "urn:uuid:unknown", "http://192.168.1.222/onvif")
	if !matched || u != "default_admin" || p != "default_password" {
		t.Errorf("expected default_admin / default_password, got %s / %s (matched: %v)", u, p, matched)
	}
}

func TestLoadConfigShorthand(t *testing.T) {
	yamlContent := `
username: "simple_user"
password: "simple_password"
`
	tmpFile, err := os.CreateTemp("", "config-simple-*.yaml")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())

	_ = os.WriteFile(tmpFile.Name(), []byte(yamlContent), 0644)

	cfg, err := LoadConfig(tmpFile.Name())
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}

	u, p, matched := cfg.GetCredentials("192.168.1.50", 80, "", "")
	if !matched || u != "simple_user" || p != "simple_password" {
		t.Errorf("expected simple_user / simple_password, got %s / %s (matched: %v)", u, p, matched)
	}
}

func TestLoadConfigNonExistent(t *testing.T) {
	_, err := LoadConfig("non_existent_file_12345.yaml")
	if err == nil {
		t.Error("expected error for non-existent explicit path")
	}

	// Empty path when config.yaml does not exist should return empty config without error
	cfg, err := LoadConfig("")
	if err != nil {
		t.Errorf("unexpected error for empty path: %v", err)
	}
	if cfg == nil {
		t.Error("expected non-nil config")
	}
}

func TestSaveConfigAndMutations(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "config-save-*.yaml")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())
	tmpFile.Close()

	cfg := &Config{}
	cfg.SetFilePath(tmpFile.Name())

	// 1. Set default credentials
	if err := cfg.SetDefaultCredentials("root", "toor"); err != nil {
		t.Fatalf("SetDefaultCredentials failed: %v", err)
	}

	u, p := cfg.GetDefaultCredentials()
	if u != "root" || p != "toor" {
		t.Errorf("expected root/toor, got %s/%s", u, p)
	}

	// 2. Set camera credentials with Name
	if err := cfg.SetCameraCredentials(CameraConfig{
		Name:     "Driveway Cam",
		IP:       "192.168.1.55",
		Username: "cam_admin",
		Password: "cam_secret",
	}); err != nil {
		t.Fatalf("SetCameraCredentials failed: %v", err)
	}

	cameras := cfg.ListCameras()
	if len(cameras) != 1 || cameras[0].IP != "192.168.1.55" || cameras[0].Name != "Driveway Cam" {
		t.Fatalf("expected 1 camera with IP 192.168.1.55 and Name 'Driveway Cam', got: %+v", cameras)
	}

	if camName := cfg.GetCameraName("192.168.1.55", 80, "", ""); camName != "Driveway Cam" {
		t.Errorf("expected GetCameraName 'Driveway Cam', got %q", camName)
	}

	// 3. Reload from disk to verify persistence
	reloaded, err := LoadConfig(tmpFile.Name())
	if err != nil {
		t.Fatalf("failed to reload config from disk: %v", err)
	}

	ru, rp, matched := reloaded.GetCredentials("192.168.1.55", 80, "", "")
	if !matched || ru != "cam_admin" || rp != "cam_secret" {
		t.Errorf("expected reloaded camera creds cam_admin/cam_secret, got %s/%s (matched=%v)", ru, rp, matched)
	}

	du, dp := reloaded.GetDefaultCredentials()
	if du != "root" || dp != "toor" {
		t.Errorf("expected reloaded default root/toor, got %s/%s", du, dp)
	}

	// 4. Update existing camera
	if err := reloaded.SetCameraCredentials(CameraConfig{
		IP:       "192.168.1.55",
		Password: "new_secret",
	}); err != nil {
		t.Fatalf("failed to update camera: %v", err)
	}

	ru, rp, _ = reloaded.GetCredentials("192.168.1.55", 80, "", "")
	if rp != "new_secret" {
		t.Errorf("expected updated password new_secret, got %s", rp)
	}

	// 5. Remove camera
	removed, err := reloaded.RemoveCamera("192.168.1.55")
	if err != nil || !removed {
		t.Fatalf("RemoveCamera failed: err=%v, removed=%v", err, removed)
	}
	if len(reloaded.ListCameras()) != 0 {
		t.Errorf("expected 0 cameras after removal, got %d", len(reloaded.ListCameras()))
	}
}
