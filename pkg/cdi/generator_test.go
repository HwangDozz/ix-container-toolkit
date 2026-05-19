package cdi

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/sirupsen/logrus"

	"github.com/accelerator-toolkit/accelerator-toolkit/pkg/device"
	"github.com/accelerator-toolkit/accelerator-toolkit/pkg/profile"
	"github.com/accelerator-toolkit/accelerator-toolkit/pkg/strutil"
)

func testLogger() *logrus.Logger {
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	return log
}

// setupTestProfile creates a test profile backed by real temp directories so that
// host-path resolution succeeds. Returns the profile and the temp root.
func setupTestProfile(t *testing.T) (*profile.Profile, string) {
	t.Helper()
	tmpDir := t.TempDir()

	// lib64 with .so files.
	libDir := filepath.Join(tmpDir, "lib64")
	if err := os.MkdirAll(libDir, 0755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"libtest.so", "libtest.so.1"} {
		if err := os.WriteFile(filepath.Join(libDir, name), []byte("fake"), 0644); err != nil {
			t.Fatal(err)
		}
	}

	// bin directory.
	binDir := filepath.Join(tmpDir, "bin")
	if err := os.MkdirAll(binDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(binDir, "testsmi"), []byte("fake"), 0755); err != nil {
		t.Fatal(err)
	}

	p := &profile.Profile{
		Metadata: profile.Metadata{
			Name:        "test-device",
			Vendor:      "test",
			ModelFamily: "gpu",
			Version:     "v1",
		},
		Runtime: profile.Runtime{
			UnderlyingRuntime: "runc",
			HookStage:         "prestart",
			HookBinary:        "/usr/local/bin/accelerator-container-hook",
		},
		Kubernetes: profile.Kubernetes{
			ResourceNames: []string{"test.com/gpu"},
			NodeLabels:    map[string]string{"accelerator": "test-gpu"},
		},
		Device: profile.Device{
			SelectorEnvVars:    []string{"TEST_VISIBLE_DEVICES"},
			SelectorFormats:    []string{"all", "index-list"},
			DeviceGlobs:        []string{"/dev/null"}, // dummy, not used by generator
			ControlDeviceGlobs: []string{},
			Mapping: profile.DeviceMapping{
				Strategy: profile.MappingStrategy{Primary: "env-index-list"},
			},
		},
		Inject: profile.Inject{
			ContainerRoot: tmpDir,
			Artifacts: []profile.Artifact{
				{
					Name:          "device-nodes",
					Kind:          "device-nodes",
					HostPaths:     []string{"/dev/null"},
					ContainerPath: "/dev",
					Mode:          "bind",
				},
				{
					Name:          "driver-libraries",
					Kind:          "shared-libraries",
					HostPaths:     []string{libDir},
					ContainerPath: tmpDir,
					Mode:          "so-only",
					ExcludeDirs:   []string{"python3"},
				},
				{
					Name:          "driver-binaries",
					Kind:          "directory",
					HostPaths:     []string{binDir},
					ContainerPath: tmpDir,
					Mode:          "bind",
				},
			},
			Linker: profile.Linker{
				Strategy:    "ldconfig",
				ConfigPath:  "/etc/ld.so.conf.d/accelerator-toolkit.conf",
				Paths:       []string{libDir},
				RunLdconfig: true,
			},
			ExtraEnv: map[string]string{
				"LD_LIBRARY_PATH": libDir,
				"PATH":            binDir + ":/usr/bin",
			},
		},
	}
	return p, tmpDir
}

func TestGenerate_Kind(t *testing.T) {
	p, _ := setupTestProfile(t)
	devs := []device.Device{
		{Path: "/dev/test0", Index: 0, UUID: "GPU-aaaa"},
	}
	gen := NewGenerator(p, devs, testLogger())

	spec, err := gen.Generate()
	if err != nil {
		t.Fatalf("Generate() error: %v", err)
	}

	if spec.CDIVersion != SpecVersion {
		t.Errorf("CDIVersion = %q, want %q", spec.CDIVersion, SpecVersion)
	}
	if spec.Kind != "test.com/gpu" {
		t.Errorf("Kind = %q, want %q", spec.Kind, "test.com/gpu")
	}
}

