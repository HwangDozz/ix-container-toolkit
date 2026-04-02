package device

import (
	"testing"

	"github.com/sirupsen/logrus"
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

// Discover tests — only "none" and "all" branches are deterministic
// without real /dev/iluvatar* nodes.

func TestDiscover_None(t *testing.T) {
	log := testLogger()
	result, err := Discover("none", log)
	if err != nil {
		t.Fatalf("Discover(\"none\") returned error: %v", err)
	}
	if result != nil {
		t.Errorf("Discover(\"none\") should return nil, got %v", result)
	}
}

func TestDiscover_NoneVariants(t *testing.T) {
	log := testLogger()
	for _, v := range []string{"NONE", "None", "  none  "} {
		result, err := Discover(v, log)
		if err != nil {
			t.Fatalf("Discover(%q) returned error: %v", v, err)
		}
		if result != nil {
			t.Errorf("Discover(%q) should return nil", v)
		}
	}
}

func TestDiscover_InvalidIndex(t *testing.T) {
	log := testLogger()
	_, err := Discover("notanumber", log)
	// enumerateAll succeeds (returns empty on hosts without iluvatar devices),
	// then filterByIndex returns an error for "notanumber".
	if err == nil {
		t.Error("expected error for invalid index in Discover, got nil")
	}
}
