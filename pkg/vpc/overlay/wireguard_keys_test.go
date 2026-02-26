package overlay

import (
	"encoding/base64"
	"testing"
)

func TestGenerateKeyPair(t *testing.T) {
	privKey, pubKey, err := GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair failed: %v", err)
	}

	// Keys should be valid base64
	privBytes, err := base64.StdEncoding.DecodeString(privKey)
	if err != nil {
		t.Fatalf("private key is not valid base64: %v", err)
	}
	pubBytes, err := base64.StdEncoding.DecodeString(pubKey)
	if err != nil {
		t.Fatalf("public key is not valid base64: %v", err)
	}

	// Keys should be 32 bytes
	if len(privBytes) != 32 {
		t.Errorf("private key length: %d, expected 32", len(privBytes))
	}
	if len(pubBytes) != 32 {
		t.Errorf("public key length: %d, expected 32", len(pubBytes))
	}

	// Verify private key clamping
	if privBytes[0]&7 != 0 {
		t.Error("private key not clamped: low 3 bits should be 0")
	}
	if privBytes[31]&128 != 0 {
		t.Error("private key not clamped: high bit should be 0")
	}
	if privBytes[31]&64 == 0 {
		t.Error("private key not clamped: bit 6 should be 1")
	}
}

func TestGenerateKeyPair_Unique(t *testing.T) {
	privKey1, pubKey1, _ := GenerateKeyPair()
	privKey2, pubKey2, _ := GenerateKeyPair()

	if privKey1 == privKey2 {
		t.Error("two keypairs should have different private keys")
	}
	if pubKey1 == pubKey2 {
		t.Error("two keypairs should have different public keys")
	}
}

func TestPublicKeyFromPrivate(t *testing.T) {
	privKey, expectedPub, err := GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair failed: %v", err)
	}

	derivedPub, err := PublicKeyFromPrivate(privKey)
	if err != nil {
		t.Fatalf("PublicKeyFromPrivate failed: %v", err)
	}

	if derivedPub != expectedPub {
		t.Errorf("derived public key %q does not match expected %q", derivedPub, expectedPub)
	}
}

func TestPublicKeyFromPrivate_Deterministic(t *testing.T) {
	privKey, _, _ := GenerateKeyPair()

	pub1, _ := PublicKeyFromPrivate(privKey)
	pub2, _ := PublicKeyFromPrivate(privKey)

	if pub1 != pub2 {
		t.Error("same private key should always derive the same public key")
	}
}

func TestPublicKeyFromPrivate_InvalidBase64(t *testing.T) {
	_, err := PublicKeyFromPrivate("not-valid-base64!!!")
	if err == nil {
		t.Fatal("expected error for invalid base64")
	}
}

func TestPublicKeyFromPrivate_WrongLength(t *testing.T) {
	// 16 bytes instead of 32
	short := base64.StdEncoding.EncodeToString(make([]byte, 16))
	_, err := PublicKeyFromPrivate(short)
	if err == nil {
		t.Fatal("expected error for wrong key length")
	}
}
