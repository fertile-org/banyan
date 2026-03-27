package cmd

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/fertile-org/banyan/pkg/logging"
	"github.com/fertile-org/banyan/pkg/types"
)

var secretCmd = &cobra.Command{
	Use:   "secret",
	Short: "Manage secrets",
	Long:  "Create, list, get, and delete encrypted secrets stored in the Banyan engine.",
}

var secretCreateCmd = &cobra.Command{
	Use:   "create <name>",
	Short: "Create or update a secret",
	Long:  "Store an encrypted secret on the engine. If the secret already exists, its value is updated.",
	Args:  cobra.ExactArgs(1),
	RunE:  runSecretCreate,
}

var secretListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all secrets",
	RunE:  runSecretList,
}

var secretGetCmd = &cobra.Command{
	Use:   "get <name>",
	Short: "Show secret metadata (use --reveal for value)",
	Args:  cobra.ExactArgs(1),
	RunE:  runSecretGet,
}

var secretDeleteCmd = &cobra.Command{
	Use:   "delete <name>",
	Short: "Delete a secret",
	Args:  cobra.ExactArgs(1),
	RunE:  runSecretDelete,
}

func init() {
	rootCmd.AddCommand(secretCmd)
	secretCmd.AddCommand(secretCreateCmd)
	secretCmd.AddCommand(secretListCmd)
	secretCmd.AddCommand(secretGetCmd)
	secretCmd.AddCommand(secretDeleteCmd)

	secretCreateCmd.Flags().String("from-file", "", "Read secret value from file")
	secretCreateCmd.Flags().String("value", "", "Secret value (visible in shell history — prefer interactive prompt or --from-file)")
	secretGetCmd.Flags().Bool("reveal", false, "Show the decrypted secret value")
}

func runSecretCreate(cmd *cobra.Command, args []string) error {
	logging.Setup(nil)
	name := args[0]

	var value []byte

	fromFile, _ := cmd.Flags().GetString("from-file")
	valueFlag, _ := cmd.Flags().GetString("value")

	switch {
	case fromFile != "":
		data, err := os.ReadFile(fromFile)
		if err != nil {
			return fmt.Errorf("failed to read file %q: %w", fromFile, err)
		}
		value = []byte(strings.TrimRight(string(data), "\n\r"))
	case valueFlag != "":
		value = []byte(valueFlag)
	default:
		// Interactive prompt with hidden input
		fmt.Print("Enter secret value: ")
		raw, err := term.ReadPassword(int(os.Stdin.Fd()))
		fmt.Println() // newline after hidden input
		if err != nil {
			return fmt.Errorf("failed to read secret value: %w", err)
		}
		value = raw
	}

	engineAddr := types.GetCLIEngineEndpoint(configPath)
	if engineAddr == "" {
		return fmt.Errorf("engine endpoint not configured. Run 'banyan-cli init' to configure")
	}
	client, err := NewAutoEngineClient(engineAddr)
	if err != nil {
		return fmt.Errorf("failed to connect to engine: %w", err)
	}
	defer client.Close()

	if err := client.CreateSecret(cmd.Context(), name, value); err != nil {
		return fmt.Errorf("failed to create secret: %w", err)
	}
	fmt.Printf("Secret %q created.\n", name)
	return nil
}

func runSecretList(cmd *cobra.Command, _ []string) error {
	logging.Setup(nil)

	engineAddr := types.GetCLIEngineEndpoint(configPath)
	if engineAddr == "" {
		return fmt.Errorf("engine endpoint not configured. Run 'banyan-cli init' to configure")
	}
	client, err := NewAutoEngineClient(engineAddr)
	if err != nil {
		return fmt.Errorf("failed to connect to engine: %w", err)
	}
	defer client.Close()

	secrets, err := client.ListSecrets(cmd.Context())
	if err != nil {
		return fmt.Errorf("failed to list secrets: %w", err)
	}

	if len(secrets) == 0 {
		fmt.Println("No secrets found.")
		return nil
	}

	fmt.Printf("%-30s %-20s %-20s\n", "NAME", "CREATED", "UPDATED")
	fmt.Println(strings.Repeat("-", 72))
	for _, s := range secrets {
		created := formatSecretTime(s.CreatedAt)
		updated := formatSecretTime(s.UpdatedAt)
		fmt.Printf("%-30s %-20s %-20s\n", s.Name, created, updated)
	}
	return nil
}

func runSecretGet(cmd *cobra.Command, args []string) error {
	logging.Setup(nil)
	name := args[0]
	reveal, _ := cmd.Flags().GetBool("reveal")

	engineAddr := types.GetCLIEngineEndpoint(configPath)
	if engineAddr == "" {
		return fmt.Errorf("engine endpoint not configured. Run 'banyan-cli init' to configure")
	}
	client, err := NewAutoEngineClient(engineAddr)
	if err != nil {
		return fmt.Errorf("failed to connect to engine: %w", err)
	}
	defer client.Close()

	resp, err := client.GetSecret(cmd.Context(), name, reveal)
	if err != nil {
		return fmt.Errorf("failed to get secret: %w", err)
	}

	fmt.Printf("Secret: %s\n", resp.Name)
	fmt.Printf("  Created:  %s\n", resp.CreatedAt)
	fmt.Printf("  Updated:  %s\n", resp.UpdatedAt)
	if reveal && len(resp.Value) > 0 {
		fmt.Printf("  Value:    %s\n", string(resp.Value))
	}
	return nil
}

func runSecretDelete(cmd *cobra.Command, args []string) error {
	logging.Setup(nil)
	name := args[0]

	engineAddr := types.GetCLIEngineEndpoint(configPath)
	if engineAddr == "" {
		return fmt.Errorf("engine endpoint not configured. Run 'banyan-cli init' to configure")
	}
	client, err := NewAutoEngineClient(engineAddr)
	if err != nil {
		return fmt.Errorf("failed to connect to engine: %w", err)
	}
	defer client.Close()

	if err := client.DeleteSecret(cmd.Context(), name); err != nil {
		return fmt.Errorf("%w", err)
	}
	fmt.Printf("Secret %q deleted.\n", name)
	return nil
}

// formatSecretTime parses RFC3339 and formats as relative time.
func formatSecretTime(s string) string {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return s
	}
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return fmt.Sprintf("%ds ago", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd ago", int(d.Hours()/24))
	}
}
