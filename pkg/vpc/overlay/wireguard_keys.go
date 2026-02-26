package overlay

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"

	"golang.org/x/crypto/curve25519"
)

const keySize = 32

// GenerateKeyPair generates an X25519 keypair for WireGuard.
// Returns base64-encoded private and public keys in standard WireGuard format.
func GenerateKeyPair() (privateKey, publicKey string, err error) {
	privKey := make([]byte, keySize)
	if _, err := rand.Read(privKey); err != nil {
		return "", "", fmt.Errorf("generate random key: %w", err)
	}

	// Clamp the private key per WireGuard/X25519 convention
	privKey[0] &= 248
	privKey[31] &= 127
	privKey[31] |= 64

	pubKey, err := curve25519.X25519(privKey, curve25519.Basepoint)
	if err != nil {
		return "", "", fmt.Errorf("derive public key: %w", err)
	}

	return base64.StdEncoding.EncodeToString(privKey),
		base64.StdEncoding.EncodeToString(pubKey),
		nil
}

// PublicKeyFromPrivate derives the public key from a base64-encoded private key.
func PublicKeyFromPrivate(privateKeyB64 string) (string, error) {
	privKey, err := base64.StdEncoding.DecodeString(privateKeyB64)
	if err != nil {
		return "", fmt.Errorf("decode private key: %w", err)
	}
	if len(privKey) != keySize {
		return "", fmt.Errorf("invalid private key length: %d (expected %d)", len(privKey), keySize)
	}

	pubKey, err := curve25519.X25519(privKey, curve25519.Basepoint)
	if err != nil {
		return "", fmt.Errorf("derive public key: %w", err)
	}

	return base64.StdEncoding.EncodeToString(pubKey), nil
}
