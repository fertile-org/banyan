package vpc

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestInitializeNetwork_Success(t *testing.T) {
	var receivedMethod, receivedPath string
	var receivedReq etcdV3PutRequest

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedMethod = r.Method
		receivedPath = r.URL.Path
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &receivedReq)
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, `{"header":{"revision":"1"}}`)
	}))
	defer srv.Close()

	err := InitializeNetwork(context.Background(), []string{srv.URL}, "10.0.0.0/16")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if receivedMethod != http.MethodPost {
		t.Errorf("expected POST, got %s", receivedMethod)
	}
	if receivedPath != "/v3/kv/put" {
		t.Errorf("expected /v3/kv/put, got %s", receivedPath)
	}

	// Verify the key is base64-encoded /coreos.com/network/config
	decodedKey, err := base64.StdEncoding.DecodeString(receivedReq.Key)
	if err != nil {
		t.Fatalf("failed to decode key: %v", err)
	}
	if string(decodedKey) != "/coreos.com/network/config" {
		t.Errorf("expected key /coreos.com/network/config, got %s", decodedKey)
	}

	// Verify the value contains the CIDR
	decodedValue, err := base64.StdEncoding.DecodeString(receivedReq.Value)
	if err != nil {
		t.Fatalf("failed to decode value: %v", err)
	}
	if string(decodedValue) == "" {
		t.Error("expected non-empty value")
	}
	expectedConfig := `{"Network": "10.0.0.0/16", "SubnetLen": 24, "Backend": {"Type": "vxlan"}}`
	if string(decodedValue) != expectedConfig {
		t.Errorf("expected value %q, got %q", expectedConfig, decodedValue)
	}
}

func TestInitializeNetwork_EtcdError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprint(w, `{"error":"internal error"}`)
	}))
	defer srv.Close()

	err := InitializeNetwork(context.Background(), []string{srv.URL}, "10.0.0.0/16")
	if err == nil {
		t.Fatal("expected error for 500 response, got nil")
	}
}

func TestInitializeNetwork_ConnectionRefused(t *testing.T) {
	err := InitializeNetwork(context.Background(), []string{"http://127.0.0.1:19999"}, "10.0.0.0/16")
	if err == nil {
		t.Fatal("expected error for connection refused, got nil")
	}
}
