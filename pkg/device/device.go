// Package device handles discovery and enumeration of accelerator device nodes.
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

	"github.com/accelerator-toolkit/accelerator-toolkit/pkg/profile"
)

// Device represents a single accelerator device node.
type Device struct {
	// Path is the absolute host path of the device node, e.g. /dev/davinci0.
	Path string
	// Index is the numeric index of the accelerator device (0, 1, 2, …).
	Index int
	// UUID is the opaque device identifier reported by the mapping command.
	UUID string
}

// ResolverConfig describes how device nodes are enumerated and how UUIDs are
// resolved into device indices.
type ResolverConfig struct {
	DeviceGlobs           []string
	MappingPathCandidates []string
	MappingArgs           []string
	MappingEnv            map[string]string
	MappingParser         string
}

// MappingQueryFunc is the function used to query device identifier-to-index mapping.
// It returns a map from UUID to device index. Replaceable for testing.
var MappingQueryFunc = queryMapping

// DefaultResolverConfig returns an empty resolver config.
func DefaultResolverConfig() ResolverConfig {
	return ResolverConfig{}
}

// ResolverConfigFromProfile derives a device resolver config from a generic profile.
func ResolverConfigFromProfile(p *profile.Profile) ResolverConfig {
	if p == nil {
		return ResolverConfig{}
	}

	return ResolverConfig{
		DeviceGlobs:           append([]string(nil), p.Device.DeviceGlobs...),
		MappingPathCandidates: append([]string(nil), p.Device.Mapping.Command.PathCandidates...),
		MappingArgs:           append([]string(nil), p.Device.Mapping.Command.Args...),
		MappingEnv:            cloneStringMap(p.Device.Mapping.Command.Env),
		MappingParser:         p.Device.Mapping.Parser,
	}
}

// DiscoverWithProfile resolves devices using the profile's device globs and
// mapping command settings.
func DiscoverWithProfile(visibleDevices string, p *profile.Profile, log *logrus.Logger) ([]Device, error) {
	return DiscoverWithConfig(visibleDevices, ResolverConfigFromProfile(p), log)
}

// DiscoverWithConfig resolves devices using the provided resolver config.
func DiscoverWithConfig(visibleDevices string, resolverCfg ResolverConfig, log *logrus.Logger) ([]Device, error) {
	all, err := enumerateAll(resolverCfg.DeviceGlobs)
	if err != nil {
		return nil, fmt.Errorf("enumerating accelerator devices: %w", err)
	}
	log.WithField("count", len(all)).Debug("found accelerator devices on host")

	switch strings.ToLower(strings.TrimSpace(visibleDevices)) {
	case "all", "":
		return all, nil
	case "none":
		return nil, nil
	default:
		return filterDevices(all, visibleDevices, resolverCfg, log)
	}
}

// filterDevices routes to identifier-based or index-based filtering depending
// on the format of the visibleDevices string.
func filterDevices(all []Device, visibleDevices string, resolverCfg ResolverConfig, log *logrus.Logger) ([]Device, error) {
	parts := strings.Split(visibleDevices, ",")
	if len(parts) > 0 && usesMappedIdentifiers(strings.TrimSpace(parts[0])) {
		return filterByUUID(all, visibleDevices, resolverCfg, log)
	}
	return filterByIndex(all, visibleDevices)
}

// usesMappedIdentifiers returns true when the selector is not one of the
// built-in keywords and does not look like a numeric device index.
func usesMappedIdentifiers(s string) bool {
	s = strings.TrimSpace(s)
	if s == "" {
		return false
	}
	switch strings.ToLower(s) {
	case "", "all", "none":
		return false
	}
	_, err := strconv.Atoi(s)
	return err != nil
}

// enumerateAll scans device globs for matching device nodes.
func enumerateAll(patterns []string) ([]Device, error) {
	if len(patterns) == 0 {
		return nil, nil
	}

	seen := map[string]bool{}
	var devs []Device
	for _, pattern := range patterns {
		matches, err := filepath.Glob(pattern)
		if err != nil {
			return nil, err
		}
		for _, path := range matches {
			if seen[path] {
				continue
			}
			seen[path] = true
			// Only accept character or block device files.
			info, err := os.Stat(path)
			if err != nil {
				continue
			}
			if info.Mode()&os.ModeDevice == 0 {
				continue
			}

			idx, ok := trailingIndex(filepath.Base(path))
			if !ok {
				continue
			}

			devs = append(devs, Device{Path: path, Index: idx})
		}
	}

	sort.Slice(devs, func(i, j int) bool { return devs[i].Index < devs[j].Index })
	return devs, nil
}

