package dashboard

import (
	"strings"
	"testing"
)

func TestRenderConfirmDialog_YesNo(t *testing.T) {
	state := confirmState{
		message: "Kill web-app-0? [y/n]",
	}
	got := renderConfirmDialog(state, 50)

	if !strings.Contains(got, "Kill web-app-0") {
		t.Error("dialog missing message")
	}
	if !strings.Contains(got, "Yes") {
		t.Error("dialog missing Yes option")
	}
	if !strings.Contains(got, "No") {
		t.Error("dialog missing No option")
	}
	if !strings.Contains(got, "Confirm") {
		t.Error("dialog missing title")
	}
}

func TestRenderConfirmDialog_TypeToConfirm(t *testing.T) {
	state := confirmState{
		message:  "Teardown deployment web-app?",
		typeName: "web-app",
		input:    "web",
	}
	got := renderConfirmDialog(state, 50)

	if !strings.Contains(got, "Teardown deployment web-app") {
		t.Error("dialog missing message")
	}
	if !strings.Contains(got, "web-app") {
		t.Error("dialog missing type name")
	}
	if !strings.Contains(got, "web") {
		t.Error("dialog missing input")
	}
	if !strings.Contains(got, "Confirm") {
		t.Error("dialog missing Confirm hint")
	}
}
