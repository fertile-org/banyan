package vpc

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// etcdV3PutRequest is the request body for the etcd v3 HTTP gateway PUT.
type etcdV3PutRequest struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

// InitializeNetwork configures the Flannel network in etcd.
// Uses the etcd v3 HTTP gateway API because Flannel v0.22.x reads from the v3 store.
func InitializeNetwork(ctx context.Context, etcdEndpoints []string, vpcCIDR string) error {
	config := fmt.Sprintf(`{"Network": "%s", "SubnetLen": 24, "Backend": {"Type": "vxlan"}}`, vpcCIDR)

	endpoint := etcdEndpoints[0]
	apiURL := strings.TrimRight(endpoint, "/") + "/v3/kv/put"

	putReq := etcdV3PutRequest{
		Key:   base64.StdEncoding.EncodeToString([]byte("/coreos.com/network/config")),
		Value: base64.StdEncoding.EncodeToString([]byte(config)),
	}
	body, err := json.Marshal(putReq)
	if err != nil {
		return fmt.Errorf("failed to marshal etcd put request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, apiURL, strings.NewReader(string(body)))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to write Flannel config to etcd: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("etcd returned status %d: %s", resp.StatusCode, respBody)
	}

	return nil
}