func TestGenerate_PerDeviceEntries(t *testing.T) {
	p, _ := setupTestProfile(t)
	devs := []device.Device{
		{Path: "/dev/test0", Index: 0, UUID: "GPU-aaaa"},
		{Path: "/dev/test1", Index: 1, UUID: "GPU-bbbb"},
	}
	gen := NewGenerator(p, devs, testLogger())

	spec, err := gen.Generate()
	if err != nil {
		t.Fatalf("Generate() error: %v", err)
	}

	if len(spec.Devices) != 2 {
		t.Fatalf("len(Devices) = %d, want 2", len(spec.Devices))
	}

	if spec.Devices[0].Name != "GPU-aaaa" {
		t.Errorf("Devices[0].Name = %q, want %q", spec.Devices[0].Name, "GPU-aaaa")
	}
	if spec.Devices[1].Name != "GPU-bbbb" {
		t.Errorf("Devices[1].Name = %q, want %q", spec.Devices[1].Name, "GPU-bbbb")
	}
}

func TestGenerate_DeviceFallbackToIndex(t *testing.T) {
	p, _ := setupTestProfile(t)
	devs := []device.Device{
		{Path: "/dev/test0", Index: 0},
	}
	gen := NewGenerator(p, devs, testLogger())

	spec, err := gen.Generate()
	if err != nil {
		t.Fatalf("Generate() error: %v", err)
	}

	if spec.Devices[0].Name != "0" {
		t.Errorf("Devices[0].Name = %q, want %q", spec.Devices[0].Name, "0")
	}
}

func TestGenerate_EnvVars(t *testing.T) {
	p, tmpDir := setupTestProfile(t)
	devs := []device.Device{
		{Path: "/dev/test0", Index: 0, UUID: "GPU-aaaa"},
	}
	gen := NewGenerator(p, devs, testLogger())

	spec, err := gen.Generate()
	if err != nil {
		t.Fatalf("Generate() error: %v", err)
	}

	env := spec.Devices[0].ContainerEdits.Env

	if len(env) == 0 {
		t.Fatal("Env is empty")
	}
	if env[0] != "TEST_VISIBLE_DEVICES=all" {
		t.Errorf("Env[0] = %q, want %q", env[0], "TEST_VISIBLE_DEVICES=all")
	}

	libDir := filepath.Join(tmpDir, "lib64")
	binDir := filepath.Join(tmpDir, "bin")
	foundLD := false
	foundPATH := false
	for _, e := range env {
		if e == "LD_LIBRARY_PATH="+libDir {
			foundLD = true
		}
		if e == "PATH="+binDir+":/usr/bin" {
			foundPATH = true
		}
	}
	if !foundLD {
		t.Error("missing LD_LIBRARY_PATH in env")
	}
	if !foundPATH {
		t.Error("missing PATH in env")
	}
}

func TestGenerate_DeviceNodes(t *testing.T) {
	p, _ := setupTestProfile(t)
	devs := []device.Device{
		{Path: "/dev/test0", Index: 0, UUID: "GPU-aaaa"},
	}
	gen := NewGenerator(p, devs, testLogger())

	spec, err := gen.Generate()
	if err != nil {
		t.Fatalf("Generate() error: %v", err)
	}

	nodes := spec.Devices[0].ContainerEdits.DeviceNodes
	if len(nodes) < 1 {
		t.Fatal("DeviceNodes is empty")
	}

	if nodes[0].Path != "/dev/test0" {
		t.Errorf("DeviceNodes[0].Path = %q, want %q", nodes[0].Path, "/dev/test0")
	}
	if nodes[0].Permissions != "rwm" {
		t.Errorf("DeviceNodes[0].Permissions = %q, want %q", nodes[0].Permissions, "rwm")
	}
}

