package device

import (
	"testing"

	"github.com/sirupsen/logrus"

	"github.com/accelerator-toolkit/accelerator-toolkit/pkg/profile"
)

func testLogger() *logrus.Logger {
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	return log
}

// filterByIndex tests — pure logic, no filesystem dependency.

func TestFilterByIndex_SingleDevice(t *testing.T) {
	all := []Device{
		{Path: "/dev/iluvatar0", Index: 0},
		{Path: "/dev/iluvatar1", Index: 1},
		{Path: "/dev/iluvatar2", Index: 2},
	}

	result, err := filterByIndex(all, "1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("expected 1 device, got %d", len(result))
	}
	if result[0].Index != 1 {
		t.Errorf("expected index 1, got %d", result[0].Index)
	}
}

func TestFilterByIndex_MultipleDevices(t *testing.T) {
	all := []Device{
		{Path: "/dev/iluvatar0", Index: 0},
		{Path: "/dev/iluvatar1", Index: 1},
		{Path: "/dev/iluvatar2", Index: 2},
	}

	result, err := filterByIndex(all, "0,2")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 2 {
		t.Fatalf("expected 2 devices, got %d", len(result))
	}
	idxSet := map[int]bool{}
	for _, d := range result {
		idxSet[d.Index] = true
	}
	if !idxSet[0] || !idxSet[2] {
		t.Errorf("expected indices {0,2}, got %v", idxSet)
	}
}

func TestFilterByIndex_IndexNotPresent(t *testing.T) {
	all := []Device{
		{Path: "/dev/iluvatar0", Index: 0},
	}

	result, err := filterByIndex(all, "5")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 0 {
		t.Errorf("expected 0 devices, got %d", len(result))
	}
}

func TestFilterByIndex_InvalidIndex(t *testing.T) {
	all := []Device{{Path: "/dev/iluvatar0", Index: 0}}

	_, err := filterByIndex(all, "abc")
	if err == nil {
		t.Error("expected error for non-numeric index, got nil")
	}
}

func TestFilterByIndex_WithSpaces(t *testing.T) {
	all := []Device{
		{Path: "/dev/iluvatar0", Index: 0},
		{Path: "/dev/iluvatar1", Index: 1},
	}

	result, err := filterByIndex(all, " 0 , 1 ")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 2 {
		t.Errorf("expected 2 devices, got %d", len(result))
	}
}

