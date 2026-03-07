package storage

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	clientv3 "go.etcd.io/etcd/client/v3"
)

// etcdKV abstracts the etcd client operations used by EtcdStore.
// *clientv3.Client satisfies this interface.
type etcdKV interface {
	Put(ctx context.Context, key, val string, opts ...clientv3.OpOption) (*clientv3.PutResponse, error)
	Get(ctx context.Context, key string, opts ...clientv3.OpOption) (*clientv3.GetResponse, error)
	Delete(ctx context.Context, key string, opts ...clientv3.OpOption) (*clientv3.DeleteResponse, error)
	Grant(ctx context.Context, ttl int64) (*clientv3.LeaseGrantResponse, error)
	KeepAliveOnce(ctx context.Context, id clientv3.LeaseID) (*clientv3.LeaseKeepAliveResponse, error)
	Watch(ctx context.Context, key string, opts ...clientv3.OpOption) clientv3.WatchChan
	Close() error
}

// EtcdOptions configures the etcd client connection.
type EtcdOptions struct {
	Endpoints []string
	Prefix    string
	Username  string // etcd RBAC username
	Password  string // etcd RBAC password
	CertFile  string // client certificate for mTLS
	KeyFile   string // client key for mTLS
	CAFile    string // CA certificate for server verification
}

// EtcdStore implements StateStore using etcd as the backend
type EtcdStore struct {
	client etcdKV
	prefix string // Key prefix for all VPC data (e.g., "/banyan/vpc/")
}

// NewEtcdStore creates a new EtcdStore with the given options.
func NewEtcdStore(opts *EtcdOptions) (*EtcdStore, error) {
	if len(opts.Endpoints) == 0 {
		return nil, fmt.Errorf("at least one etcd endpoint is required")
	}

	prefix := opts.Prefix
	if prefix == "" {
		prefix = "/banyan/vpc/"
	}

	// Ensure prefix ends with /
	if !strings.HasSuffix(prefix, "/") {
		prefix += "/"
	}

	cfg := clientv3.Config{
		Endpoints:   opts.Endpoints,
		DialTimeout: 5 * time.Second,
	}

	if opts.Username != "" {
		cfg.Username = opts.Username
		cfg.Password = opts.Password
	}

	tlsCfg, err := buildTLSConfig(opts.CertFile, opts.KeyFile, opts.CAFile)
	if err != nil {
		return nil, fmt.Errorf("failed to build TLS config: %w", err)
	}
	if tlsCfg != nil {
		cfg.TLS = tlsCfg
	}

	client, err := clientv3.New(cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create etcd client: %w", err)
	}

	// Verify connectivity with a short timeout
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	_, err = client.Status(ctx, opts.Endpoints[0])
	if err != nil {
		client.Close()
		return nil, fmt.Errorf("failed to connect to etcd: %w", err)
	}

	return &EtcdStore{
		client: client,
		prefix: prefix,
	}, nil
}

// buildTLSConfig builds a *tls.Config from optional cert/key/CA file paths.
// Returns nil if no TLS files are provided.
func buildTLSConfig(certFile, keyFile, caFile string) (*tls.Config, error) {
	if certFile == "" && keyFile == "" && caFile == "" {
		return nil, nil
	}

	tlsCfg := &tls.Config{MinVersion: tls.VersionTLS12}

	// Load client certificate for mTLS
	if certFile != "" && keyFile != "" {
		cert, err := tls.LoadX509KeyPair(certFile, keyFile)
		if err != nil {
			return nil, fmt.Errorf("failed to load client cert/key: %w", err)
		}
		tlsCfg.Certificates = []tls.Certificate{cert}
	} else if certFile != "" || keyFile != "" {
		return nil, fmt.Errorf("both etcd_cert_file and etcd_key_file must be provided together")
	}

	// Load CA certificate for server verification
	if caFile != "" {
		caCert, err := os.ReadFile(caFile)
		if err != nil {
			return nil, fmt.Errorf("failed to read CA file: %w", err)
		}
		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM(caCert) {
			return nil, fmt.Errorf("failed to parse CA certificate from %s", caFile)
		}
		tlsCfg.RootCAs = pool
	}

	return tlsCfg, nil
}

// NewEtcdStoreWithClient creates an EtcdStore with an existing client (useful for testing)
func NewEtcdStoreWithClient(client etcdKV, prefix string) *EtcdStore {
	if prefix == "" {
		prefix = "/banyan/vpc/"
	}

	if !strings.HasSuffix(prefix, "/") {
		prefix += "/"
	}

	return &EtcdStore{
		client: client,
		prefix: prefix,
	}
}

// Save stores a value by key
func (s *EtcdStore) Save(ctx context.Context, key string, value interface{}) error {
	if key == "" {
		return fmt.Errorf("key cannot be empty")
	}

	if value == nil {
		return fmt.Errorf("value cannot be nil")
	}

	// Marshal value to JSON
	data, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("failed to marshal value: %w", err)
	}

	// Build full key with prefix
	fullKey := s.prefix + key

	// Save to etcd
	_, err = s.client.Put(ctx, fullKey, string(data))
	if err != nil {
		return fmt.Errorf("failed to save to etcd: %w", err)
	}

	return nil
}