func TestGenerate_SoOnlyMounts(t *testing.T) {
	tmpDir := t.TempDir()
	libDir := filepath.Join(tmpDir, "lib64")
	if err := os.MkdirAll(libDir, 0755); err != nil {
		t.Fatal(err)
	}

	for _, name := range []string{"libcuda.so", "libcuda.so.1", "libthunk.so.1.2", "readme.txt", "libcuda.a"} {
		if err := os.WriteFile(filepath.Join(libDir, name), []byte("fake"), 0644); err != nil {
			t.Fatal(err)
		}
	}

	subDir := filepath.Join(libDir, "nvvm")
	if err := os.MkdirAll(subDir, 0755); err != nil {
		t.Fatal(err)
	}

	excludedDir := filepath.Join(libDir, "python3")
	if err := os.MkdirAll(excludedDir, 0755); err != nil {
		t.Fatal(err)
	}

	binDir := filepath.Join(tmpDir, "bin")
	if err := os.MkdirAll(binDir, 0755); err != nil {
		t.Fatal(err)
	}

	p := &profile.Profile{
		Metadata: profile.Metadata{
			Name: "test", Vendor: "test", ModelFamily: "gpu", Version: "v1",
		},
		Runtime: profile.Runtime{
			UnderlyingRuntime: "runc", HookStage: "prestart", HookBinary: "/usr/local/bin/hook",
		},
		Kubernetes: profile.Kubernetes{
			ResourceNames: []string{"test.com/gpu"},
		},
		Device: profile.Device{
			SelectorEnvVars: []string{"TEST_VISIBLE_DEVICES"},
			SelectorFormats: []string{"all"},
			DeviceGlobs:     []string{"/dev/null"},
			Mapping: profile.DeviceMapping{
				Strategy: profile.MappingStrategy{Primary: "env-index-list"},
			},
		},
		Inject: profile.Inject{
			ContainerRoot: tmpDir,
			Artifacts: []profile.Artifact{
				{Name: "device-nodes", Kind: "device-nodes", HostPaths: []string{"/dev/null"}, ContainerPath: "/dev", Mode: "bind"},
				{Name: "driver-libraries", Kind: "shared-libraries", HostPaths: []string{libDir}, ContainerPath: tmpDir, Mode: "so-only", ExcludeDirs: []string{"python3"}},
				{Name: "driver-binaries", Kind: "directory", HostPaths: []string{binDir}, ContainerPath: tmpDir, Mode: "bind"},
			},
			Linker: profile.Linker{ConfigPath: "/etc/ld.so.conf.d/test.conf"},
		},
	}

	devs := []device.Device{{Path: "/dev/test0", Index: 0, UUID: "GPU-aaaa"}}
	gen := NewGenerator(p, devs, testLogger())

	spec, err := gen.Generate()
	if err != nil {
		t.Fatalf("Generate() error: %v", err)
	}

	mounts := spec.Devices[0].ContainerEdits.Mounts
	mountPaths := make(map[string]bool)
	for _, m := range mounts {
		mountPaths[filepath.Base(m.HostPath)] = true
	}

	for _, name := range []string{"libcuda.so", "libcuda.so.1", "libthunk.so.1.2", "nvvm"} {
		if !mountPaths[name] {
			t.Errorf("expected mount for %q not found", name)
		}
	}

	for _, name := range []string{"readme.txt", "libcuda.a", "python3"} {
		if mountPaths[name] {
			t.Errorf("unexpected mount for %q found", name)
		}
	}
}

func TestGenerate_Hooks(t *testing.T) {
	p, _ := setupTestProfile(t)
	devs := []device.Device{{Path: "/dev/test0", Index: 0, UUID: "GPU-aaaa"}}
	gen := NewGenerator(p, devs, testLogger())

	spec, err := gen.Generate()
	if err != nil {
		t.Fatalf("Generate() error: %v", err)
	}

	hooks := spec.Devices[0].ContainerEdits.Hooks
	if hooks == nil {
		t.Fatal("Hooks is nil, expected ldconfig hook")
	}

	if len(hooks.Prestart) != 1 {
		t.Fatalf("len(Prestart) = %d, want 1", len(hooks.Prestart))
	}

	if hooks.Prestart[0].Path != "/sbin/ldconfig" {
		t.Errorf("Prestart[0].Path = %q, want %q", hooks.Prestart[0].Path, "/sbin/ldconfig")
	}
}

