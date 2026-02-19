package storage

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/redis/go-redis/v9"
)

// RedisStore implements StateStore using Redis as the backend.
type RedisStore struct {
	client *redis.Client
	prefix string
}

// NewRedisStore creates a new RedisStore connected to the given address.
func NewRedisStore(address, prefix string) (*RedisStore, error) {
	if address == "" {
		address = "localhost:6379"
	}

	if prefix == "" {
		prefix = "/banyan/"
	}

	if !strings.HasSuffix(prefix, "/") {
		prefix += "/"
	}

	client := redis.NewClient(&redis.Options{
		Addr: address,
	})

	// Verify connectivity
	ctx := context.Background()
	if err := client.Ping(ctx).Err(); err != nil {
		client.Close()
		return nil, fmt.Errorf("failed to connect to redis at %s: %w", address, err)
	}

	return &RedisStore{
		client: client,
		prefix: prefix,
	}, nil
}

// NewRedisStoreWithClient creates a RedisStore with an existing client (useful for testing).
func NewRedisStoreWithClient(client *redis.Client, prefix string) *RedisStore {
	if prefix == "" {
		prefix = "/banyan/"
	}

	if !strings.HasSuffix(prefix, "/") {
		prefix += "/"
	}

	return &RedisStore{
		client: client,
		prefix: prefix,
	}
}

func (s *RedisStore) Save(ctx context.Context, key string, value interface{}) error {
	if key == "" {
		return fmt.Errorf("key cannot be empty")
	}

	if value == nil {
		return fmt.Errorf("value cannot be nil")
	}

	data, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("failed to marshal value: %w", err)
	}

	fullKey := s.prefix + key
	if err := s.client.Set(ctx, fullKey, data, 0).Err(); err != nil {
		return fmt.Errorf("failed to save to redis: %w", err)
	}

	return nil
}

func (s *RedisStore) Get(ctx context.Context, key string, value interface{}) error {
	if key == "" {
		return fmt.Errorf("key cannot be empty")
	}

	if value == nil {
		return fmt.Errorf("value cannot be nil")
	}

	fullKey := s.prefix + key
	data, err := s.client.Get(ctx, fullKey).Bytes()
	if err != nil {
		if err == redis.Nil {
			return fmt.Errorf("key not found: %s", key)
		}
		return fmt.Errorf("failed to get from redis: %w", err)
	}

	if err := json.Unmarshal(data, value); err != nil {
		return fmt.Errorf("failed to unmarshal value: %w", err)
	}

	return nil
}

func (s *RedisStore) Delete(ctx context.Context, key string) error {
	if key == "" {
		return fmt.Errorf("key cannot be empty")
	}

	fullKey := s.prefix + key
	if err := s.client.Del(ctx, fullKey).Err(); err != nil {
		return fmt.Errorf("failed to delete from redis: %w", err)
	}

	return nil
}

func (s *RedisStore) List(ctx context.Context, prefix string) ([]string, error) {
	fullPrefix := s.prefix + prefix
	pattern := fullPrefix + "*"

	var keys []string
	var cursor uint64
	for {
		var batch []string
		var err error
		batch, cursor, err = s.client.Scan(ctx, cursor, pattern, 100).Result()
		if err != nil {
			return nil, fmt.Errorf("failed to scan keys: %w", err)
		}
		for _, key := range batch {
			keys = append(keys, strings.TrimPrefix(key, s.prefix))
		}
		if cursor == 0 {
			break
		}
	}

	return keys, nil
}

func (s *RedisStore) Close() error {
	if s.client != nil {
		return s.client.Close()
	}
	return nil
}
