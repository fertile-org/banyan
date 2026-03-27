package engine

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/fertile-org/banyan/pkg/storage"
)

func newTestSecretsManager(t *testing.T) *SecretsManager {
	t.Helper()
	store := storage.NewMemoryStore()
	key := make([]byte, 32)
	for i := range key {
		key[i] = byte(i)
	}
	sm, err := NewSecretsManagerFromKey(store, key)
	if err != nil {
		t.Fatalf("NewSecretsManagerFromKey: %v", err)
	}
	return sm
}

func TestGenerateSecretsKey(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "secrets.key")

	if err := GenerateSecretsKey(path); err != nil {
		t.Fatalf("GenerateSecretsKey: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if len(data) != 32 {
		t.Errorf("expected 32 bytes, got %d", len(data))
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Errorf("expected 0600 permissions, got %o", info.Mode().Perm())
	}
}

func TestLoadSecretsKey(t *testing.T) {
	t.Run("valid key", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "secrets.key")
		_ = GenerateSecretsKey(path)

		key, err := LoadSecretsKey(path)
		if err != nil {
			t.Fatalf("LoadSecretsKey: %v", err)
		}
		if len(key) != 32 {
			t.Errorf("expected 32 bytes, got %d", len(key))
		}
	})

	t.Run("wrong size", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "bad.key")
		_ = os.WriteFile(path, []byte("short"), 0o600)

		_, err := LoadSecretsKey(path)
		if err == nil {
			t.Fatal("expected error for wrong size key")
		}
	})

	t.Run("missing file", func(t *testing.T) {
		_, err := LoadSecretsKey("/nonexistent/path")
		if err == nil {
			t.Fatal("expected error for missing file")
		}
	})
}

func TestEncryptDecrypt(t *testing.T) {
	sm := newTestSecretsManager(t)
	plaintext := []byte("super-secret-password")

	encrypted, err := sm.encrypt(plaintext, "TEST_SECRET")
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}

	// Encrypted should be different from plaintext
	if bytes.Equal(encrypted, plaintext) {
		t.Error("encrypted should differ from plaintext")
	}

	decrypted, err := sm.decrypt(encrypted, "TEST_SECRET")
	if err != nil {
		t.Fatalf("decrypt: %v", err)
	}

	if !bytes.Equal(decrypted, plaintext) {
		t.Errorf("round-trip failed: got %q, want %q", decrypted, plaintext)
	}
}

func TestEncryptDifferentNonce(t *testing.T) {
	sm := newTestSecretsManager(t)
	plaintext := []byte("same-value")

	enc1, _ := sm.encrypt(plaintext, "KEY")
	enc2, _ := sm.encrypt(plaintext, "KEY")

	if bytes.Equal(enc1, enc2) {
		t.Error("same plaintext should produce different ciphertext (random nonce)")
	}
}

func TestDecryptTampered(t *testing.T) {
	sm := newTestSecretsManager(t)
	encrypted, _ := sm.encrypt([]byte("secret"), "KEY")

	// Tamper with ciphertext
	tampered := make([]byte, len(encrypted))
	copy(tampered, encrypted)
	tampered[len(tampered)-1] ^= 0xff

	_, err := sm.decrypt(tampered, "KEY")
	if err == nil {
		t.Error("expected error for tampered ciphertext")
	}
}

func TestDecryptTooShort(t *testing.T) {
	sm := newTestSecretsManager(t)
	_, err := sm.decrypt([]byte("short"), "KEY")
	if err == nil {
		t.Error("expected error for short ciphertext")
	}
}

func TestCreateSecret(t *testing.T) {
	sm := newTestSecretsManager(t)
	ctx := context.Background()

	err := sm.Create(ctx, "DB_PASSWORD", []byte("secret123"))
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	// Verify it can be read back
	value, err := sm.Get(ctx, "DB_PASSWORD")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if string(value) != "secret123" {
		t.Errorf("got %q, want %q", value, "secret123")
	}
}

func TestCreateSecretDuplicate(t *testing.T) {
	sm := newTestSecretsManager(t)
	ctx := context.Background()

	_ = sm.Create(ctx, "DB_PASSWORD", []byte("first"))
	err := sm.Create(ctx, "DB_PASSWORD", []byte("second"))
	if err == nil {
		t.Fatal("expected error for duplicate secret")
	}
}

func TestUpdateSecret(t *testing.T) {
	sm := newTestSecretsManager(t)
	ctx := context.Background()

	_ = sm.Create(ctx, "API_KEY", []byte("old-key"))
	err := sm.Update(ctx, "API_KEY", []byte("new-key"))
	if err != nil {
		t.Fatalf("Update: %v", err)
	}

	value, _ := sm.Get(ctx, "API_KEY")
	if string(value) != "new-key" {
		t.Errorf("got %q after update, want %q", value, "new-key")
	}
}

func TestUpdateSecretNotFound(t *testing.T) {
	sm := newTestSecretsManager(t)
	ctx := context.Background()

	err := sm.Update(ctx, "NONEXISTENT", []byte("value"))
	if err == nil {
		t.Fatal("expected error for missing secret")
	}
}

func TestGetSecretNotFound(t *testing.T) {
	sm := newTestSecretsManager(t)
	ctx := context.Background()

	_, err := sm.Get(ctx, "NONEXISTENT")
	if err == nil {
		t.Fatal("expected error for missing secret")
	}
}

