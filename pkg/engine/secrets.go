package engine

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"fmt"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/fertile-org/banyan/pkg/storage"
	"github.com/fertile-org/banyan/pkg/types"
)

// secretNameRegexp validates that a secret name is a valid environment variable identifier.
var secretNameRegexp = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// SecretsManager handles encrypted secret storage in etcd.
type SecretsManager struct {
	store  storage.StateStore
	aesgcm cipher.AEAD
}

// NewSecretsManager creates a manager that encrypts/decrypts secrets using the
// AES-256-GCM key stored at keyPath.
func NewSecretsManager(store storage.StateStore, keyPath string) (*SecretsManager, error) {
	key, err := LoadSecretsKey(keyPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load secrets key: %w", err)
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("failed to create AES cipher: %w", err)
	}
	aesgcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("failed to create GCM: %w", err)
	}
	return &SecretsManager{store: store, aesgcm: aesgcm}, nil
}

// NewSecretsManagerFromKey creates a manager directly from a 32-byte key (for testing).
func NewSecretsManagerFromKey(store storage.StateStore, key []byte) (*SecretsManager, error) {
	if len(key) != 32 {
		return nil, fmt.Errorf("secrets key must be 32 bytes, got %d", len(key))
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("failed to create AES cipher: %w", err)
	}
	aesgcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("failed to create GCM: %w", err)
	}
	return &SecretsManager{store: store, aesgcm: aesgcm}, nil
}

// GenerateSecretsKey creates a random 256-bit (32-byte) key file.
func GenerateSecretsKey(path string) error {
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return fmt.Errorf("failed to generate random key: %w", err)
	}
	if err := os.MkdirAll(dirOf(path), 0o700); err != nil {
		return fmt.Errorf("failed to create key directory: %w", err)
	}
	if err := os.WriteFile(path, key, 0o600); err != nil {
		return fmt.Errorf("failed to write key file: %w", err)
	}
	return nil
}

// LoadSecretsKey reads a 32-byte key from disk and verifies file permissions.
func LoadSecretsKey(path string) ([]byte, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("failed to stat key file: %w", err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		return nil, fmt.Errorf("secrets.key has insecure permissions %04o (must be 0600). Fix with: chmod 600 %s", perm, path)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read key file: %w", err)
	}
	if len(data) != 32 {
		return nil, fmt.Errorf("invalid key size: expected 32 bytes, got %d", len(data))
	}
	return data, nil
}

// ValidateSecretName checks that a name is a valid environment variable identifier.
func ValidateSecretName(name string) error {
	if name == "" {
		return fmt.Errorf("secret name cannot be empty")
	}
	if !secretNameRegexp.MatchString(name) {
		return fmt.Errorf("secret name %q must be a valid environment variable name (letters, digits, underscores, cannot start with digit)", name)
	}
	return nil
}

// Create stores a new encrypted secret. Returns error if name already exists.
func (sm *SecretsManager) Create(ctx context.Context, name string, value []byte) error {
	key := types.KeySecrets + name
	var existing types.SecretRecord
	if err := sm.store.Get(ctx, key, &existing); err == nil {
		return fmt.Errorf("secret %q already exists (use update to change its value)", name)
	}

	encrypted, err := sm.encrypt(value, name)
	if err != nil {
		return fmt.Errorf("failed to encrypt secret: %w", err)
	}

	now := time.Now()
	record := types.SecretRecord{
		Name:           name,
		EncryptedValue: encrypted,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	return sm.store.Save(ctx, key, &record)
}

// Update replaces the value of an existing secret. Returns error if not found.
func (sm *SecretsManager) Update(ctx context.Context, name string, value []byte) error {
	key := types.KeySecrets + name
	var existing types.SecretRecord
	if err := sm.store.Get(ctx, key, &existing); err != nil {
		return fmt.Errorf("secret %q not found", name)
	}

	encrypted, err := sm.encrypt(value, name)
	if err != nil {
		return fmt.Errorf("failed to encrypt secret: %w", err)
	}

	existing.EncryptedValue = encrypted
	existing.UpdatedAt = time.Now()
	return sm.store.Save(ctx, key, &existing)
}

// Get decrypts and returns a secret value. Returns error if not found.
func (sm *SecretsManager) Get(ctx context.Context, name string) ([]byte, error) {
	key := types.KeySecrets + name
	var record types.SecretRecord
	if err := sm.store.Get(ctx, key, &record); err != nil {
		return nil, fmt.Errorf("secret %q not found", name)
	}
	plaintext, err := sm.decrypt(record.EncryptedValue, name)
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt secret %q: %w", name, err)
	}
	return plaintext, nil
}