// SaveWithTTL stores a value with a TTL (time-to-live) for automatic expiration
func (s *EtcdStore) SaveWithTTL(ctx context.Context, key string, value interface{}, ttl time.Duration) error {
	if key == "" {
		return fmt.Errorf("key cannot be empty")
	}

	if value == nil {
		return fmt.Errorf("value cannot be nil")
	}

	// Marshal value to JSON
	data, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("failed to marshal value: %w", err)
	}

	// Create lease
	lease, err := s.client.Grant(ctx, int64(ttl.Seconds()))
	if err != nil {
		return fmt.Errorf("failed to create lease: %w", err)
	}

	// Build full key with prefix
	fullKey := s.prefix + key

	// Save to etcd with lease
	_, err = s.client.Put(ctx, fullKey, string(data), clientv3.WithLease(lease.ID))
	if err != nil {
		return fmt.Errorf("failed to save with TTL: %w", err)
	}

	return nil
}

// Get retrieves a value by key
func (s *EtcdStore) Get(ctx context.Context, key string, value interface{}) error {
	if key == "" {
		return fmt.Errorf("key cannot be empty")
	}

	if value == nil {
		return fmt.Errorf("value cannot be nil")
	}

	// Build full key with prefix
	fullKey := s.prefix + key

	// Get from etcd
	resp, err := s.client.Get(ctx, fullKey)
	if err != nil {
		return fmt.Errorf("failed to get from etcd: %w", err)
	}

	// Check if key exists
	if len(resp.Kvs) == 0 {
		return fmt.Errorf("key not found: %s", key)
	}

	// Unmarshal value
	if err := json.Unmarshal(resp.Kvs[0].Value, value); err != nil {
		return fmt.Errorf("failed to unmarshal value: %w", err)
	}

	return nil
}

// Delete removes a value by key
func (s *EtcdStore) Delete(ctx context.Context, key string) error {
	if key == "" {
		return fmt.Errorf("key cannot be empty")
	}

	// Build full key with prefix
	fullKey := s.prefix + key

	// Delete from etcd
	_, err := s.client.Delete(ctx, fullKey)
	if err != nil {
		return fmt.Errorf("failed to delete from etcd: %w", err)
	}

	return nil
}

// List returns all keys with a given prefix
func (s *EtcdStore) List(ctx context.Context, prefix string) ([]string, error) {
	// Build full prefix
	fullPrefix := s.prefix + prefix

	// Get all keys with prefix
	resp, err := s.client.Get(ctx, fullPrefix, clientv3.WithPrefix(), clientv3.WithKeysOnly())
	if err != nil {
		return nil, fmt.Errorf("failed to list keys: %w", err)
	}

	// Extract keys and remove the store prefix
	keys := make([]string, 0, len(resp.Kvs))
	for _, kv := range resp.Kvs {
		// Remove the store prefix to return relative keys
		key := strings.TrimPrefix(string(kv.Key), s.prefix)
		keys = append(keys, key)
	}

	return keys, nil
}

// Watch watches for changes to keys with a given prefix
func (s *EtcdStore) Watch(ctx context.Context, prefix string) clientv3.WatchChan {
	// Build full prefix
	fullPrefix := s.prefix + prefix

	// Create watch channel
	return s.client.Watch(ctx, fullPrefix, clientv3.WithPrefix())
}

// Close closes the etcd client connection
func (s *EtcdStore) Close() error {
	if s.client != nil {
		return s.client.Close()
	}
	return nil
}

// KeepAlive keeps a lease alive by renewing it periodically
func (s *EtcdStore) KeepAlive(ctx context.Context, key string, value interface{}, ttl time.Duration, renewInterval time.Duration) error {
	// Create initial lease
	lease, err := s.client.Grant(ctx, int64(ttl.Seconds()))
	if err != nil {
		return fmt.Errorf("failed to create lease: %w", err)
	}

	// Marshal value
	data, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("failed to marshal value: %w", err)
	}

	// Build full key
	fullKey := s.prefix + key

	// Initial save with lease
	_, err = s.client.Put(ctx, fullKey, string(data), clientv3.WithLease(lease.ID))
	if err != nil {
		return fmt.Errorf("failed to save with lease: %w", err)
	}

	// Start keepalive goroutine
	go func() {
		ticker := time.NewTicker(renewInterval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				// Renew lease
				_, err := s.client.KeepAliveOnce(ctx, lease.ID)
				if err != nil {
					// Lease might have expired, create new one
					newLease, err := s.client.Grant(ctx, int64(ttl.Seconds()))
					if err != nil {
						continue
					}

					// Update lease ID
					lease = newLease

					// Re-save with new lease
					if _, putErr := s.client.Put(ctx, fullKey, string(data), clientv3.WithLease(lease.ID)); putErr != nil {
						continue
					}
				}
			}
		}
	}()

	return nil
}
