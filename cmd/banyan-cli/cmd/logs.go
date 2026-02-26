package cmd

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/fertile-org/banyan/pkg/types"
)

var (
	logsFollow bool
	logsTail   int
)

var logsCmd = &cobra.Command{
	Use:   "logs <container-name>",
	Short: "Stream container logs",
	Long: `Stream logs from a container by name.

The CLI requests logs from the engine, which proxies them from the agent running the container.

Examples:
  banyan-cli logs my-app-web-0
  banyan-cli logs my-app-web-0 --follow
  banyan-cli logs my-app-web-0 --tail 100
  banyan-cli logs my-app-web-0 -f --tail 50`,
	Args: cobra.ExactArgs(1),
	RunE: runLogs,
}

func init() {
	rootCmd.AddCommand(logsCmd)

	logsCmd.Flags().BoolVarP(&logsFollow, "follow", "f", false, "Follow log output")
	logsCmd.Flags().IntVar(&logsTail, "tail", 0, "Number of lines from end (0 means all)")
}

func runLogs(cmd *cobra.Command, args []string) error {
	containerName := args[0]

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		cancel()
	}()

	engineAddr := types.GetCLIEngineEndpoint(configPath)
	if engineAddr == "" {
		return fmt.Errorf("engine endpoint not configured. Run 'banyan-cli init' to configure")
	}

	client, err := NewAutoEngineClient(engineAddr)
	if err != nil {
		return fmt.Errorf("failed to connect to engine: %w", err)
	}
	defer client.Close()

	reader, err := client.StreamLogs(ctx, containerName, logsFollow, logsTail)
	if err != nil {
		return fmt.Errorf("failed to stream logs: %w", err)
	}
	defer reader.Close()

	_, _ = io.Copy(os.Stdout, reader)
	return nil
}
