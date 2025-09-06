package unit

import (
	"testing"

	"github.com/fertile-org/banyan/internal/common"
)

func TestVersion(t *testing.T) {
	version := common.Version
	if version == "" {
		t.Fatal("Version should not be empty")
	}
	
	if version != "0.1.0-dev" {
		t.Errorf("Expected version '0.1.0-dev', got '%s'", version)
	}
}

func TestInitLogger(t *testing.T) {
	// Should not panic
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("InitLogger panicked: %v", r)
		}
	}()
	
	common.InitLogger()
}