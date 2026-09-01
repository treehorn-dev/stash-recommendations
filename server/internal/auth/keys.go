package auth

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"strings"

	"golang.org/x/crypto/argon2"
)

const (
	argon2Memory      = 64 * 1024
	argon2Iterations  = 3
	argon2Parallelism = 2
	argon2SaltLength  = 16
	argon2KeyLength   = 32
	apiKeyLength      = 32
)

// NewAPIKey returns a randomly generated bearer credential for an account.
func NewAPIKey() (string, error) {
	key := make([]byte, apiKeyLength)
	if _, err := rand.Read(key); err != nil {
		return "", fmt.Errorf("generate API key: %w", err)
	}
	return "srk_" + base64.RawURLEncoding.EncodeToString(key), nil
}

// HashAPIKey derives a self-describing Argon2id hash suitable for storage.
func HashAPIKey(plaintext string) (string, error) {
	if strings.TrimSpace(plaintext) == "" {
		return "", fmt.Errorf("API key is required")
	}

	salt := make([]byte, argon2SaltLength)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("generate API key salt: %w", err)
	}
	hash := argon2.IDKey([]byte(plaintext), salt, argon2Iterations, argon2Memory, argon2Parallelism, argon2KeyLength)
	return fmt.Sprintf(
		"argon2id$v=19$m=%d,t=%d,p=%d$%s$%s",
		argon2Memory,
		argon2Iterations,
		argon2Parallelism,
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(hash),
	), nil
}

// VerifyAPIKey reports whether plaintext matches a stored Argon2id hash.
func VerifyAPIKey(encodedHash, plaintext string) bool {
	memory, iterations, parallelism, salt, expectedHash, ok := parseHash(encodedHash)
	if !ok {
		return false
	}
	actualHash := argon2.IDKey([]byte(plaintext), salt, iterations, memory, parallelism, uint32(len(expectedHash)))
	return subtle.ConstantTimeCompare(actualHash, expectedHash) == 1
}

func parseHash(encoded string) (uint32, uint32, uint8, []byte, []byte, bool) {
	parts := strings.Split(encoded, "$")
	if len(parts) != 5 || parts[0] != "argon2id" || parts[1] != "v=19" {
		return 0, 0, 0, nil, nil, false
	}

	var memory uint32
	var iterations uint32
	var parallelism uint8
	if _, err := fmt.Sscanf(parts[2], "m=%d,t=%d,p=%d", &memory, &iterations, &parallelism); err != nil {
		return 0, 0, 0, nil, nil, false
	}
	if memory == 0 || memory > 256*1024 || iterations == 0 || iterations > 10 || parallelism == 0 || parallelism > 16 {
		return 0, 0, 0, nil, nil, false
	}

	salt, err := base64.RawStdEncoding.DecodeString(parts[3])
	if err != nil || len(salt) < argon2SaltLength {
		return 0, 0, 0, nil, nil, false
	}
	hash, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil || len(hash) == 0 {
		return 0, 0, 0, nil, nil, false
	}
	return memory, iterations, parallelism, salt, hash, true
}