// filterByUUID uses the configured mapping command to resolve opaque device
// identifiers to device indices, then returns the matching device nodes.
//
// If the mapping command is unavailable or returns no data, it falls back to
// positional matching (first identifier → index 0, etc.) so the hook never fails
// solely because the vendor mapping CLI is missing or uses a different syntax.
func filterByUUID(all []Device, uuids string, resolverCfg ResolverConfig, log *logrus.Logger) ([]Device, error) {
	uuidMap, err := MappingQueryFunc(resolverCfg)
	if err != nil {
		log.WithError(err).Warn("mapping command unavailable, falling back to positional identifier-to-index mapping")
		return filterByUUIDPositional(all, uuids, log)
	}
	log.WithField("identifierMap", uuidMap).Debug("resolved device identifier-to-index mapping")

	requested := make(map[int]bool)
	for _, part := range strings.Split(uuids, ",") {
		uuid := strings.TrimSpace(part)
		if uuid == "" {
			continue
		}
		idx, ok := uuidMap[uuid]
		if !ok {
			return nil, fmt.Errorf("device identifier %q not found on host (known identifiers: %v)", uuid, mapKeys(uuidMap))
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

// filterByUUIDPositional matches device identifiers to devices by position when
// the vendor mapping command is unavailable. The first identifier in the list
// is mapped to the device with the lowest index, the second to the next
// device, and so on.
func filterByUUIDPositional(all []Device, uuids string, log *logrus.Logger) ([]Device, error) {
	parts := strings.Split(uuids, ",")
	var requested []string
	for _, p := range parts {
		if u := strings.TrimSpace(p); u != "" {
			requested = append(requested, u)
		}
	}

	if len(requested) > len(all) {
		return nil, fmt.Errorf("requested %d device identifiers but only %d accelerator devices found on host", len(requested), len(all))
	}

	result := make([]Device, len(requested))
	for i, uuid := range requested {
		d := all[i]
		d.UUID = uuid
		result[i] = d
		log.WithFields(logrus.Fields{
			"deviceIdentifier": uuid,
			"device":           d.Path,
		}).Warn("positional device identifier mapping used")
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
			return nil, fmt.Errorf("invalid accelerator index %q in device list", part)
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

func queryMapping(resolverCfg ResolverConfig) (map[string]int, error) {
	cmdPath, err := resolveCommandPath(resolverCfg.MappingPathCandidates)
	if err != nil {
		return nil, err
	}

	cmd := exec.Command(cmdPath, resolverCfg.MappingArgs...)
	cmd.Env = append(os.Environ(), flattenEnv(resolverCfg.MappingEnv)...)
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("running mapping command %s: %w", cmdPath, err)
	}

	return parseMappingOutput(out, resolverCfg.MappingParser)
}

func resolveCommandPath(candidates []string) (string, error) {
	for _, candidate := range candidates {
		if candidate == "" {
			continue
		}
		if strings.Contains(candidate, "/") {
			if _, err := os.Stat(candidate); err == nil {
				return candidate, nil
			}
			continue
		}
		path, err := exec.LookPath(candidate)
		if err == nil {
			return path, nil
		}
	}
	return "", fmt.Errorf("mapping command not found in PATH or configured locations")
}

func parseMappingOutput(out []byte, parser string) (map[string]int, error) {
	if parser == "" {
		parser = "csv-header-index-uuid"
	}
	if parser != "csv-header-index-uuid" {
		return nil, fmt.Errorf("unsupported mapping parser %q", parser)
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
		// Each line: "0, device-identifier"
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
		return nil, fmt.Errorf("mapping command returned no device entries")
	}
	return result, nil
}

func trailingIndex(name string) (int, bool) {
	end := len(name)
	start := end
	for start > 0 {
		c := name[start-1]
		if c < '0' || c > '9' {
			break
		}
		start--
	}
	if start == end {
		return 0, false
	}
	idx, err := strconv.Atoi(name[start:end])
	if err != nil {
		return 0, false
	}
	return idx, true
}

func flattenEnv(env map[string]string) []string {
	if len(env) == 0 {
		return nil
	}
	keys := make([]string, 0, len(env))
	for key := range env {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	result := make([]string, 0, len(keys))
	for _, key := range keys {
		result = append(result, key+"="+env[key])
	}
	return result
}

func cloneStringMap(src map[string]string) map[string]string {
	if len(src) == 0 {
		return nil
	}
	dst := make(map[string]string, len(src))
	for key, value := range src {
		dst[key] = value
	}
	return dst
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