func TestGetMetadata(t *testing.T) {
	sm := newTestSecretsManager(t)
	ctx := context.Background()

	_ = sm.Create(ctx, "MY_SECRET", []byte("value"))
	record, err := sm.GetMetadata(ctx, "MY_SECRET")
	if err != nil {
		t.Fatalf("GetMetadata: %v", err)
	}
	if record.Name != "MY_SECRET" {
		t.Errorf("got name %q, want %q", record.Name, "MY_SECRET")
	}
	if record.CreatedAt.IsZero() {
		t.Error("expected non-zero CreatedAt")
	}
}

func TestDeleteSecret(t *testing.T) {
	sm := newTestSecretsManager(t)
	ctx := context.Background()

	_ = sm.Create(ctx, "TEMP_SECRET", []byte("value"))
	err := sm.Delete(ctx, "TEMP_SECRET")
	if err != nil {
		t.Fatalf("Delete: %v", err)
	}

	_, err = sm.Get(ctx, "TEMP_SECRET")
	if err == nil {
		t.Error("expected error after delete")
	}
}

func TestDeleteSecretNotFound(t *testing.T) {
	sm := newTestSecretsManager(t)
	ctx := context.Background()

	err := sm.Delete(ctx, "NONEXISTENT")
	if err == nil {
		t.Fatal("expected error for missing secret")
	}
}

func TestListSecrets(t *testing.T) {
	sm := newTestSecretsManager(t)
	ctx := context.Background()

	_ = sm.Create(ctx, "SECRET_A", []byte("a"))
	_ = sm.Create(ctx, "SECRET_B", []byte("b"))
	_ = sm.Create(ctx, "SECRET_C", []byte("c"))

	records, err := sm.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(records) != 3 {
		t.Errorf("expected 3 secrets, got %d", len(records))
	}

	// Verify encrypted values are not included
	for _, r := range records {
		if r.EncryptedValue != nil {
			t.Errorf("List should not include encrypted values, got %d bytes for %q", len(r.EncryptedValue), r.Name)
		}
	}
}

func TestResolveSecrets(t *testing.T) {
	sm := newTestSecretsManager(t)
	ctx := context.Background()

	_ = sm.Create(ctx, "DB_PASSWORD", []byte("pass123"))
	_ = sm.Create(ctx, "API_KEY", []byte("key456"))

	resolved, err := sm.ResolveSecrets(ctx, []string{"DB_PASSWORD", "API_KEY"})
	if err != nil {
		t.Fatalf("ResolveSecrets: %v", err)
	}
	if resolved["DB_PASSWORD"] != "pass123" {
		t.Errorf("DB_PASSWORD = %q, want %q", resolved["DB_PASSWORD"], "pass123")
	}
	if resolved["API_KEY"] != "key456" {
		t.Errorf("API_KEY = %q, want %q", resolved["API_KEY"], "key456")
	}
}

func TestResolveSecretsMissing(t *testing.T) {
	sm := newTestSecretsManager(t)
	ctx := context.Background()

	_ = sm.Create(ctx, "EXISTS", []byte("value"))

	_, err := sm.ResolveSecrets(ctx, []string{"EXISTS", "MISSING"})
	if err == nil {
		t.Fatal("expected error for missing secret")
	}
}

func TestResolveSecretsEmpty(t *testing.T) {
	sm := newTestSecretsManager(t)
	ctx := context.Background()

	resolved, err := sm.ResolveSecrets(ctx, []string{})
	if err != nil {
		t.Fatalf("ResolveSecrets: %v", err)
	}
	if len(resolved) != 0 {
		t.Errorf("expected empty map, got %d entries", len(resolved))
	}
}

func TestValidateSecretName(t *testing.T) {
	valid := []string{"DB_PASSWORD", "API_KEY", "a", "A", "_private", "myVar123"}
	for _, name := range valid {
		if err := ValidateSecretName(name); err != nil {
			t.Errorf("expected %q to be valid, got error: %v", name, err)
		}
	}

	invalid := []string{"", "123abc", "my-secret", "my.secret", "has space", "special@char"}
	for _, name := range invalid {
		if err := ValidateSecretName(name); err == nil {
			t.Errorf("expected %q to be invalid", name)
		}
	}
}

func TestCreateEmptyValue(t *testing.T) {
	sm := newTestSecretsManager(t)
	ctx := context.Background()

	// Empty values are allowed (some tokens can be empty)
	err := sm.Create(ctx, "EMPTY_SECRET", []byte(""))
	if err != nil {
		t.Fatalf("Create with empty value: %v", err)
	}

	value, err := sm.Get(ctx, "EMPTY_SECRET")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if string(value) != "" {
		t.Errorf("expected empty value, got %q", value)
	}
}

func TestNewSecretsManagerFromFile(t *testing.T) {
	dir := t.TempDir()
	keyPath := filepath.Join(dir, "secrets.key")
	_ = GenerateSecretsKey(keyPath)

	store := storage.NewMemoryStore()
	sm, err := NewSecretsManager(store, keyPath)
	if err != nil {
		t.Fatalf("NewSecretsManager: %v", err)
	}

	ctx := context.Background()
	_ = sm.Create(ctx, "TEST", []byte("value"))
	value, _ := sm.Get(ctx, "TEST")
	if string(value) != "value" {
		t.Errorf("got %q, want %q", value, "value")
	}
}