func TestFilterByIndex_EmptyIndicesString(t *testing.T) {
	all := []Device{{Path: "/dev/iluvatar0", Index: 0}}

	// Empty string after split produces only an empty part which is skipped.
	result, err := filterByIndex(all, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// No valid indices requested → no matches.
	if len(result) != 0 {
		t.Errorf("expected 0 devices, got %d", len(result))
	}
}

func TestFilterByIndex_DuplicateIndices(t *testing.T) {
	all := []Device{
		{Path: "/dev/iluvatar0", Index: 0},
		{Path: "/dev/iluvatar1", Index: 1},
	}

	// map-based dedup: device 0 should appear only once.
	result, err := filterByIndex(all, "0,0,0")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 1 {
		t.Errorf("expected 1 device (dedup via map), got %d", len(result))
	}
}

// DiscoverWithConfig tests — only "none" and index parsing branches are
// deterministic without real device nodes.

func TestDiscoverWithConfig_None(t *testing.T) {
	log := testLogger()
	result, err := DiscoverWithConfig("none", ResolverConfig{}, log)
	if err != nil {
		t.Fatalf("DiscoverWithConfig(\"none\") returned error: %v", err)
	}
	if result != nil {
		t.Errorf("DiscoverWithConfig(\"none\") should return nil, got %v", result)
	}
}

func TestDiscoverWithConfig_NoneVariants(t *testing.T) {
	log := testLogger()
	for _, v := range []string{"NONE", "None", "  none  "} {
		result, err := DiscoverWithConfig(v, ResolverConfig{}, log)
		if err != nil {
			t.Fatalf("DiscoverWithConfig(%q) returned error: %v", v, err)
		}
		if result != nil {
			t.Errorf("DiscoverWithConfig(%q) should return nil", v)
		}
	}
}

func TestDiscoverWithConfig_InvalidIndex(t *testing.T) {
	log := testLogger()
	_, err := DiscoverWithConfig("notanumber", ResolverConfig{}, log)
	if err == nil {
		t.Error("expected error for invalid index in DiscoverWithConfig, got nil")
	}
}

func TestTrailingIndex(t *testing.T) {
	idx, ok := trailingIndex("iluvatar7")
	if !ok || idx != 7 {
		t.Fatalf("trailingIndex = (%d, %v), want (7, true)", idx, ok)
	}

	_, ok = trailingIndex("iluvatarctl")
	if ok {
		t.Fatal("trailingIndex should reject names without numeric suffix")
	}
}

func TestResolverConfigFromProfile(t *testing.T) {
	p := &profile.Profile{
		Device: profile.Device{
			DeviceGlobs: []string{"/dev/vendor*"},
			Mapping: profile.DeviceMapping{
				Command: profile.MappingCommand{
					PathCandidates: []string{"/usr/bin/vendor-smi"},
					Args:           []string{"--query"},
					Env: map[string]string{
						"LD_LIBRARY_PATH": "/opt/vendor/lib",
					},
				},
				Parser: "csv-header-index-uuid",
			},
		},
	}

	cfg := ResolverConfigFromProfile(p)
	if len(cfg.DeviceGlobs) != 1 || cfg.DeviceGlobs[0] != "/dev/vendor*" {
		t.Fatalf("DeviceGlobs = %v", cfg.DeviceGlobs)
	}
	if len(cfg.MappingPathCandidates) != 1 || cfg.MappingPathCandidates[0] != "/usr/bin/vendor-smi" {
		t.Fatalf("MappingPathCandidates = %v", cfg.MappingPathCandidates)
	}
	if len(cfg.MappingArgs) != 1 || cfg.MappingArgs[0] != "--query" {
		t.Fatalf("MappingArgs = %v", cfg.MappingArgs)
	}
	if cfg.MappingEnv["LD_LIBRARY_PATH"] != "/opt/vendor/lib" {
		t.Fatalf("MappingEnv = %v", cfg.MappingEnv)
	}
}

func TestResolverConfigFromProfile_DoesNotLeakIluvatarDefaults(t *testing.T) {
	p := &profile.Profile{
		Device: profile.Device{
			DeviceGlobs: []string{"/dev/vendor*"},
		},
	}

	cfg := ResolverConfigFromProfile(p)
	if len(cfg.DeviceGlobs) != 1 || cfg.DeviceGlobs[0] != "/dev/vendor*" {
		t.Fatalf("DeviceGlobs = %v", cfg.DeviceGlobs)
	}
	if len(cfg.MappingPathCandidates) != 0 {
		t.Fatalf("MappingPathCandidates = %v, want empty", cfg.MappingPathCandidates)
	}
	if len(cfg.MappingArgs) != 0 {
		t.Fatalf("MappingArgs = %v, want empty", cfg.MappingArgs)
	}
	if cfg.MappingParser != "" {
		t.Fatalf("MappingParser = %q, want empty", cfg.MappingParser)
	}
}

func TestParseMappingOutput_UnsupportedParser(t *testing.T) {
	_, err := parseMappingOutput([]byte("index, uuid\n0, GPU-test\n"), "json")
	if err == nil {
		t.Fatal("expected parser error, got nil")
	}
}

func TestUsesMappedIdentifiers(t *testing.T) {
	cases := map[string]bool{
		"":          false,
		"all":       false,
		"none":      false,
		"0":         false,
		" 12 ":      false,
		"GPU-test":  true,
		"Ascend910": true,
	}

	for input, want := range cases {
		if got := usesMappedIdentifiers(input); got != want {
			t.Fatalf("usesMappedIdentifiers(%q) = %v, want %v", input, got, want)
		}
	}
}
