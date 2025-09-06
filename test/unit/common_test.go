package unit

import (
	"testing"

	"github.com/fertile-org/banyan/internal/common"
	"github.com/stretchr/testify/assert"
)

func TestVersion(t *testing.T) {
	version := common.Version
	
	assert.NotEmpty(t, version, "Version should not be empty")
	assert.Equal(t, "0.1.0-dev", version, "Expected version to be '0.1.0-dev'")
}

func TestInitLogger(t *testing.T) {
	// Should not panic
	assert.NotPanics(t, func() {
		common.InitLogger()
	}, "InitLogger should not panic")
}