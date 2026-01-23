// Copyright 2024 Sherlock Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Package crypto provides encryption utilities for sensitive data.
package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// Encryptor provides AES-256-GCM encryption for sensitive data.
type Encryptor struct {
	key []byte // 32 bytes for AES-256
}

// NewEncryptor creates a new encryptor with the given secret.
// If secret is empty, it will try to get the key from:
// 1. Environment variable SHERLOCK_MASTER_KEY
// 2. Config file ~/.config/sherlock/master.key
// 3. Default key derived from machine ID
func NewEncryptor(secret string) (*Encryptor, error) {
	var key []byte

	if secret != "" {
		// Derive key from provided secret
		key = deriveKey(secret)
	} else {
		// Try environment variable
		if envKey := os.Getenv("SHERLOCK_MASTER_KEY"); envKey != "" {
			key = deriveKey(envKey)
		} else {
			// Try config file
			homeDir, _ := os.UserHomeDir()
			keyFile := filepath.Join(homeDir, ".config", "sherlock", "master.key")
			if data, err := os.ReadFile(keyFile); err == nil && len(data) > 0 {
				key = deriveKey(string(data))
			} else {
				// Use default key derived from machine-specific info
				key = deriveKey(getDefaultSecret())
			}
		}
	}

	return &Encryptor{key: key}, nil
}

// Encrypt encrypts plaintext using AES-256-GCM and returns base64-encoded ciphertext.
func (e *Encryptor) Encrypt(plaintext string) (string, error) {
	if plaintext == "" {
		return "", nil
	}

	block, err := aes.NewCipher(e.key)
	if err != nil {
		return "", fmt.Errorf("failed to create cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("failed to create GCM: %w", err)
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", fmt.Errorf("failed to generate nonce: %w", err)
	}

	ciphertext := gcm.Seal(nonce, nonce, []byte(plaintext), nil)
	return base64.StdEncoding.EncodeToString(ciphertext), nil
}

// Decrypt decrypts base64-encoded ciphertext using AES-256-GCM.
func (e *Encryptor) Decrypt(ciphertext string) (string, error) {
	if ciphertext == "" {
		return "", nil
	}

	data, err := base64.StdEncoding.DecodeString(ciphertext)
	if err != nil {
		return "", fmt.Errorf("failed to decode base64: %w", err)
	}

	block, err := aes.NewCipher(e.key)
	if err != nil {
		return "", fmt.Errorf("failed to create cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("failed to create GCM: %w", err)
	}

	nonceSize := gcm.NonceSize()
	if len(data) < nonceSize {
		return "", fmt.Errorf("ciphertext too short")
	}

	nonce, ciphertextBytes := data[:nonceSize], data[nonceSize:]
	plaintext, err := gcm.Open(nil, nonce, ciphertextBytes, nil)
	if err != nil {
		return "", fmt.Errorf("failed to decrypt: %w", err)
	}

	return string(plaintext), nil
}

// deriveKey derives a 32-byte key from the given secret using SHA-256.
func deriveKey(secret string) []byte {
	hash := sha256.Sum256([]byte(secret))
	return hash[:]
}

// getDefaultSecret returns a machine-specific default secret.
func getDefaultSecret() string {
	// Combine hostname and home directory for a machine-specific secret
	hostname, _ := os.Hostname()
	homeDir, _ := os.UserHomeDir()
	return fmt.Sprintf("sherlock-%s-%s", hostname, homeDir)
}

// DefaultEncryptor returns the default encryptor instance.
func DefaultEncryptor() (*Encryptor, error) {
	return NewEncryptor("")
}