// GetMetadata returns a secret record without decrypting the value.
func (sm *SecretsManager) GetMetadata(ctx context.Context, name string) (*types.SecretRecord, error) {
	key := types.KeySecrets + name
	var record types.SecretRecord
	if err := sm.store.Get(ctx, key, &record); err != nil {
		return nil, fmt.Errorf("secret %q not found", name)
	}
	return &record, nil
}

// List returns all secret names with metadata (no values decrypted).
func (sm *SecretsManager) List(ctx context.Context) ([]types.SecretRecord, error) {
	keys, err := sm.store.List(ctx, types.KeySecrets)
	if err != nil {
		return nil, fmt.Errorf("failed to list secrets: %w", err)
	}
	var records []types.SecretRecord
	for _, k := range keys {
		var record types.SecretRecord
		if err := sm.store.Get(ctx, k, &record); err != nil {
			continue
		}
		// Clear encrypted value — callers don't need it
		record.EncryptedValue = nil
		records = append(records, record)
	}
	return records, nil
}

// Delete removes a secret from the store. Does NOT check references — caller's responsibility.
func (sm *SecretsManager) Delete(ctx context.Context, name string) error {
	key := types.KeySecrets + name
	var existing types.SecretRecord
	if err := sm.store.Get(ctx, key, &existing); err != nil {
		return fmt.Errorf("secret %q not found", name)
	}
	return sm.store.Delete(ctx, key)
}

// ResolveSecrets decrypts multiple secrets by name, returning a name→value map.
// Used by PollTasks for just-in-time secret resolution.
func (sm *SecretsManager) ResolveSecrets(ctx context.Context, names []string) (map[string]string, error) {
	result := make(map[string]string, len(names))
	var missing []string
	for _, name := range names {
		value, err := sm.Get(ctx, name)
		if err != nil {
			missing = append(missing, name)
			continue
		}
		result[name] = string(value)
	}
	if len(missing) > 0 {
		return nil, fmt.Errorf("secrets not found: %s", strings.Join(missing, ", "))
	}
	return result, nil
}

// encrypt encrypts plaintext with AES-256-GCM. The nonce is prepended to the ciphertext.
// The secret name is used as additional authenticated data (AAD) to bind the ciphertext
// to this specific key path — prevents ciphertext relocation attacks in etcd.
func (sm *SecretsManager) encrypt(plaintext []byte, name string) ([]byte, error) {
	nonce := make([]byte, sm.aesgcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, fmt.Errorf("failed to generate nonce: %w", err)
	}
	return sm.aesgcm.Seal(nonce, nonce, plaintext, []byte(name)), nil
}

// decrypt decrypts ciphertext with AES-256-GCM. Expects nonce prepended to ciphertext.
// The secret name must match the AAD used during encryption.
func (sm *SecretsManager) decrypt(ciphertext []byte, name string) ([]byte, error) {
	nonceSize := sm.aesgcm.NonceSize()
	if len(ciphertext) < nonceSize {
		return nil, fmt.Errorf("ciphertext too short")
	}
	return sm.aesgcm.Open(nil, ciphertext[:nonceSize], ciphertext[nonceSize:], []byte(name))
}

// dirOf returns the directory part of a file path.
func dirOf(path string) string {
	for i := len(path) - 1; i >= 0; i-- {
		if path[i] == '/' {
			return path[:i]
		}
	}
	return "."
}
