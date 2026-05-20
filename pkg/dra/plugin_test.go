package dra

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/sirupsen/logrus"
	resourceapi "k8s.io/api/resource/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/dynamic-resource-allocation/kubeletplugin"

	"github.com/accelerator-toolkit/accelerator-toolkit/pkg/cdi"
	"github.com/accelerator-toolkit/accelerator-toolkit/pkg/device"
	"github.com/accelerator-toolkit/accelerator-toolkit/pkg/profile"
)

func testLogger() *logrus.Logger {
	log := logrus.New()
	log.SetLevel(logrus.WarnLevel)
	return log
}

// setupTestPlugin creates a Plugin with a test profile, real temp directories,
// and two test devices. Returns the plugin, temp root, and the device list.
func setupTestPlugin(t *testing.T) (*Plugin, string, []device.Device) {
	t.Helper()
	tmpDir := t.TempDir()

	// Create real directories so the CDI generator can resolve paths.
	libDir := filepath.Join(tmpDir, "lib64")
	if err := os.MkdirAll(libDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(libDir, "libtest.so"), []byte("fake"), 0644); err != nil {
		t.Fatal(err)
	}

	cdiDir := filepath.Join(tmpDir, "cdi")
	if err := os.MkdirAll(cdiDir, 0755); err != nil {
		t.Fatal(err)
	}

	p := &profile.Profile{
		Metadata: profile.Metadata{
			Name: "test", Vendor: "Test", ModelFamily: "GPU", Version: "v1",
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
				{Name: "driver-libraries", Kind: "shared-libraries", HostPaths: []string{libDir}, ContainerPath: tmpDir, Mode: "so-only"},
			},
			Linker: profile.Linker{ConfigPath: "/etc/ld.so.conf.d/test.conf"},
		},
	}

	devs := []device.Device{
		{Path: "/dev/test0", Index: 0, UUID: "GPU-aaaa"},
		{Path: "/dev/test1", Index: 1, UUID: "GPU-bbbb"},
	}

	plugin := NewPlugin(PluginConfig{
		Profile:  p,
		Devices:  devs,
		CDIDir:   cdiDir,
		PoolName: "test-node",
	}, testLogger())

	return plugin, cdiDir, devs
}

// makeClaim creates a ResourceClaim with the given UID, name, and allocated device names.
func makeClaim(uid, name string, deviceNames ...string) *resourceapi.ResourceClaim {
	var results []resourceapi.DeviceRequestAllocationResult
	for _, dn := range deviceNames {
		results = append(results, resourceapi.DeviceRequestAllocationResult{
			Request: "gpu-0",
			Driver:  DriverName,
			Device:  dn,
			Pool:    "test-node",
		})
	}
	return &resourceapi.ResourceClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
			UID:  types.UID(uid),
		},
		Status: resourceapi.ResourceClaimStatus{
			Allocation: &resourceapi.AllocationResult{
				Devices: resourceapi.DeviceAllocationResult{
					Results: results,
				},
			},
		},
	}
}

func TestPrepareClaim_MergesWithExisting(t *testing.T) {
	plugin, cdiDir, _ := setupTestPlugin(t)

	// Prepare claim A (GPU-aaaa).
	claimA := makeClaim("uid-a", "claim-a", "GPU-aaaa")
	cdiIDsA, _, err := plugin.prepareClaim(claimA)
	if err != nil {
		t.Fatalf("prepareClaim(A) error: %v", err)
	}
	if len(cdiIDsA) != 1 {
		t.Fatalf("len(cdiIDsA) = %d, want 1", len(cdiIDsA))
	}

	// Prepare claim B (GPU-bbbb).
	claimB := makeClaim("uid-b", "claim-b", "GPU-bbbb")
	cdiIDsB, _, err := plugin.prepareClaim(claimB)
	if err != nil {
		t.Fatalf("prepareClaim(B) error: %v", err)
	}
	if len(cdiIDsB) != 1 {
		t.Fatalf("len(cdiIDsB) = %d, want 1", len(cdiIDsB))
	}

	// Read the CDI spec and verify both devices are present.
	// Device names include claim UID suffix for cache uniqueness.
	kind := plugin.profile.Kubernetes.ResourceNames[0]
	spec, err := cdi.ReadSpec(cdiDir, kind)
	if err != nil {
		t.Fatalf("ReadSpec error: %v", err)
	}
	if spec == nil {
		t.Fatal("CDI spec is nil after two prepares")
	}
	if len(spec.Devices) != 2 {
		t.Fatalf("len(Devices) = %d, want 2 (both claims should be present)", len(spec.Devices))
	}

	names := map[string]bool{}
	for _, d := range spec.Devices {
		names[d.Name] = true
	}
	if !names["GPU-aaaa-uid-a"] {
		t.Error("GPU-aaaa-uid-a missing from CDI spec after merge")
	}
	if !names["GPU-bbbb-uid-b"] {
		t.Error("GPU-bbbb-uid-b missing from CDI spec after merge")
	}
}

