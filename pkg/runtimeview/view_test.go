package runtimeview

import (
	"errors"
	"path/filepath"
	"testing"

	"github.com/accelerator-toolkit/accelerator-toolkit/pkg/config"
	"github.com/accelerator-toolkit/accelerator-toolkit/pkg/profile"
)

func TestLoad_WithProfile(t *testing.T) {
	view, err := Load("", filepath.Join("..", "..", "profiles", "iluvatar-bi-v150.yaml"))
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}

	if view.Profile() == nil {
		t.Fatal("expected loaded profile")
	}
	if view.HandlerName() != "xpu-runtime" {
		t.Fatalf("HandlerName = %q, want %q", view.HandlerName(), "xpu-runtime")
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

func TestDefaultSelectorValue_EnvAll(t *testing.T) {
	view := New(&config.Config{}, &profile.Profile{
		Device: profile.Device{
			SelectorFormats: []string{"all", "none"},
			Mapping: profile.DeviceMapping{
				Strategy: profile.MappingStrategy{Primary: "env-all"},
			},
		},
	})

	if got := view.DefaultSelectorValue(); got != "all" {
		t.Fatalf("DefaultSelectorValue = %q, want %q", got, "all")
	}
}

func TestDefaultSelectorValue_NonEnvAll(t *testing.T) {
	view := New(&config.Config{}, &profile.Profile{
		Device: profile.Device{
			SelectorFormats: []string{"all", "none"},
			Mapping: profile.DeviceMapping{
				Strategy: profile.MappingStrategy{Primary: "env-index-list"},
			},
		},
	})

	if got := view.DefaultSelectorValue(); got != "" {
		t.Fatalf("DefaultSelectorValue = %q, want empty", got)
	}
}

func TestDelegateOnly(t *testing.T) {
	view := New(&config.Config{}, &profile.Profile{
		Runtime: profile.Runtime{InjectMode: profile.InjectModeDelegateOnly},
	})

	if !view.DelegateOnly() {
		t.Fatal("DelegateOnly = false, want true")
	}
}
