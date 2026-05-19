package cdi

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/accelerator-toolkit/accelerator-toolkit/pkg/profile"
)

func TestRenderPreviewSpec(t *testing.T) {
	p, err := profile.Load(filepath.Join("..", "..", "profiles", "metax-c500.yaml"))
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}

	data, err := RenderPreviewSpec(p, "")
	if err != nil {
		t.Fatalf("RenderPreviewSpec returned error: %v", err)
	}

	out := string(data)
	if !strings.Contains(out, "cdiVersion: 0.8.0") {
		t.Fatalf("rendered CDI spec missing version: %s", out)
	}
	if !strings.Contains(out, "kind: metax-tech.com/gpu") {
		t.Fatalf("rendered CDI spec missing kind: %s", out)
	}
	if !strings.Contains(out, "name: all") {
		t.Fatalf("rendered CDI spec missing default device name: %s", out)
	}
	if !strings.Contains(out, "METAX_VISIBLE_DEVICES=all") {
		t.Fatalf("rendered CDI spec missing selector env: %s", out)
	}
	if !strings.Contains(out, "MACA_PATH=/opt/maca") {
		t.Fatalf("rendered CDI spec missing extra env: %s", out)
	}
	if !strings.Contains(out, "hostPath: /dev/mxcd") {
		t.Fatalf("rendered CDI spec missing control device node: %s", out)
	}
	if !strings.Contains(out, "containerPath: /opt/maca/ompi/lib") {
		t.Fatalf("rendered CDI spec missing derived mount target: %s", out)
	}
	if !strings.Contains(out, "- rbind") || !strings.Contains(out, "- ro") {
		t.Fatalf("rendered CDI spec missing mount options: %s", out)
	}
}

func TestRenderPreviewSpec_CustomDeviceName(t *testing.T) {
	p, err := profile.Load(filepath.Join("..", "..", "profiles", "metax-c500.yaml"))
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}

	data, err := RenderPreviewSpec(p, "GPU-aaaa")
	if err != nil {
		t.Fatalf("RenderPreviewSpec returned error: %v", err)
	}

	out := string(data)
	if !strings.Contains(out, "name: GPU-aaaa") {
		t.Fatalf("rendered CDI spec missing custom device name: %s", out)
	}
}

func TestRenderPreviewSpec_NilProfile(t *testing.T) {
	_, err := RenderPreviewSpec(nil, "")
	if err == nil {
		t.Fatal("expected error for nil profile")
	}
}