func TestGenerate_NoLdconfigNoHooks(t *testing.T) {
	p, _ := setupTestProfile(t)
	p.Inject.Linker.RunLdconfig = false
	devs := []device.Device{{Path: "/dev/test0", Index: 0, UUID: "GPU-aaaa"}}
	gen := NewGenerator(p, devs, testLogger())

	spec, err := gen.Generate()
	if err != nil {
		t.Fatalf("Generate() error: %v", err)
	}

	if spec.Devices[0].ContainerEdits.Hooks != nil {
		t.Error("Hooks should be nil when RunLdconfig is false")
	}
}

func TestGenerate_EmptyDevices(t *testing.T) {
	p, _ := setupTestProfile(t)
	gen := NewGenerator(p, nil, testLogger())

	_, err := gen.Generate()
	if err == nil {
		t.Error("expected error for empty devices, got nil")
	}
}

func TestGenerate_InvalidProfile(t *testing.T) {
	p, _ := setupTestProfile(t)
	p.Metadata.Name = ""
	devs := []device.Device{{Path: "/dev/test0", Index: 0}}
	gen := NewGenerator(p, devs, testLogger())

	_, err := gen.Generate()
	if err == nil {
		t.Error("expected error for invalid profile, got nil")
	}
}

func TestSpecFilename(t *testing.T) {
	tests := []struct {
		kind string
		want string
	}{
		{"iluvatar.com/gpu", "iluvatar.json"},
		{"huawei.com/Ascend910", "huawei.json"},
		{"nvidia.com/gpu", "nvidia.json"},
		{"test.com/device", "test.json"},
	}
	for _, tt := range tests {
		got := specFilename(tt.kind)
		if got != tt.want {
			t.Errorf("specFilename(%q) = %q, want %q", tt.kind, got, tt.want)
		}
	}
}

func TestIsSharedLibrary(t *testing.T) {
	tests := []struct {
		name string
		want bool
	}{
		{"libcuda.so", true},
		{"libcuda.so.1", true},
		{"libcuda.so.1.2.3", true},
		{"libthunk.so", true},
		{"readme.txt", false},
		{"libcuda.a", false},
		{"libcuda.so.bak", false},
		{"libcuda.something", false},
	}
	for _, tt := range tests {
		got := strutil.IsSharedLibrary(tt.name)
		if got != tt.want {
			t.Errorf("IsSharedLibrary(%q) = %v, want %v", tt.name, got, tt.want)
		}
	}
}

func TestArtifactContainerPath(t *testing.T) {
	tests := []struct {
		containerRoot string
		containerPath string
		hostPath      string
		want          string
	}{
		{"/usr/local/corex", "/usr/local/corex", "/usr/local/corex/lib64", "/usr/local/corex/lib64"},
		{"/usr/local/corex", "/usr/local/corex", "/opt/other/lib", "/usr/local/corex/lib"},
	}
	for _, tt := range tests {
		got := artifactContainerPath(tt.containerRoot, tt.containerPath, tt.hostPath)
		if got != tt.want {
			t.Errorf("artifactContainerPath(%q, %q, %q) = %q, want %q",
				tt.containerRoot, tt.containerPath, tt.hostPath, got, tt.want)
		}
	}
}

func TestWriteSpec(t *testing.T) {
	spec := &Spec{
		CDIVersion: SpecVersion,
		Kind:       "test.com/gpu",
		Devices: []Device{
			{
				Name: "GPU-aaaa",
				ContainerEdits: ContainerEdits{
					Env: []string{"TEST_VISIBLE_DEVICES=all"},
					DeviceNodes: []DeviceNode{
						{HostPath: "/dev/test0", Path: "/dev/test0", Permissions: "rwm"},
					},
				},
			},
		},
	}

	tmpDir := t.TempDir()
	path, err := WriteSpec(spec, tmpDir)
	if err != nil {
		t.Fatalf("WriteSpec() error: %v", err)
	}

	if path != filepath.Join(tmpDir, "test.json") {
		t.Errorf("path = %q, want %q", path, filepath.Join(tmpDir, "test.json"))
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile error: %v", err)
	}

	if len(data) == 0 {
		t.Error("written file is empty")
	}
}
