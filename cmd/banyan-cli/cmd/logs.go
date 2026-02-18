package cmd

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/fertile-org/banyan/pkg/vpc/storage"
	"github.com/spf13/cobra"
)

var (
	logsFollow       bool
	logsTail         int
	logsEtcdEndpoint string
)

var logsCmd = &cobra.Command{
	Use:   "logs <container-name>",
	Short: "Stream container logs",
	Long: `Stream logs from a container by name.

If the container is running locally, logs are streamed directly.
If not found locally, the cluster is queried to locate the container
and logs are streamed from the remote agent.

Examples:
  banyan-cli logs my-app-web-0
  banyan-cli logs my-app-web-0 --follow
  banyan-cli logs my-app-web-0 --tail 100
  banyan-cli logs my-app-web-0 -f --tail 50 --etcd http://engine:2379`,
	Args: cobra.ExactArgs(1),
	RunE: runLogs,
}

func init() {
	rootCmd.AddCommand(logsCmd)

	logsCmd.Flags().BoolVarP(&logsFollow, "follow", "f", false, "Follow log output")
	logsCmd.Flags().IntVar(&logsTail, "tail", 0, "Number of lines from end (0 means all)")
	logsCmd.Flags().StringVar(&logsEtcdEndpoint, "etcd", "http://localhost:2379", "etcd endpoint to look up container location")
}

func runLogs(cmd *cobra.Command, args []string) error {
	containerName := args[0]
	opts := LogOptions{
		Follow: logsFollow,
		Tail:   logsTail,
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle interrupt signals for clean shutdown
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		cancel()
	}()

	// Try local streaming first
	provider := &NerdctlLogProvider{}
	reader, err := provider.StreamLogs(ctx, containerName, opts)
	if err == nil {
		defer reader.Close()
		_, _ = io.Copy(os.Stdout, reader)
		return nil
	}

	// Container not found locally — look up in the cluster
	fmt.Println("Container not found locally. Looking up in cluster...")

	connCtx, connCancel := context.WithTimeout(ctx, 10*time.Second)
	defer connCancel()

	store, err := storage.NewEtcdStore([]string{logsEtcdEndpoint}, "/banyan")
	if err != nil {
		return fmt.Errorf("failed to connect to etcd: %w", err)
	}

	task, node, err := findContainerAgent(connCtx, store, containerName)
	if err != nil {
		return err
	}

	if node.APIAddress == "" {
		fmt.Printf("Container %s is on node %s. Agent API not available.\n", containerName, task.AgentID)
		fmt.Printf("Run on that node: banyan-cli logs %s\n", containerName)
		return nil
	}

	// Stream logs from remote agent
	return streamRemoteLogs(ctx, node.APIAddress, containerName, opts)
}

// findContainerAgent scans etcd to find which agent has the given container.
// Returns the matching task and its node record.
func findContainerAgent(ctx context.Context, store *storage.EtcdStore, containerName string) (*TaskRecord, *NodeRecord, error) {
	nodeKeys, err := store.List(ctx, keyNodes)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to list nodes: %w", err)
	}

	for _, nodeKey := range nodeKeys {
		var node NodeRecord
		if err := store.Get(ctx, nodeKey, &node); err != nil {
			continue
		}

		taskKeys, err := store.List(ctx, keyTasks+node.Name+"/")
		if err != nil {
			continue
		}

		for _, taskKey := range taskKeys {
			var task TaskRecord
			if err := store.Get(ctx, taskKey, &task); err != nil {
				continue
			}
			if task.ContainerName == containerName && task.Type == taskTypeCreateAndStart {
				return &task, &node, nil
			}
		}
	}

	return nil, nil, fmt.Errorf("container not found locally or in cluster")
}

// streamRemoteLogs streams logs from a remote agent's HTTP API.
func streamRemoteLogs(ctx context.Context, apiAddress, containerName string, opts LogOptions) error {
	url := fmt.Sprintf("http://%s/v1/logs/%s?follow=%t&tail=%d", apiAddress, containerName, opts.Follow, opts.Tail)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to connect to agent at %s: %w", apiAddress, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("agent returned status %d: %s", resp.StatusCode, string(body))
	}

	_, _ = io.Copy(os.Stdout, resp.Body)
	return nil
}
