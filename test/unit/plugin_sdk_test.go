package unit

import (
	"testing"

	"github.com/fertile-org/banyan/pkg/interfaces"
	"github.com/fertile-org/banyan/pkg/plugin-sdk"
)

func TestPluginSDK(t *testing.T) {
	plugin := sdk.NewPlugin("test-plugin", "1.0.0")

	if plugin.Name() != "test-plugin" {
		t.Errorf("Expected name 'test-plugin', got '%s'", plugin.Name())
	}

	if plugin.Version() != "1.0.0" {
		t.Errorf("Expected version '1.0.0', got '%s'", plugin.Version())
	}
}

func TestSDKErrors(t *testing.T) {
	errors := []error{
		sdk.ErrMissingDeploymentID,
		sdk.ErrMissingDeploymentName,
		sdk.ErrInvalidConfiguration,
		sdk.ErrProviderNotSupported,
		sdk.ErrDeploymentFailed,
		sdk.ErrDeploymentNotFound,
	}

	for i, err := range errors {
		if err == nil {
			t.Errorf("Error %d should not be nil", i)
		}
		if err.Error() == "" {
			t.Errorf("Error %d should have a message", i)
		}
	}
}