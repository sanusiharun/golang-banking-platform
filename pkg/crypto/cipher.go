// Package crypto provides shared cryptographic utilities for the banking platform.
package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
)

// EncryptSubject encrypts a plaintext subject (e.g. user ID) using AES-256-GCM
// and returns a base64url-encoded ciphertext safe for use in JWT claims.
// key must be exactly 32 bytes (AES-256).
func EncryptSubject(key []byte, plaintext string) (string, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", fmt.Errorf("cipher: new block: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("cipher: new gcm: %w", err)
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", fmt.Errorf("cipher: generate nonce: %w", err)
	}

	// Seal appends ciphertext+tag to nonce so both travel together.
	sealed := gcm.Seal(nonce, nonce, []byte(plaintext), nil)
	return base64.RawURLEncoding.EncodeToString(sealed), nil
}

// DecryptSubject decodes and decrypts a base64url-encoded AES-256-GCM ciphertext
// produced by EncryptSubject.
// key must be exactly 32 bytes (AES-256).
func DecryptSubject(key []byte, encoded string) (string, error) {
	data, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil {
		return "", fmt.Errorf("cipher: base64 decode: %w", err)
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return "", fmt.Errorf("cipher: new block: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("cipher: new gcm: %w", err)
	}

	if len(data) < gcm.NonceSize() {
		return "", errors.New("cipher: ciphertext too short")
	}

	nonce, ciphertext := data[:gcm.NonceSize()], data[gcm.NonceSize():]
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return "", fmt.Errorf("cipher: decrypt: %w", err)
	}

	return string(plaintext), nil
}
