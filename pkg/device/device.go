// Package device handles discovery and enumeration of Iluvatar GPU devices.
package device

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
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
	// UUID is the GPU UUID reported by ixsmi, e.g. GPU-c22ac027-569b-548c-93dd-5ec7ef8eca9a.
	UUID string
}

// IxsmiQueryFunc is the function used to query GPU UUID-to-index mapping.
// It returns a map from UUID to device index. Replaceable for testing.
var IxsmiQueryFunc = ixsmiQuery

// Discover returns the Device nodes that correspond to the requested GPUs.
//
// visibleDevices is the raw value of ILUVATAR_COREX_VISIBLE_DEVICES. Supported formats:
//
//	"all"                      — expose every Iluvatar GPU found on the host.
//	"none"                     — expose no GPUs (empty result).
//	""                         — same as "all" (when DisableRequire is set by the caller).
//	"0"  or "0,1,2"            — expose GPUs by numeric index.
//	"GPU-xxxx,...,GPU-yyyy"    — expose GPUs by UUID (as set by the Device Plugin).
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
		return filterDevices(all, visibleDevices, log)
	}
}

// filterDevices routes to UUID-based or index-based filtering depending on the
// format of the visibleDevices string.
func filterDevices(all []Device, visibleDevices string, log *logrus.Logger) ([]Device, error) {
	parts := strings.Split(visibleDevices, ",")
	if len(parts) > 0 && isUUID(strings.TrimSpace(parts[0])) {
		return filterByUUID(all, visibleDevices, log)
	}
	return filterByIndex(all, visibleDevices)
}

// isUUID returns true if the string looks like a GPU UUID (starts with "GPU-").
func isUUID(s string) bool {
	return strings.HasPrefix(s, "GPU-")
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

// filterByUUID uses ixsmi to resolve UUIDs to device indices, then returns the
// matching device nodes.
func filterByUUID(all []Device, uuids string, log *logrus.Logger) ([]Device, error) {
	uuidMap, err := IxsmiQueryFunc()
	if err != nil {
		return nil, fmt.Errorf("querying GPU UUIDs via ixsmi: %w", err)
	}
	log.WithField("uuidMap", uuidMap).Debug("resolved UUID-to-index mapping")

	requested := make(map[int]bool)
	for _, part := range strings.Split(uuids, ",") {
		uuid := strings.TrimSpace(part)
		if uuid == "" {
			continue
		}
		idx, ok := uuidMap[uuid]
		if !ok {
			return nil, fmt.Errorf("GPU UUID %q not found on host (known UUIDs: %v)", uuid, mapKeys(uuidMap))
		}
		requested[idx] = true
	}

	var result []Device
	for _, d := range all {
		if requested[d.Index] {
			d.UUID = findUUIDByIndex(uuidMap, d.Index)
			result = append(result, d)
		}
	}
	return result, nil
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
			return nil, fmt.Errorf("invalid GPU index %q in device list", part)
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

// ixsmiQuery calls `ixsmi --query-gpu=index,uuid --format=csv` and parses the
// output into a map of UUID → device index.
func ixsmiQuery() (map[string]int, error) {
	cmd := exec.Command("ixsmi", "--query-gpu=index,uuid", "--format=csv")
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("running ixsmi: %w", err)
	}

	result := make(map[string]int)
	scanner := bufio.NewScanner(strings.NewReader(string(out)))

	// Skip header line: "index, uuid"
	if scanner.Scan() {
		// header consumed
	}

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		// Each line: "0, GPU-c22ac027-569b-548c-93dd-5ec7ef8eca9a"
		parts := strings.SplitN(line, ",", 2)
		if len(parts) != 2 {
			continue
		}
		idxStr := strings.TrimSpace(parts[0])
		uuid := strings.TrimSpace(parts[1])

		idx, err := strconv.Atoi(idxStr)
		if err != nil {
			continue
		}
		result[uuid] = idx
	}

	if len(result) == 0 {
		return nil, fmt.Errorf("ixsmi returned no GPU entries")
	}
	return result, nil
}

// findUUIDByIndex returns the UUID for a given device index from the UUID map.
func findUUIDByIndex(uuidMap map[string]int, index int) string {
	for uuid, idx := range uuidMap {
		if idx == index {
			return uuid
		}
	}
	return ""
}

// mapKeys returns the keys of a map as a sorted slice.
func mapKeys(m map[string]int) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
