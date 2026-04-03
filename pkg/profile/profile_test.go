package profile

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoad_ValidProfile(t *testing.T) {
	profilePath := filepath.Join("..", "..", "profiles", "iluvatar-bi-v150.yaml")

	p, err := Load(profilePath)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}

	if p.Metadata.Name != "iluvatar-bi-v150" {
		t.Fatalf("Metadata.Name = %q, want %q", p.Metadata.Name, "iluvatar-bi-v150")
	}
	if p.Runtime.HandlerName != "iluvatar-bi-v150" {
		t.Fatalf("Runtime.HandlerName = %q, want %q", p.Runtime.HandlerName, "iluvatar-bi-v150")
	}
	if got := p.Device.SelectorEnvVars[0]; got != "ILUVATAR_COREX_VISIBLE_DEVICES" {
		t.Fatalf("Device.SelectorEnvVars[0] = %q, want %q", got, "ILUVATAR_COREX_VISIBLE_DEVICES")
	}
	if len(p.Inject.Artifacts) != 3 {
		t.Fatalf("len(Inject.Artifacts) = %d, want 3", len(p.Inject.Artifacts))
	}
}

func TestLoad_Ascend910BProfile(t *testing.T) {
	profilePath := filepath.Join("..", "..", "profiles", "ascend-910b.yaml")

	p, err := Load(profilePath)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}

	if p.Metadata.Name != "ascend-910b" {
		t.Fatalf("Metadata.Name = %q, want %q", p.Metadata.Name, "ascend-910b")
	}
	if got := p.Kubernetes.ResourceNames[0]; got != "huawei.com/Ascend910" {
		t.Fatalf("Kubernetes.ResourceNames[0] = %q, want %q", got, "huawei.com/Ascend910")
	}
	if got := p.Device.SelectorEnvVars[0]; got != "ASCEND_VISIBLE_DEVICES" {
		t.Fatalf("Device.SelectorEnvVars[0] = %q, want %q", got, "ASCEND_VISIBLE_DEVICES")
	}
	if len(p.Device.ControlDeviceGlobs) != 3 {
		t.Fatalf("len(Device.ControlDeviceGlobs) = %d, want 3", len(p.Device.ControlDeviceGlobs))
	}
}

func TestLoad_RejectsInvalidProfile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.yaml")
	content := `
metadata:
  name: bad
runtime:
  handlerName: test-handler
`
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	_, err := Load(path)
	if err == nil {
		t.Fatal("Load should fail for invalid profile")
	}
	if !strings.Contains(err.Error(), "metadata.vendor is required") {
		t.Fatalf("Load error = %q, want metadata.vendor validation", err)
	}
}

func TestLoad_RejectsUnknownField(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "unknown.yaml")
	content := `
metadata:
  name: ok
  vendor: vendor
  version: v1alpha1
runtime:
  handlerName: vendor-runtime
  runtimeClassName: vendor-runtime
  underlyingRuntime: runc
  hookStage: prestart
  hookBinary: /usr/local/bin/accelerator-container-hook
kubernetes:
  resourceNames:
    - vendor.com/gpu
device:
  selectorEnvVars:
    - GPU_VISIBLE_DEVICES
  selectorFormats:
    - index-list
  deviceGlobs:
    - /dev/vendor*
  mapping:
    strategy:
      primary: command
    command:
      pathCandidates:
        - vendor-smi
      args:
        - --query
    parser: csv
inject:
  containerRoot: /usr/local/vendor
  artifacts:
    - name: libs
      kind: shared-libraries
      hostPaths:
        - /usr/local/vendor/lib
      containerPath: /usr/local/vendor
      mode: so-only
      typoField: true
  linker:
    configPath: /etc/ld.so.conf.d/vendor.conf
`
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	_, err := Load(path)
	if err == nil {
		t.Fatal("Load should fail for unknown field")
	}
	if !strings.Contains(err.Error(), "typoField") {
		t.Fatalf("Load error = %q, want unknown field details", err)
	}
}

func TestValidate_RejectsInvalidArtifactMode(t *testing.T) {
	p := &Profile{
		Metadata: Metadata{
			Name:    "test",
			Vendor:  "vendor",
			Version: "v1alpha1",
		},
		Runtime: Runtime{
			HandlerName:       "vendor-runtime",
			RuntimeClassName:  "vendor-runtime",
			UnderlyingRuntime: "runc",
			HookStage:         HookStagePrestart,
			HookBinary:        "/usr/local/bin/hook",
		},
		Kubernetes: Kubernetes{
			ResourceNames: []string{"vendor.com/gpu"},
		},
		Device: Device{
			SelectorEnvVars: []string{"GPU_VISIBLE_DEVICES"},
			DeviceGlobs:     []string{"/dev/vendor*"},
			Mapping: DeviceMapping{
				Strategy: MappingStrategy{
					Primary: "command",
				},
				Command: MappingCommand{
					PathCandidates: []string{"vendor-smi"},
					Args:           []string{"--query"},
				},
				Parser: "csv",
			},
		},
		Inject: Inject{
			ContainerRoot: "/usr/local/vendor",
			Artifacts: []Artifact{
				{
					Name:          "libs",
					Kind:          "shared-libraries",
					HostPaths:     []string{"/usr/local/vendor/lib"},
					ContainerPath: "/usr/local/vendor",
					Mode:          "invalid",
				},
			},
			Linker: Linker{
				ConfigPath: "/etc/ld.so.conf.d/vendor.conf",
			},
		},
	}

	err := p.Validate()
	if err == nil {
		t.Fatal("Validate should fail for invalid artifact mode")
	}
	if !strings.Contains(err.Error(), `mode "invalid" is invalid`) {
		t.Fatalf("Validate error = %q, want invalid mode", err)
	}
}

