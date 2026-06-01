package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"syscall"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	banyanpb "github.com/fertile-org/banyan/pkg/rpc/banyanpb"
)

// credentials stores JWT tokens on disk for the CLI.
type credentials struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	EngineAddr   string `json:"engine_addr"`
	Username     string `json:"username"`
	Role         string `json:"role"`
}

var credentialsPath = filepath.Join(os.Getenv("HOME"), ".config", "banyan", "credentials.json")

var loginCmd = &cobra.Command{
	Use:   "login",
	Short: "Authenticate with the Banyan engine",
	Long: `Authenticate with the Banyan engine using username and password.

Stores a session token locally so subsequent commands work without re-authenticating.
The token auto-refreshes when it expires (7-day session lifetime).

Example:
  banyan login`,
	RunE: runAuthLogin,
}

var logoutCmd = &cobra.Command{
	Use:   "logout",
	Short: "Clear stored credentials",
	RunE:  runLogout,
}

var whoamiCmd = &cobra.Command{
	Use:   "whoami",
	Short: "Show current identity and role",
	RunE:  runWhoami,
}

func init() {
	loginCmd.Flags().String("username", "", "Username (non-interactive login)")
	loginCmd.Flags().String("password", "", "Password (non-interactive login)")
	rootCmd.AddCommand(loginCmd)
	rootCmd.AddCommand(logoutCmd)
	rootCmd.AddCommand(whoamiCmd)
}

func runAuthLogin(cmd *cobra.Command, args []string) error {
	flagUser, _ := cmd.Flags().GetString("username")
	flagPass, _ := cmd.Flags().GetString("password")

	var username, password string

	if flagUser != "" && flagPass != "" {
		// Non-interactive: both flags supplied
		username = flagUser
		password = flagPass
	} else {
		fmt.Print("Username: ")
		if _, err := fmt.Scanln(&username); err != nil {
			return fmt.Errorf("failed to read username: %w", err)
		}
		if username == "" {
			return fmt.Errorf("username cannot be empty")
		}

		fmt.Print("Password: ")
		passwordBytes, err := term.ReadPassword(syscall.Stdin)
		fmt.Println() // newline after hidden input
		if err != nil {
			return fmt.Errorf("failed to read password: %w", err)
		}
		password = string(passwordBytes)
		if password == "" {
			return fmt.Errorf("password cannot be empty")
		}
	}

	client, err := NewAutoEngineClient("")
	if err != nil {
		return fmt.Errorf("failed to connect to engine: %w", err)
	}
	defer client.Close()

	resp, err := client.GRPCClient().Login(cmd.Context(), &banyanpb.LoginRequest{
		Username: username,
		Password: password,
	})
	if err != nil {
		return fmt.Errorf("login failed: %v", err)
	}

	creds := credentials{
		AccessToken:  resp.AccessToken,
		RefreshToken: resp.RefreshToken,
		Username:     resp.Username,
		Role:         resp.Role,
	}

	if err := saveCredentials(&creds); err != nil {
		return fmt.Errorf("login succeeded but failed to save credentials: %w", err)
	}

	fmt.Printf("Logged in as %s (role: %s)\n", resp.Username, resp.Role)
	return nil
}

func runLogout(cmd *cobra.Command, args []string) error {
	creds, err := loadCredentials()
	if err != nil {
		// No credentials to clear
		fmt.Println("Not logged in.")
		return nil
	}

	// Revoke refresh token on engine
	if creds.RefreshToken != "" {
		client, err := NewAutoEngineClient("")
		if err == nil {
			_, _ = client.GRPCClient().RefreshToken(cmd.Context(), &banyanpb.RefreshTokenRequest{
				RefreshToken: creds.RefreshToken,
			})
			client.Close()
		}
	}

	_ = os.Remove(credentialsPath)
	fmt.Println("Logged out.")
	return nil
}

func runWhoami(cmd *cobra.Command, args []string) error {
	creds, err := loadCredentials()
	if err != nil {
		return fmt.Errorf("not logged in. Run: banyan login")
	}
	fmt.Printf("Username: %s\nRole:     %s\n", creds.Username, creds.Role)
	return nil
}

func saveCredentials(creds *credentials) error {
	dir := filepath.Dir(credentialsPath)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(creds, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(credentialsPath, data, 0o600)
}

func loadCredentials() (*credentials, error) {
	data, err := os.ReadFile(credentialsPath)
	if err != nil {
		return nil, err
	}
	var creds credentials
	if err := json.Unmarshal(data, &creds); err != nil {
		return nil, err
	}
	return &creds, nil
}