func TestUnprepareClaim_CleansCDIEntries(t *testing.T) {
	plugin, cdiDir, _ := setupTestPlugin(t)

	// Prepare both claims via the public API so preparedClaims is populated.
	claimA := makeClaim("uid-a", "claim-a", "GPU-aaaa")
	claimB := makeClaim("uid-b", "claim-b", "GPU-bbbb")

	results, err := plugin.PrepareResourceClaims(nil, []*resourceapi.ResourceClaim{claimA, claimB})
	if err != nil {
		t.Fatalf("PrepareResourceClaims error: %v", err)
	}
	if results[types.UID("uid-a")].Err != nil {
		t.Fatalf("prepare A error: %v", results[types.UID("uid-a")].Err)
	}
	if results[types.UID("uid-b")].Err != nil {
		t.Fatalf("prepare B error: %v", results[types.UID("uid-b")].Err)
	}

	// Unprepare claim A.
	err = plugin.unprepareClaim(types.UID("uid-a"))
	if err != nil {
		t.Fatalf("unprepareClaim(A) error: %v", err)
	}

	// Verify only GPU-bbbb remains in CDI spec.
	kind := plugin.profile.Kubernetes.ResourceNames[0]
	spec, err := cdi.ReadSpec(cdiDir, kind)
	if err != nil {
		t.Fatalf("ReadSpec error: %v", err)
	}
	if spec == nil {
		t.Fatal("CDI spec is nil, expected GPU-bbbb to remain")
	}
	if len(spec.Devices) != 1 {
		t.Fatalf("len(Devices) = %d, want 1", len(spec.Devices))
	}
	if spec.Devices[0].Name != "GPU-bbbb-uid-b" {
		t.Errorf("Devices[0].Name = %q, want %q", spec.Devices[0].Name, "GPU-bbbb-uid-b")
	}
}

func TestUnprepareClaim_DeletesEmptySpec(t *testing.T) {
	plugin, cdiDir, _ := setupTestPlugin(t)

	claimA := makeClaim("uid-a", "claim-a", "GPU-aaaa")
	results, err := plugin.PrepareResourceClaims(nil, []*resourceapi.ResourceClaim{claimA})
	if err != nil {
		t.Fatalf("PrepareResourceClaims error: %v", err)
	}
	if results[types.UID("uid-a")].Err != nil {
		t.Fatalf("prepare A error: %v", results[types.UID("uid-a")].Err)
	}

	// Verify spec exists.
	specPath := filepath.Join(cdiDir, "test.json")
	if _, err := os.Stat(specPath); err != nil {
		t.Fatalf("spec file should exist: %v", err)
	}

	// Unprepare the only claim.
	if err := plugin.unprepareClaim(types.UID("uid-a")); err != nil {
		t.Fatalf("unprepareClaim error: %v", err)
	}

	// Verify spec file is deleted.
	if _, err := os.Stat(specPath); !os.IsNotExist(err) {
		t.Error("spec file should have been deleted after last device removed")
	}
}

