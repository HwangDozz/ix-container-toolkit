package cdi

import (
	"os"
	"path/filepath"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestReadSpec_Existing(t *testing.T) {
	tmpDir := t.TempDir()
	spec := &Spec{
		CDIVersion: SpecVersion,
		Kind:       "test.com/gpu",
		Devices: []Device{
			{Name: "GPU-aaaa", ContainerEdits: ContainerEdits{Env: []string{"X=1"}}},
		},
	}
	writeTestSpec(t, tmpDir, spec)

	got, err := ReadSpec(tmpDir, "test.com/gpu")
	if err != nil {
		t.Fatalf("ReadSpec() error: %v", err)
	}
	if got == nil {
		t.Fatal("ReadSpec() returned nil for existing file")
	}
	if got.Kind != "test.com/gpu" {
		t.Errorf("Kind = %q, want %q", got.Kind, "test.com/gpu")
	}
	if len(got.Devices) != 1 {
		t.Fatalf("len(Devices) = %d, want 1", len(got.Devices))
	}
	if got.Devices[0].Name != "GPU-aaaa" {
		t.Errorf("Devices[0].Name = %q, want %q", got.Devices[0].Name, "GPU-aaaa")
	}
}

func TestReadSpec_Missing(t *testing.T) {
	tmpDir := t.TempDir()
	got, err := ReadSpec(tmpDir, "test.com/gpu")
	if err != nil {
		t.Fatalf("ReadSpec() error: %v", err)
	}
	if got != nil {
		t.Errorf("ReadSpec() = %v, want nil for missing file", got)
	}
}

func TestMergeDevices_NilExisting(t *testing.T) {
	newDevs := []Device{
		{Name: "GPU-aaaa", ContainerEdits: ContainerEdits{Env: []string{"X=1"}}},
	}
	result := MergeDevices(nil, newDevs, "test.com/gpu")
	if result == nil {
		t.Fatal("MergeDevices() returned nil")
	}
	if result.Kind != "test.com/gpu" {
		t.Errorf("Kind = %q, want %q", result.Kind, "test.com/gpu")
	}
	if len(result.Devices) != 1 {
		t.Fatalf("len(Devices) = %d, want 1", len(result.Devices))
	}
}

func TestMergeDevices_AppendToExisting(t *testing.T) {
	existing := &Spec{
		CDIVersion: SpecVersion,
		Kind:       "test.com/gpu",
		Devices: []Device{
			{Name: "GPU-aaaa", ContainerEdits: ContainerEdits{Env: []string{"X=1"}}},
		},
	}
	newDevs := []Device{
		{Name: "GPU-bbbb", ContainerEdits: ContainerEdits{Env: []string{"X=2"}}},
	}
	result := MergeDevices(existing, newDevs, "test.com/gpu")
	if len(result.Devices) != 2 {
		t.Fatalf("len(Devices) = %d, want 2", len(result.Devices))
	}
	if result.Devices[0].Name != "GPU-aaaa" {
		t.Errorf("Devices[0].Name = %q, want %q", result.Devices[0].Name, "GPU-aaaa")
	}
	if result.Devices[1].Name != "GPU-bbbb" {
		t.Errorf("Devices[1].Name = %q, want %q", result.Devices[1].Name, "GPU-bbbb")
	}
}

func TestMergeDevices_ReplaceExisting(t *testing.T) {
	existing := &Spec{
		CDIVersion: SpecVersion,
		Kind:       "test.com/gpu",
		Devices: []Device{
			{Name: "GPU-aaaa", ContainerEdits: ContainerEdits{Env: []string{"OLD=1"}}},
		},
	}
	newDevs := []Device{
		{Name: "GPU-aaaa", ContainerEdits: ContainerEdits{Env: []string{"NEW=2"}}},
	}
	result := MergeDevices(existing, newDevs, "test.com/gpu")
	if len(result.Devices) != 1 {
		t.Fatalf("len(Devices) = %d, want 1", len(result.Devices))
	}
	if result.Devices[0].ContainerEdits.Env[0] != "NEW=2" {
		t.Errorf("Env[0] = %q, want %q", result.Devices[0].ContainerEdits.Env[0], "NEW=2")
	}
}

func TestMergeDevices_EmptyNew(t *testing.T) {
	existing := &Spec{
		CDIVersion: SpecVersion,
		Kind:       "test.com/gpu",
		Devices: []Device{
			{Name: "GPU-aaaa"},
		},
	}
	result := MergeDevices(existing, nil, "test.com/gpu")
	if len(result.Devices) != 1 {
		t.Errorf("len(Devices) = %d, want 1 (unchanged)", len(result.Devices))
	}
}

func TestRemoveDevices_OneOfMany(t *testing.T) {
	spec := &Spec{
		CDIVersion: SpecVersion,
		Kind:       "test.com/gpu",
		Devices: []Device{
			{Name: "GPU-aaaa"},
			{Name: "GPU-bbbb"},
			{Name: "GPU-cccc"},
		},
	}
	result := RemoveDevices(spec, []string{"GPU-bbbb"})
	if result == nil {
		t.Fatal("RemoveDevices() returned nil, expected 2 remaining")
	}
	if len(result.Devices) != 2 {
		t.Fatalf("len(Devices) = %d, want 2", len(result.Devices))
	}
	if result.Devices[0].Name != "GPU-aaaa" || result.Devices[1].Name != "GPU-cccc" {
		t.Errorf("remaining devices = [%s, %s], want [GPU-aaaa, GPU-cccc]",
			result.Devices[0].Name, result.Devices[1].Name)
	}
}

func TestRemoveDevices_All(t *testing.T) {
	spec := &Spec{
		CDIVersion: SpecVersion,
		Kind:       "test.com/gpu",
		Devices: []Device{
			{Name: "GPU-aaaa"},
		},
	}
	result := RemoveDevices(spec, []string{"GPU-aaaa"})
	if result != nil {
		t.Errorf("RemoveDevices() = %v, want nil (all removed)", result)
	}
}

func TestRemoveDevices_Nonexistent(t *testing.T) {
	spec := &Spec{
		CDIVersion: SpecVersion,
		Kind:       "test.com/gpu",
		Devices: []Device{
			{Name: "GPU-aaaa"},
		},
	}
	result := RemoveDevices(spec, []string{"GPU-xxxx"})
	if result == nil {
		t.Fatal("RemoveDevices() returned nil, expected unchanged")
	}
	if len(result.Devices) != 1 {
		t.Errorf("len(Devices) = %d, want 1 (unchanged)", len(result.Devices))
	}
}

func TestRemoveDevices_NilSpec(t *testing.T) {
	result := RemoveDevices(nil, []string{"GPU-aaaa"})
	if result != nil {
		t.Errorf("RemoveDevices(nil) = %v, want nil", result)
	}
}

func TestDeleteSpecFile_Existing(t *testing.T) {
	tmpDir := t.TempDir()
	spec := &Spec{CDIVersion: SpecVersion, Kind: "test.com/gpu"}
	writeTestSpec(t, tmpDir, spec)

	path := filepath.Join(tmpDir, "test.json")
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("file should exist: %v", err)
	}

	if err := DeleteSpecFile(tmpDir, "test.com/gpu"); err != nil {
		t.Fatalf("DeleteSpecFile() error: %v", err)
	}

	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Error("file should have been deleted")
	}
}

func TestDeleteSpecFile_Missing(t *testing.T) {
	tmpDir := t.TempDir()
	if err := DeleteSpecFile(tmpDir, "test.com/gpu"); err != nil {
		t.Fatalf("DeleteSpecFile() error on missing file: %v", err)
	}
}

// writeTestSpec marshals and writes a spec to the temp dir for test setup.
func writeTestSpec(t *testing.T, dir string, spec *Spec) {
	t.Helper()
	data, err := yaml.Marshal(spec)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, specFilename(spec.Kind))
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatal(err)
	}
}