func TestValidate_AllowsIndexOnlyProfileWithoutMappingCommand(t *testing.T) {
	p := &Profile{
		Metadata: Metadata{
			Name:    "ascend-910b",
			Vendor:  "Ascend",
			Version: "v1alpha1",
		},
		Runtime: Runtime{
			HandlerName:       "ascend-910b",
			RuntimeClassName:  "ascend-910b",
			UnderlyingRuntime: "runc",
			HookStage:         HookStagePrestart,
			HookBinary:        "/usr/local/bin/accelerator-container-hook",
		},
		Kubernetes: Kubernetes{
			ResourceNames: []string{"huawei.com/Ascend910"},
		},
		Device: Device{
			SelectorEnvVars: []string{"ASCEND_VISIBLE_DEVICES"},
			DeviceGlobs:     []string{"/dev/davinci*"},
			Mapping: DeviceMapping{
				Strategy: MappingStrategy{
					Primary:  "env-index-list",
					Fallback: "none",
				},
			},
		},
		Inject: Inject{
			ContainerRoot: "/usr/local/Ascend",
			Artifacts: []Artifact{
				{
					Name:          "device-nodes",
					Kind:          "device-nodes",
					HostPaths:     []string{"/dev/davinci*"},
					ContainerPath: "/dev",
					Mode:          "bind",
				},
			},
			Linker: Linker{
				ConfigPath: "/etc/ld.so.conf.d/accelerator-toolkit.conf",
			},
		},
	}

	if err := p.Validate(); err != nil {
		t.Fatalf("Validate returned error: %v", err)
	}
}

func TestRenderRuntimeClassYAML(t *testing.T) {
	p, err := Load(filepath.Join("..", "..", "profiles", "iluvatar-bi-v150.yaml"))
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}

	data, err := p.RenderRuntimeClassYAML()
	if err != nil {
		t.Fatalf("RenderRuntimeClassYAML returned error: %v", err)
	}

	out := string(data)
	if !strings.Contains(out, "kind: RuntimeClass") {
		t.Fatalf("rendered manifest missing kind: %s", out)
	}
	if !strings.Contains(out, "name: iluvatar-bi-v150") {
		t.Fatalf("rendered manifest missing runtimeClass name: %s", out)
	}
	if !strings.Contains(out, "handler: iluvatar-bi-v150") {
		t.Fatalf("rendered manifest missing handler: %s", out)
	}
	if !strings.Contains(out, "iluvatar.ai/gpu: present") {
		t.Fatalf("rendered manifest missing node selector label: %s", out)
	}
}

func TestRenderDaemonSetYAML(t *testing.T) {
	p, err := Load(filepath.Join("..", "..", "profiles", "iluvatar-bi-v150.yaml"))
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}

	data, err := p.RenderDaemonSetYAML("docker.io/accelerator-toolkit/installer:latest", "profiles/iluvatar-bi-v150.yaml")
	if err != nil {
		t.Fatalf("RenderDaemonSetYAML returned error: %v", err)
	}

	out := string(data)
	if !strings.Contains(out, "kind: DaemonSet") {
		t.Fatalf("rendered manifest missing kind: %s", out)
	}
	if !strings.Contains(out, "image: docker.io/accelerator-toolkit/installer:latest") {
		t.Fatalf("rendered manifest missing installer image: %s", out)
	}
	if !strings.Contains(out, "ACCELERATOR_PROFILE_FILE") {
		t.Fatalf("rendered manifest missing ACCELERATOR_PROFILE_FILE env: %s", out)
	}
	if !strings.Contains(out, "iluvatar-bi-v150.yaml") {
		t.Fatalf("rendered manifest missing profile file name: %s", out)
	}
	if !strings.Contains(out, "iluvatar.ai/gpu: present") {
		t.Fatalf("rendered manifest missing nodeSelector label: %s", out)
	}
	if strings.Contains(out, "IX_DRIVER_LIB_PATHS") || strings.Contains(out, "IX_DRIVER_BIN_PATHS") {
		t.Fatalf("rendered manifest should not hardcode legacy driver path envs: %s", out)
	}
	if strings.Contains(out, "NoExecute") {
		t.Fatalf("rendered manifest should prefer profile tolerations over generic defaults: %s", out)
	}
}

func TestRenderBundleYAML(t *testing.T) {
	p, err := Load(filepath.Join("..", "..", "profiles", "iluvatar-bi-v150.yaml"))
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}

	data, err := p.RenderBundleYAML("docker.io/accelerator-toolkit/installer:latest", "profiles/iluvatar-bi-v150.yaml")
	if err != nil {
		t.Fatalf("RenderBundleYAML returned error: %v", err)
	}

	out := string(data)
	if !strings.Contains(out, "kind: ServiceAccount") {
		t.Fatalf("bundle missing ServiceAccount: %s", out)
	}
	if !strings.Contains(out, "kind: RuntimeClass") {
		t.Fatalf("bundle missing RuntimeClass: %s", out)
	}
	if !strings.Contains(out, "kind: DaemonSet") {
		t.Fatalf("bundle missing DaemonSet: %s", out)
	}
}
