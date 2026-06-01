package cmd

import (
	"fmt"
	"os"
	"syscall"
	"text/tabwriter"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	banyanpb "github.com/fertile-org/banyan/pkg/rpc/banyanpb"
)

var userRole string

var userCmd = &cobra.Command{
	Use:   "user",
	Short: "Manage users",
	Long:  "Manage Banyan users. Requires admin role.",
}

var userAddCmd = &cobra.Command{
	Use:   "add <username>",
	Short: "Create a new user",
	Args:  cobra.ExactArgs(1),
	RunE:  runUserAdd,
}

var userListCmd = &cobra.Command{
	Use:     "list",
	Short:   "List all users",
	Aliases: []string{"ls"},
	RunE:    runUserList,
}

var userRemoveCmd = &cobra.Command{
	Use:     "remove <username>",
	Aliases: []string{"rm"},
	Short:   "Delete a user",
	Args:    cobra.ExactArgs(1),
	RunE:    runUserRemove,
}

var userSetRoleCmd = &cobra.Command{
	Use:   "set-role <username> <role>",
	Short: "Change a user's role (admin, deployer, viewer)",
	Args:  cobra.ExactArgs(2),
	RunE:  runUserSetRole,
}

var changePasswordCmd = &cobra.Command{
	Use:   "change-password",
	Short: "Change your own password",
	RunE:  runChangePassword,
}

func init() {
	userAddCmd.Flags().StringVarP(&userRole, "role", "r", "viewer", "Role: admin, deployer, viewer")
	userCmd.AddCommand(userAddCmd, userListCmd, userRemoveCmd, userSetRoleCmd)
	rootCmd.AddCommand(userCmd)
	rootCmd.AddCommand(changePasswordCmd)
}

func runUserAdd(cmd *cobra.Command, args []string) error {
	username := args[0]

	fmt.Printf("Password for %s: ", username)
	passwordBytes, err := term.ReadPassword(syscall.Stdin)
	fmt.Println()
	if err != nil {
		return fmt.Errorf("failed to read password: %w", err)
	}
	password := string(passwordBytes)
	if password == "" {
		return fmt.Errorf("password cannot be empty")
	}

	client, err := NewAutoEngineClient("")
	if err != nil {
		return fmt.Errorf("failed to connect to engine: %w", err)
	}
	defer client.Close()

	_, err = client.GRPCClient().CreateUser(cmd.Context(), &banyanpb.CreateUserRequest{
		Username: username,
		Password: password,
		Role:     userRole,
	})
	if err != nil {
		return fmt.Errorf("failed to create user: %v", err)
	}

	fmt.Printf("User %q created with role %q\n", username, userRole)
	return nil
}

func runUserList(cmd *cobra.Command, args []string) error {
	client, err := NewAutoEngineClient("")
	if err != nil {
		return fmt.Errorf("failed to connect to engine: %w", err)
	}
	defer client.Close()

	resp, err := client.GRPCClient().ListUsers(cmd.Context(), &banyanpb.ListUsersRequest{})
	if err != nil {
		return fmt.Errorf("failed to list users: %v", err)
	}

	if len(resp.Users) == 0 {
		fmt.Println("No users found.")
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "USERNAME\tROLE\tCREATED\tCREATED BY\tSTATUS")
	for _, u := range resp.Users {
		status := "active"
		if u.Disabled {
			status = "disabled"
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n", u.Username, u.Role, u.CreatedAt, u.CreatedBy, status)
	}
	return w.Flush()
}

func runUserRemove(cmd *cobra.Command, args []string) error {
	username := args[0]
	client, err := NewAutoEngineClient("")
	if err != nil {
		return fmt.Errorf("failed to connect to engine: %w", err)
	}
	defer client.Close()

	_, err = client.GRPCClient().DeleteUser(cmd.Context(), &banyanpb.DeleteUserRequest{
		Username: username,
	})
	if err != nil {
		return fmt.Errorf("failed to delete user: %v", err)
	}

	fmt.Printf("User %q deleted\n", username)
	return nil
}

func runUserSetRole(cmd *cobra.Command, args []string) error {
	username, role := args[0], args[1]
	client, err := NewAutoEngineClient("")
	if err != nil {
		return fmt.Errorf("failed to connect to engine: %w", err)
	}
	defer client.Close()

	_, err = client.GRPCClient().UpdateUserRole(cmd.Context(), &banyanpb.UpdateUserRoleRequest{
		Username: username,
		Role:     role,
	})
	if err != nil {
		return fmt.Errorf("failed to update role: %v", err)
	}

	fmt.Printf("User %q role updated to %q\n", username, role)
	return nil
}

func runChangePassword(cmd *cobra.Command, args []string) error {
	fmt.Print("Current password: ")
	currentBytes, err := term.ReadPassword(syscall.Stdin)
	fmt.Println()
	if err != nil {
		return fmt.Errorf("failed to read current password: %w", err)
	}

	fmt.Print("New password: ")
	newBytes, err := term.ReadPassword(syscall.Stdin)
	fmt.Println()
	if err != nil {
		return fmt.Errorf("failed to read new password: %w", err)
	}

	if len(newBytes) == 0 {
		return fmt.Errorf("new password cannot be empty")
	}

	client, err := NewAutoEngineClient("")
	if err != nil {
		return fmt.Errorf("failed to connect to engine: %w", err)
	}
	defer client.Close()

	_, err = client.GRPCClient().ChangePassword(cmd.Context(), &banyanpb.ChangePasswordRequest{
		CurrentPassword: string(currentBytes),
		NewPassword:     string(newBytes),
	})
	if err != nil {
		return fmt.Errorf("failed to change password: %v", err)
	}

	fmt.Println("Password changed successfully.")
	return nil
}
