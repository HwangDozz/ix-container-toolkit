package cdi

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

const DefaultCDIDir = "/etc/cdi"

// WriteSpec writes a CDI spec to the standard CDI directory.
// The filename is derived from the spec kind (e.g., "iluvatar.com/gpu" -> "iluvatar.json").
func WriteSpec(spec *Spec, cdiDir string) (string, error) {
	if cdiDir == "" {
		cdiDir = DefaultCDIDir
	}

	if err := os.MkdirAll(cdiDir, 0755); err != nil {
		return "", fmt.Errorf("creating CDI directory %s: %w", cdiDir, err)
	}

	data, err := yaml.Marshal(spec)
	if err != nil {
		return "", fmt.Errorf("marshalling CDI spec: %w", err)
	}

	filename := specFilename(spec.Kind)
	path := filepath.Join(cdiDir, filename)

	if err := writeFileAtomic(path, data, 0644); err != nil {
		return "", fmt.Errorf("writing CDI spec to %s: %w", path, err)
	}

	return path, nil
}

// ReadSpec reads an existing CDI spec file for the given kind.
// Returns nil if the file does not exist.
func ReadSpec(cdiDir, kind string) (*Spec, error) {
	if cdiDir == "" {
		cdiDir = DefaultCDIDir
	}
	path := filepath.Join(cdiDir, specFilename(kind))

	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading CDI spec %s: %w", path, err)
	}

	var spec Spec
	if err := yaml.Unmarshal(data, &spec); err != nil {
		return nil, fmt.Errorf("parsing CDI spec %s: %w", path, err)
	}
	return &spec, nil
}

// MergeDevices adds or replaces device entries in the spec.
// Devices are matched by name; existing entries with the same name are replaced.
// If existing is nil, a new spec is created using the first device's context.
func MergeDevices(existing *Spec, newDevices []Device, kind string) *Spec {
	if len(newDevices) == 0 {
		return existing
	}

	if existing == nil {
		return &Spec{
			CDIVersion: SpecVersion,
			Kind:       kind,
			Devices:    newDevices,
		}
	}

	// Index existing devices by name for O(1) lookup.
	byName := make(map[string]int, len(existing.Devices))
	for i, d := range existing.Devices {
		byName[d.Name] = i
	}

	for _, nd := range newDevices {
		if idx, ok := byName[nd.Name]; ok {
			existing.Devices[idx] = nd
		} else {
			existing.Devices = append(existing.Devices, nd)
			byName[nd.Name] = len(existing.Devices) - 1
		}
	}

	return existing
}

// RemoveDevices removes device entries matching the given names.
// Returns nil if the resulting spec has no devices left.
func RemoveDevices(spec *Spec, deviceNames []string) *Spec {
	if spec == nil || len(deviceNames) == 0 {
		return spec
	}

	remove := make(map[string]bool, len(deviceNames))
	for _, n := range deviceNames {
		remove[n] = true
	}

	var kept []Device
	for _, d := range spec.Devices {
		if !remove[d.Name] {
			kept = append(kept, d)
		}
	}

	if len(kept) == 0 {
		return nil
	}
	spec.Devices = kept
	return spec
}

// DeleteSpecFile removes the CDI spec file for the given kind.
// Returns nil if the file does not exist.
func DeleteSpecFile(cdiDir, kind string) error {
	if cdiDir == "" {
		cdiDir = DefaultCDIDir
	}
	path := filepath.Join(cdiDir, specFilename(kind))

	if err := os.Remove(path); os.IsNotExist(err) {
		return nil
	} else if err != nil {
		return fmt.Errorf("deleting CDI spec %s: %w", path, err)
	}
	return nil
}

func writeFileAtomic(path string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)
	base := filepath.Base(path)

	tmp, err := os.CreateTemp(dir, "."+base+".tmp-")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	removeTmp := true
	defer func() {
		if removeTmp {
			_ = os.Remove(tmpPath)
		}
	}()

	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Chmod(perm); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return err
	}
	removeTmp = false
	return nil
}

// specFilename converts a CDI kind to a filename.
// "iluvatar.com/gpu" -> "iluvatar.json"
// "huawei.com/Ascend910" -> "huawei.json"
func specFilename(kind string) string {
	// Take the domain part before the first dot or slash.
	parts := strings.SplitN(kind, ".", 2)
	vendor := parts[0]
	// Sanitize: lowercase and replace non-alphanumeric.
	vendor = strings.ToLower(vendor)
	return vendor + ".json"
}
