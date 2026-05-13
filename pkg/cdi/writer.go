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

	if err := os.WriteFile(path, data, 0644); err != nil {
		return "", fmt.Errorf("writing CDI spec to %s: %w", path, err)
	}

	return path, nil
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
