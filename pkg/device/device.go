// Package device handles discovery and enumeration of Iluvatar GPU devices.
package device

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/sirupsen/logrus"

	"github.com/ix-toolkit/ix-toolkit/pkg/config"
)

// Device represents a single Iluvatar GPU device node.
type Device struct {
	// Path is the absolute host path of the device node, e.g. /dev/iluvatar0.
	Path string
	// Index is the numeric index of the GPU (0, 1, 2, …).
	Index int
}

// Discover returns the Device nodes that correspond to the requested GPUs.
//
// visibleDevices is the raw value of ILUVATAR_VISIBLE_DEVICES. Supported values:
//
//	"all"            — expose every Iluvatar GPU found on the host.
//	"none"           — expose no GPUs (empty result).
//	"0"              — expose GPU index 0.
//	"0,1,2"          — expose GPUs at indices 0, 1, and 2.
//	""               — same as "all" (when DisableRequire is set by the caller).
func Discover(visibleDevices string, log *logrus.Logger) ([]Device, error) {
	all, err := enumerateAll()
	if err != nil {
		return nil, fmt.Errorf("enumerating Iluvatar devices: %w", err)
	}
	log.WithField("count", len(all)).Debug("found Iluvatar devices on host")

	switch strings.ToLower(strings.TrimSpace(visibleDevices)) {
	case "all", "":
		return all, nil
	case "none":
		return nil, nil
	default:
		return filterByIndex(all, visibleDevices)
	}
}

// enumerateAll scans /dev for iluvatar* device nodes.
func enumerateAll() ([]Device, error) {
	pattern := config.DevicePrefix + "*"
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return nil, err
	}

	var devs []Device
	for _, path := range matches {
		// Only accept character or block device files.
		info, err := os.Stat(path)
		if err != nil {
			continue
		}
		if info.Mode()&os.ModeDevice == 0 {
			continue
		}

		// Parse the trailing number from the device name, e.g. "iluvatar3" → 3.
		name := filepath.Base(path)
		suffix := strings.TrimPrefix(name, filepath.Base(config.DevicePrefix))
		idx, err := strconv.Atoi(suffix)
		if err != nil {
			// Non-numeric suffix (e.g. /dev/iluvatarctl) — skip.
			continue
		}

		devs = append(devs, Device{Path: path, Index: idx})
	}

	sort.Slice(devs, func(i, j int) bool { return devs[i].Index < devs[j].Index })
	return devs, nil
}

// filterByIndex returns the subset of all whose Index appears in the
// comma-separated indices string.
func filterByIndex(all []Device, indices string) ([]Device, error) {
	requested := make(map[int]bool)
	for _, part := range strings.Split(indices, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		idx, err := strconv.Atoi(part)
		if err != nil {
			return nil, fmt.Errorf("invalid GPU index %q in ILUVATAR_VISIBLE_DEVICES", part)
		}
		requested[idx] = true
	}

	var result []Device
	for _, d := range all {
		if requested[d.Index] {
			result = append(result, d)
		}
	}
	return result, nil
}