func TestUnprepareClaim_CleansCDIEntriesAfterRestart(t *testing.T) {
	plugin, cdiDir, _ := setupTestPlugin(t)

	claimA := makeClaim("uid-a", "claim-a", "GPU-aaaa")
	results, err := plugin.PrepareResourceClaims(nil, []*resourceapi.ResourceClaim{claimA})
	if err != nil {
		t.Fatalf("PrepareResourceClaims error: %v", err)
	}
	if results[types.UID("uid-a")].Err != nil {
		t.Fatalf("prepare A error: %v", results[types.UID("uid-a")].Err)
	}

	restarted := NewPlugin(PluginConfig{
		Profile:  plugin.profile,
		Devices:  plugin.devices,
		CDIDir:   cdiDir,
		PoolName: plugin.poolName,
	}, testLogger())

	if err := restarted.unprepareClaim(types.UID("uid-a")); err != nil {
		t.Fatalf("unprepare after restart error: %v", err)
	}

	kind := plugin.profile.Kubernetes.ResourceNames[0]
	spec, err := cdi.ReadSpec(cdiDir, kind)
	if err != nil {
		t.Fatalf("ReadSpec error: %v", err)
	}
	if spec != nil {
		t.Fatalf("CDI spec should be deleted after restart cleanup, got %#v", spec)
	}
}

func TestUnprepareClaim_IdempotentForUnknownUID(t *testing.T) {
	plugin, _, _ := setupTestPlugin(t)

	// Unpreparing a UID that was never prepared should not error.
	err := plugin.unprepareClaim(types.UID("unknown-uid"))
	if err != nil {
		t.Fatalf("unprepareClaim(unknown) error: %v", err)
	}
}

func TestPrepareClaim_BothDevicesInCDIIDs(t *testing.T) {
	plugin, _, _ := setupTestPlugin(t)

	// Prepare a claim with two devices.
	claim := makeClaim("uid-multi", "claim-multi", "GPU-aaaa", "GPU-bbbb")
	cdiIDs, _, err := plugin.prepareClaim(claim)
	if err != nil {
		t.Fatalf("prepareClaim error: %v", err)
	}
	if len(cdiIDs) != 2 {
		t.Fatalf("len(cdiIDs) = %d, want 2", len(cdiIDs))
	}

	// Verify the CDI IDs contain both devices (with claim UID suffix).
	idSet := map[string]bool{}
	for _, id := range cdiIDs {
		idSet[id] = true
	}
	if !idSet["test.com/gpu=GPU-aaaa-uid-multi"] {
		t.Error("missing CDI ID for GPU-aaaa-uid-multi")
	}
	if !idSet["test.com/gpu=GPU-bbbb-uid-multi"] {
		t.Error("missing CDI ID for GPU-bbbb-uid-multi")
	}
}

func TestExtractCDIDeviceNames(t *testing.T) {
	tests := []struct {
		cdiIDs []string
		want   []string
	}{
		{[]string{"test.com/gpu=GPU-aaaa"}, []string{"GPU-aaaa"}},
		{[]string{"test.com/gpu=GPU-aaaa", "test.com/gpu=GPU-bbbb"}, []string{"GPU-aaaa", "GPU-bbbb"}},
		{[]string{"malformed"}, nil},
		{nil, nil},
	}
	for _, tt := range tests {
		got := extractCDIDeviceNames(tt.cdiIDs)
		if len(got) != len(tt.want) {
			t.Errorf("extractCDIDeviceNames(%v) = %v, want %v", tt.cdiIDs, got, tt.want)
			continue
		}
		for i := range got {
			if got[i] != tt.want[i] {
				t.Errorf("extractCDIDeviceNames(%v)[%d] = %q, want %q", tt.cdiIDs, i, got[i], tt.want[i])
			}
		}
	}
}

// unprepareClaim is a test helper that calls UnprepareResourceClaims for a single UID.
func (p *Plugin) unprepareClaim(uid types.UID) error {
	results, err := p.UnprepareResourceClaims(nil, []kubeletplugin.NamespacedObject{
		{
			NamespacedName: types.NamespacedName{Name: "test-claim"},
			UID:            uid,
		},
	})
	if err != nil {
		return err
	}
	return results[uid]
}
