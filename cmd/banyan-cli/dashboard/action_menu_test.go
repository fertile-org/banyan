package dashboard

import (
	"strings"
	"testing"
)

func TestContainerActions(t *testing.T) {
	actions := containerActions()
	if len(actions) != 4 {
		t.Fatalf("expected 4 container actions, got %d", len(actions))
	}
	if actions[0].key != "kill" {
		t.Errorf("first action key = %q, want kill", actions[0].key)
	}
	if actions[1].key != "restart" {
		t.Errorf("second action key = %q, want restart", actions[1].key)
	}
	if actions[2].key != "logs" {
		t.Errorf("third action key = %q, want logs", actions[2].key)
	}
	if actions[3].key != "cancel" {
		t.Errorf("fourth action key = %q, want cancel", actions[3].key)
	}
}

func TestDeploymentActions(t *testing.T) {
	actions := deploymentActions()
	if len(actions) != 3 {
		t.Fatalf("expected 3 deployment actions, got %d", len(actions))
	}
	if actions[0].key != "teardown" {
		t.Errorf("first action key = %q, want teardown", actions[0].key)
	}
	if actions[1].key != "scale" {
		t.Errorf("second action key = %q, want scale", actions[1].key)
	}
	if actions[2].key != "cancel" {
		t.Errorf("third action key = %q, want cancel", actions[2].key)
	}
}

func TestRenderActionMenu(t *testing.T) {
	items := containerActions()
	got := renderActionMenu("web-app-0", items, 1, 50)

	if !strings.Contains(got, "web-app-0") {
		t.Error("menu missing target name")
	}
	if !strings.Contains(got, "Kill") {
		t.Error("menu missing Kill action")
	}
	if !strings.Contains(got, "Restart") {
		t.Error("menu missing Restart action")
	}
	if !strings.Contains(got, ">") {
		t.Error("menu missing cursor indicator")
	}
	if !strings.Contains(got, "Navigate") {
		t.Error("menu missing navigation hint")
	}
}

func TestRenderActionMenu_EmptyItems(t *testing.T) {
	got := renderActionMenu("test", nil, 0, 40)
	if !strings.Contains(got, "test") {
		t.Error("menu missing target name with empty items")
	}
}
