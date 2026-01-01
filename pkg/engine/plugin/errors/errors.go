// Package errors defines error types for the Plugin Manager.
package errors

import "errors"

var (
	// ErrPluginNotFound is returned when a plugin is not found.
	ErrPluginNotFound = errors.New("plugin not found")

	// ErrPluginAlreadyExists is returned when trying to register a duplicate plugin.
	ErrPluginAlreadyExists = errors.New("plugin already exists")

	// ErrPluginDisabled is returned when trying to execute a disabled plugin.
	ErrPluginDisabled = errors.New("plugin is disabled")

	// ErrPluginTimeout is returned when plugin execution times out.
	ErrPluginTimeout = errors.New("plugin execution timeout")

	// ErrInvalidPlugin is returned when plugin configuration is invalid.
	ErrInvalidPlugin = errors.New("invalid plugin configuration")

	// ErrNoRunnerAvailable is returned when no runner can handle the plugin type.
	ErrNoRunnerAvailable = errors.New("no runner available for plugin type")

	// ErrBuiltinNotFound is returned when a builtin plugin is not found.
	ErrBuiltinNotFound = errors.New("builtin plugin not found")

	// ErrWebhookFailed is returned when webhook execution fails.
	ErrWebhookFailed = errors.New("webhook execution failed")

	// ErrGRPCFailed is returned when gRPC execution fails.
	ErrGRPCFailed = errors.New("gRPC execution failed")
)
