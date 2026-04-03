package runtimeview

import (
	"errors"
	"path/filepath"
	"testing"

	"github.com/accelerator-toolkit/accelerator-toolkit/pkg/config"
)

func TestLoad_WithProfile(t *testing.T) {
	view, err := Load("", filepath.Join("..", "..", "profiles", "iluvatar-bi-v150.yaml"))
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}

	if view.Profile() == nil {
		t.Fatal("expected loaded profile")
	}
	if view.HandlerName() != "iluvatar-bi-v150" {
		t.Fatalf("HandlerName = %q, want %q", view.HandlerName(), "iluvatar-bi-v150")
	}
	if view.HookPath() != "/usr/local/bin/accelerator-container-hook" {
		t.Fatalf("HookPath = %q, want %q", view.HookPath(), "/usr/local/bin/accelerator-container-hook")
	}
}

func TestLoad_RequiresProfile(t *testing.T) {
	_, err := Load("", "")
	if err == nil {
		t.Fatal("Load should fail when no active profile is available")
	}
	if !errors.Is(err, config.ErrProfileRequired) {
		t.Fatalf("Load error = %v, want ErrProfileRequired", err)
	}
}
