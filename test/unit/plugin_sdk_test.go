package unit

import (
	"testing"

	"github.com/fertile-org/banyan/pkg/plugin-sdk"
	"github.com/stretchr/testify/assert"
)

func TestPluginSDK(t *testing.T) {
	plugin := sdk.NewPlugin("test-plugin", "1.0.0")

	assert.Equal(t, "test-plugin", plugin.Name(), "Plugin name should match")
	assert.Equal(t, "1.0.0", plugin.Version(), "Plugin version should match")
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
		t.Run(err.Error(), func(t *testing.T) {
			assert.NotNil(t, err, "Error %d should not be nil", i)
			assert.NotEmpty(t, err.Error(), "Error %d should have a message", i)
		})
	}
}