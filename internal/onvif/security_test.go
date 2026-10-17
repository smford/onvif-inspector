package onvif

import (
	"crypto/sha1"
	"encoding/base64"
	"strings"
	"testing"
	"time"
)

func TestComputePasswordDigest(t *testing.T) {
	// Standard verification:
	rawNonce := []byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08, 0x09, 0x0a, 0x0b, 0x0c, 0x0d, 0x0e, 0x0f, 0x10}
	createdStr := "2026-10-17T12:00:00Z"
	password := "secret123"

	digest := ComputePasswordDigest(rawNonce, createdStr, password)

	// Manual calculation
	h := sha1.New()
	h.Write(rawNonce)
	h.Write([]byte(createdStr))
	h.Write([]byte(password))
	expected := base64.StdEncoding.EncodeToString(h.Sum(nil))

	if digest != expected {
		t.Fatalf("expected digest %s, got %s", expected, digest)
	}
}

func TestGenerateSecurityHeader(t *testing.T) {
	creds := SecurityCredentials{
		Username: "admin",
		Password: "password123",
	}
	fixedTime := time.Date(2026, 10, 17, 12, 30, 45, 0, time.UTC)

	header, err := GenerateSecurityHeader(creds, fixedTime)
	if err != nil {
		t.Fatalf("GenerateSecurityHeader failed: %v", err)
	}

	if header == nil {
		t.Fatal("expected non-nil header")
	}

	if header.UsernameToken.Username != "admin" {
		t.Errorf("expected username 'admin', got '%s'", header.UsernameToken.Username)
	}

	if header.UsernameToken.Created != "2026-10-17T12:30:45Z" {
		t.Errorf("expected created '2026-10-17T12:30:45Z', got '%s'", header.UsernameToken.Created)
	}

	if !strings.HasPrefix(header.UsernameToken.Password.Type, "http://docs.oasis-open.org/wss/2004/01/oasis-200401-wss-username-token-profile-1.0#PasswordDigest") {
		t.Errorf("unexpected password type: %s", header.UsernameToken.Password.Type)
	}

	// Verify nonce can be base64-decoded into 16 bytes
	nonceBytes, err := base64.StdEncoding.DecodeString(header.UsernameToken.Nonce.Value)
	if err != nil {
		t.Fatalf("failed to decode nonce base64: %v", err)
	}
	if len(nonceBytes) != 16 {
		t.Errorf("expected 16 bytes nonce, got %d", len(nonceBytes))
	}

	// Verify digest
	recomputed := ComputePasswordDigest(nonceBytes, header.UsernameToken.Created, creds.Password)
	if header.UsernameToken.Password.Value != recomputed {
		t.Errorf("recomputed digest %s != header digest %s", recomputed, header.UsernameToken.Password.Value)
	}
}

func TestGenerateSecurityHeaderEmptyUsername(t *testing.T) {
	creds := SecurityCredentials{Username: "", Password: ""}
	header, err := GenerateSecurityHeader(creds, time.Now())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if header != nil {
		t.Errorf("expected nil header for empty username, got %v", header)
	}
}
