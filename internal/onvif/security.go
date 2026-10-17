package onvif

import (
	"crypto/rand"
	"crypto/sha1"
	"encoding/base64"
	"fmt"
	"time"
)

// SecurityCredentials holds authentication data
type SecurityCredentials struct {
	Username string
	Password string
}

// GenerateSecurityHeader creates a WS-Security header for the given credentials and time.
// The createdTime passed in should already include any clock drift compensation.
func GenerateSecurityHeader(creds SecurityCredentials, createdTime time.Time) (*SecurityHeader, error) {
	if creds.Username == "" {
		return nil, nil
	}

	// 16 random bytes for nonce
	rawNonce := make([]byte, 16)
	if _, err := rand.Read(rawNonce); err != nil {
		return nil, fmt.Errorf("failed to generate random nonce: %w", err)
	}

	// Format time in UTC ISO 8601 / RFC3339
	// Many cameras strictly require seconds precision without fractional seconds: YYYY-MM-DDTHH:MM:SSZ
	createdStr := createdTime.UTC().Format("2006-01-02T15:04:05Z")

	// Calculate PasswordDigest: Base64( SHA-1( raw_nonce + created_string + password ) )
	hasher := sha1.New()
	hasher.Write(rawNonce)
	hasher.Write([]byte(createdStr))
	hasher.Write([]byte(creds.Password))
	digest := base64.StdEncoding.EncodeToString(hasher.Sum(nil))

	nonceB64 := base64.StdEncoding.EncodeToString(rawNonce)

	header := &SecurityHeader{
		MustUnderstand: "1",
		UsernameToken: UsernameToken{
			Username: creds.Username,
			Password: PasswordElement{
				Type:  PasswordDigestType,
				Value: digest,
			},
			Nonce: NonceElement{
				EncodingType: NonceEncodingType,
				Value:        nonceB64,
			},
			Created: createdStr,
		},
	}

	return header, nil
}

// ComputePasswordDigest calculates the digest for given raw nonce, created time string, and password.
// This is exposed for deterministic testing and verification.
func ComputePasswordDigest(rawNonce []byte, createdStr string, password string) string {
	hasher := sha1.New()
	hasher.Write(rawNonce)
	hasher.Write([]byte(createdStr))
	hasher.Write([]byte(password))
	return base64.StdEncoding.EncodeToString(hasher.Sum(nil))
}
