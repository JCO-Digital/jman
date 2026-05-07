package config

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"io"
	"strings"

	"golang.org/x/crypto/hkdf"
)

const (
	// encryptedPrefix is prepended to encrypted TOTP secrets to distinguish
	// them from plaintext values (backward compatibility).
	encryptedPrefix = "enc:"

	// hkdfSalt is a fixed application-specific salt for HKDF key derivation.
	hkdfSalt = "jman-totp-encryption-v1"

	// hkdfInfo is the context info string for HKDF key derivation.
	hkdfInfo = "totp-secret-encryption"

	// aesKeySize is the size of the derived AES-256 key in bytes.
	aesKeySize = 32

	// gcmNonceSize is the standard nonce size for AES-GCM (12 bytes).
	gcmNonceSize = 12
)

// DeriveEncryptionKey uses HKDF with SHA-256 to derive a 32-byte AES-256 key
// from the JWT secret. The derivation uses a fixed application-specific salt
// and info string so the same JWT secret always produces the same encryption key.
func DeriveEncryptionKey(jwtSecret string) ([]byte, error) {
	if jwtSecret == "" {
		return nil, fmt.Errorf("JWT secret must not be empty")
	}

	hkdfReader := hkdf.New(sha256.New, []byte(jwtSecret), []byte(hkdfSalt), []byte(hkdfInfo))

	key := make([]byte, aesKeySize)
	if _, err := io.ReadFull(hkdfReader, key); err != nil {
		return nil, fmt.Errorf("failed to derive encryption key: %w", err)
	}

	return key, nil
}

// EncryptTOTPSecret encrypts a plaintext TOTP secret using AES-256-GCM.
// The result is returned as "enc:" + base64(nonce + ciphertext).
func EncryptTOTPSecret(plaintext string, key []byte) (string, error) {
	if plaintext == "" {
		return "", nil
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return "", fmt.Errorf("failed to create AES cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("failed to create GCM: %w", err)
	}

	nonce := make([]byte, gcmNonceSize)
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", fmt.Errorf("failed to generate nonce: %w", err)
	}

	ciphertext := gcm.Seal(nil, nonce, []byte(plaintext), nil)

	// Concatenate nonce + ciphertext and base64 encode
	combined := append(nonce, ciphertext...)
	encoded := base64.StdEncoding.EncodeToString(combined)

	return encryptedPrefix + encoded, nil
}

// DecryptTOTPSecret decrypts an encrypted TOTP secret. If the value does not
// have the "enc:" prefix, it is returned as-is for backward compatibility
// with plaintext secrets.
func DecryptTOTPSecret(encoded string, key []byte) (string, error) {
	if !IsEncryptedTOTPSecret(encoded) {
		// Return plaintext as-is for backward compatibility
		return encoded, nil
	}

	// Strip the "enc:" prefix
	b64Data := strings.TrimPrefix(encoded, encryptedPrefix)

	combined, err := base64.StdEncoding.DecodeString(b64Data)
	if err != nil {
		return "", fmt.Errorf("failed to base64 decode encrypted secret: %w", err)
	}

	if len(combined) < gcmNonceSize {
		return "", fmt.Errorf("encrypted data too short")
	}

	nonce := combined[:gcmNonceSize]
	ciphertext := combined[gcmNonceSize:]

	block, err := aes.NewCipher(key)
	if err != nil {
		return "", fmt.Errorf("failed to create AES cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("failed to create GCM: %w", err)
	}

	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return "", fmt.Errorf("failed to decrypt TOTP secret (wrong key or corrupted data): %w", err)
	}

	return string(plaintext), nil
}

// IsEncryptedTOTPSecret checks whether the given string has the encrypted prefix.
func IsEncryptedTOTPSecret(s string) bool {
	return strings.HasPrefix(s, encryptedPrefix)
}
