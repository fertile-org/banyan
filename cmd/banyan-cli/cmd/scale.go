package cmd

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/fertile-org/banyan/pkg/logging"
	"github.com/fertile-org/banyan/pkg/rpc/banyanpb"
	"github.com/fertile-org/banyan/pkg/types"
)

var scaleTags []string

var scaleCmd = &cobra.Command{
	Use:   "scale <app-name> <service=replicas> [service=replicas ...]",
	Short: "Scale services in a running deployment",
	Long:  "Adjust the replica count of services without redeploying. Example: banyan-cli scale my-app api=5 web=3",
	Args:  cobra.MinimumNArgs(2),
	RunE:  runScale,
}

func init() {
	rootCmd.AddCommand(scaleCmd)
	scaleCmd.Flags().StringSliceVar(&scaleTags, "tags", nil, "Deployment tags for matching")
}

func runScale(cmd *cobra.Command, args []string) error {
	logging.Setup(nil)
	log := logging.New("cli")

	appName := args[0]

	// Parse service=replicas args
	replicas := make(map[string]int32)
	for _, arg := range args[1:] {
		parts := strings.SplitN(arg, "=", 2)
		if len(parts) != 2 {
			return fmt.Errorf("invalid argument %q: expected service=replicas format", arg)
		}
		count, err := strconv.Atoi(parts[1])
		if err != nil || count < 0 {
			return fmt.Errorf("invalid replica count %q for service %q", parts[1], parts[0])
		}
		replicas[parts[0]] = int32(count) //nolint:gosec // count is validated
	}

	// Connect to engine
	engineAddr := types.GetCLIEngineEndpoint(configPath)
	if engineAddr == "" {
		return fmt.Errorf("engine endpoint not configured. Run 'banyan-cli init' to configure")
	}

	client, err := NewAutoEngineClient(engineAddr)
	if err != nil {
		return fmt.Errorf("failed to connect to engine: %w", err)
	}
	defer client.Close()

	log.Info("Scaling deployment", "name", appName, "replicas", replicas)

	resp, err := client.client.Scale(cmd.Context(), &banyanpb.ScaleRequest{
		Name:     appName,
		Replicas: replicas,
		Tags:     types.SortTags(scaleTags),
	})
	if err != nil {
		return fmt.Errorf("scale failed: %w", err)
	}

	for svc, prev := range resp.Previous {
		cur := resp.Current[svc]
		switch {
		case prev == cur:
			fmt.Printf("  %s: %d replicas (unchanged)\n", svc, cur)
		case cur > prev:
			fmt.Printf("  %s: %d → %d replicas (scaling up)\n", svc, prev, cur)
		default:
			fmt.Printf("  %s: %d → %d replicas (scaling down)\n", svc, prev, cur)
		}
	}

	return nil
}
