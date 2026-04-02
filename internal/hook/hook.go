// Package hook implements the OCI prestart hook that injects Iluvatar GPU
// devices and driver libraries into a container.
//
// The hook is called by the OCI runtime (e.g. runc) right before the container
// process starts, with stdin containing the OCI container state JSON. It:
//
//  1. Reads the container state to find the container bundle/rootfs.
//  2. Inspects the container spec to determine which GPUs to expose via the
//     ILUVATAR_COREX_VISIBLE_DEVICES environment variable.
//  3. Discovers /dev/iluvatar* device nodes on the host that correspond to the
//     requested GPUs.
//  4. Bind-mounts the device nodes and driver shared libraries into the container.
//  5. Injects an ld.so.conf.d snippet so that the dynamic linker inside the
//     container finds the mounted driver libraries.
package hook

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/opencontainers/runtime-spec/specs-go"
	"github.com/sirupsen/logrus"

	"github.com/ix-toolkit/ix-toolkit/pkg/config"
	"github.com/ix-toolkit/ix-toolkit/pkg/device"
)

// Hook is the main hook executor.
type Hook struct {
	cfg *config.Config
	log *logrus.Logger
}

// New creates a Hook using the provided configuration and logger.
func New(cfg *config.Config, log *logrus.Logger) *Hook {
	return &Hook{cfg: cfg, log: log}
}

// Run executes the OCI prestart hook. It reads the OCI container state from r
// and injects GPU devices and driver libraries into the container rootfs.
func (h *Hook) Run(r io.Reader) error {
	// 1. Parse the OCI container state from stdin.
	state, err := readContainerState(r)
	if err != nil {
		return fmt.Errorf("reading container state: %w", err)
	}
	h.log.WithField("containerID", state.ID).Debug("hook invoked")

	// 2. Load the OCI spec from the bundle.
	spec, err := loadSpec(state.Bundle)
	if err != nil {
		return fmt.Errorf("loading OCI spec: %w", err)
	}

	// 3. Determine which GPUs the container requested.
	visibleDevices := h.visibleDevices(spec)
	if visibleDevices == "" && !h.cfg.Hook.DisableRequire {
		// Container did not request any GPU — nothing to do.
		h.log.Debug("no GPU requested (ILUVATAR_COREX_VISIBLE_DEVICES not set), skipping")
		return nil
	}
	if strings.EqualFold(strings.TrimSpace(visibleDevices), "none") {
		h.log.Debug("ILUVATAR_COREX_VISIBLE_DEVICES=none, skipping GPU injection")
		return nil
	}

	rootfs := spec.Root.Path
	if !filepath.IsAbs(rootfs) {
		rootfs = filepath.Join(state.Bundle, rootfs)
	}
	h.log.WithFields(logrus.Fields{
		"rootfs":         rootfs,
		"visibleDevices": visibleDevices,
	}).Info("injecting Iluvatar GPU into container")

	// 4. Discover matching device nodes on the host.
	devs, err := device.Discover(visibleDevices, h.log)
	if err != nil {
		return fmt.Errorf("discovering devices: %w", err)
	}
	if len(devs) == 0 {
		if h.cfg.Hook.DisableRequire {
			h.log.Warn("no Iluvatar devices found, continuing because disableRequire=true")
			return nil
		}
		return fmt.Errorf("no Iluvatar devices found for ILUVATAR_COREX_VISIBLE_DEVICES=%q", visibleDevices)
	}

	// 5. Resolve symlinks in driver paths (e.g. /usr/local/corex → /usr/local/corex-4.3.0).
	h.resolveDriverPaths()

	// 6. Inject devices and driver libraries.
	if err := h.injectDevices(rootfs, devs); err != nil {
		return fmt.Errorf("injecting devices: %w", err)
	}

	if err := h.injectDriverLibraries(rootfs); err != nil {
		return fmt.Errorf("injecting driver libraries: %w", err)
	}

	if err := h.injectDriverBinaries(rootfs); err != nil {
		return fmt.Errorf("injecting driver binaries: %w", err)
	}

	return nil
}

// visibleDevices returns the value of the ILUVATAR_COREX_VISIBLE_DEVICES (or
// configured equivalent) environment variable in the container spec.
func (h *Hook) visibleDevices(spec *specs.Spec) string {
	if spec.Process == nil {
		return ""
	}
	prefix := h.cfg.Hook.DeviceListEnvvar + "="
	for _, env := range spec.Process.Env {
		if strings.HasPrefix(env, prefix) {
			return strings.TrimPrefix(env, prefix)
		}
	}
	return ""
}

// resolveDriverPaths resolves symlinks in the configured driver paths.
// For example, if ContainerDriverRoot is /usr/local/corex but the real path on
// the host is /usr/local/corex-4.3.0 (symlinked), the library/binary paths
// are updated to point to the real paths so that bind-mounts work correctly.
func (h *Hook) resolveDriverPaths() {
	// Resolve ContainerDriverRoot (used for relative path computation).
	origRoot := h.cfg.Hook.ContainerDriverRoot
	resolvedRoot, err := filepath.EvalSymlinks(origRoot)
	if err == nil && resolvedRoot != origRoot {
		h.log.WithFields(logrus.Fields{
			"original": origRoot,
			"resolved": resolvedRoot,
		}).Info("resolved symlink for driver root")
		// Keep ContainerDriverRoot as the container-side mount target (the
		// non-versioned path), but resolve library/binary host paths.
	}

	// Resolve each library path.
	for i, p := range h.cfg.Hook.DriverLibraryPaths {
		resolved, err := filepath.EvalSymlinks(p)
		if err == nil && resolved != p {
			h.log.WithFields(logrus.Fields{
				"original": p,
				"resolved": resolved,
			}).Debug("resolved symlink for driver library path")
			h.cfg.Hook.DriverLibraryPaths[i] = resolved
		}
	}

	// Resolve each binary path.
	for i, p := range h.cfg.Hook.DriverBinaryPaths {
		resolved, err := filepath.EvalSymlinks(p)
		if err == nil && resolved != p {
			h.log.WithFields(logrus.Fields{
				"original": p,
				"resolved": resolved,
			}).Debug("resolved symlink for driver binary path")
			h.cfg.Hook.DriverBinaryPaths[i] = resolved
		}
	}
}

// injectDevices bind-mounts Iluvatar device nodes into the container.
func (h *Hook) injectDevices(rootfs string, devs []device.Device) error {
	for _, dev := range devs {
		target := filepath.Join(rootfs, dev.Path)
		if err := ensureFile(target); err != nil {
			return fmt.Errorf("creating device placeholder %s: %w", target, err)
		}
		if err := bindMount(dev.Path, target); err != nil {
			return fmt.Errorf("bind-mounting device %s: %w", dev.Path, err)
		}
		h.log.WithField("device", dev.Path).Debug("device injected")
	}
	return nil
}

// injectDriverLibraries bind-mounts host driver libraries into the container
// and registers them with the dynamic linker.
//
// When LibraryFilterMode is "so-only" (default), individual .so files are
// mounted instead of the whole directory, avoiding the ~12GB Python packages
// and other SDK components under lib64/python3/, cmake/, etc.
// When LibraryFilterMode is "directory", the entire directory is mounted
// (legacy behavior).
func (h *Hook) injectDriverLibraries(rootfs string) error {
	containerRoot := h.cfg.Hook.ContainerDriverRoot

	for _, hostPath := range h.cfg.Hook.DriverLibraryPaths {
		if _, err := os.Stat(hostPath); os.IsNotExist(err) {
			h.log.WithField("path", hostPath).Debug("driver library path not found on host, skipping")
			continue
		}

		// Compute the container-side target directory.
		rel, err := filepath.Rel(containerRoot, hostPath)
		if err != nil || strings.HasPrefix(rel, "..") {
			rel = filepath.Base(hostPath)
		}
		targetDir := filepath.Join(rootfs, containerRoot, rel)

		if h.cfg.Hook.LibraryFilterMode == "directory" {
			// Legacy: mount the entire directory.
			if err := os.MkdirAll(targetDir, 0755); err != nil {
				return fmt.Errorf("creating lib dir %s: %w", targetDir, err)
			}
			if err := bindMount(hostPath, targetDir); err != nil {
				return fmt.Errorf("bind-mounting lib dir %s: %w", hostPath, err)
			}
			h.log.WithField("path", hostPath).Debug("driver library dir injected (directory mode)")
		} else {
			// so-only: mount individual .so files, skip excluded subdirectories.
			if err := h.mountSharedLibraries(hostPath, targetDir); err != nil {
				return fmt.Errorf("mounting shared libraries from %s: %w", hostPath, err)
			}
		}
	}

	// Add an ld.so.conf.d entry so that the dynamic linker inside the container
	// discovers the freshly mounted libraries.
	if err := h.injectLdSoConf(rootfs, containerRoot); err != nil {
		h.log.WithError(err).Warn("failed to inject ld.so.conf.d entry (non-fatal)")
	}

	return nil
}

// mountSharedLibraries walks hostDir (non-recursively) and bind-mounts only
// shared library files (.so, .so.1, .so.1.2.3, etc.) into targetDir, skipping
// subdirectories listed in LibraryExcludeDirs.
func (h *Hook) mountSharedLibraries(hostDir, targetDir string) error {
	if err := os.MkdirAll(targetDir, 0755); err != nil {
		return fmt.Errorf("creating target dir %s: %w", targetDir, err)
	}

	excludeSet := make(map[string]bool)
	for _, d := range h.cfg.Hook.LibraryExcludeDirs {
		excludeSet[d] = true
	}

	entries, err := os.ReadDir(hostDir)
	if err != nil {
		return fmt.Errorf("reading directory %s: %w", hostDir, err)
	}

	mounted := 0
	for _, entry := range entries {
		name := entry.Name()

		if entry.IsDir() {
			if excludeSet[name] {
				h.log.WithField("dir", name).Debug("skipping excluded subdirectory")
				continue
			}
			// For non-excluded subdirectories (e.g. nvvm/), recursively mount.
			subHost := filepath.Join(hostDir, name)
			subTarget := filepath.Join(targetDir, name)
			if err := os.MkdirAll(subTarget, 0755); err != nil {
				return fmt.Errorf("creating subdir %s: %w", subTarget, err)
			}
			if err := bindMount(subHost, subTarget); err != nil {
				return fmt.Errorf("bind-mounting subdir %s: %w", subHost, err)
			}
			h.log.WithField("dir", name).Debug("subdirectory injected")
			continue
		}

		// Only mount shared library files (.so or .so.*)
		if !isSharedLibrary(name) {
			continue
		}

		src := filepath.Join(hostDir, name)
		dst := filepath.Join(targetDir, name)
		if err := ensureFile(dst); err != nil {
			return fmt.Errorf("creating placeholder for %s: %w", dst, err)
		}
		if err := bindMount(src, dst); err != nil {
			return fmt.Errorf("bind-mounting library %s: %w", src, err)
		}
		mounted++
	}

	h.log.WithFields(logrus.Fields{
		"hostDir": hostDir,
		"count":   mounted,
	}).Info("shared libraries injected (so-only mode)")
	return nil
}

// isSharedLibrary returns true if the filename looks like a shared library:
// libfoo.so, libfoo.so.1, libfoo.so.1.2.3, etc.
func isSharedLibrary(name string) bool {
	// Match *.so
	if strings.HasSuffix(name, ".so") {
		return true
	}
	// Match *.so.N[.N...]
	if idx := strings.Index(name, ".so."); idx >= 0 {
		return true
	}
	return false
}

// injectDriverBinaries bind-mounts host driver binary directories into the container.
func (h *Hook) injectDriverBinaries(rootfs string) error {
	containerRoot := h.cfg.Hook.ContainerDriverRoot

	for _, hostPath := range h.cfg.Hook.DriverBinaryPaths {
		if _, err := os.Stat(hostPath); os.IsNotExist(err) {
			h.log.WithField("path", hostPath).Debug("driver binary path not found on host, skipping")
			continue
		}

		target := filepath.Join(rootfs, containerRoot, "bin")
		if err := os.MkdirAll(target, 0755); err != nil {
			return fmt.Errorf("creating bin dir %s: %w", target, err)
		}
		if err := bindMount(hostPath, target); err != nil {
			return fmt.Errorf("bind-mounting bin dir %s: %w", hostPath, err)
		}
		h.log.WithField("path", hostPath).Debug("driver binary dir injected")
	}
	return nil
}

// injectLdSoConf writes a .conf file into /etc/ld.so.conf.d inside the
// container so that the dynamic linker picks up the driver libraries.
func (h *Hook) injectLdSoConf(rootfs, containerDriverRoot string) error {
	confDir := filepath.Join(rootfs, "etc", "ld.so.conf.d")
	if err := os.MkdirAll(confDir, 0755); err != nil {
		return err
	}

	var lines []string
	for _, p := range h.cfg.Hook.DriverLibraryPaths {
		rel, err := filepath.Rel(h.cfg.Hook.ContainerDriverRoot, p)
		if err != nil || strings.HasPrefix(rel, "..") {
			rel = filepath.Base(p)
		}
		lines = append(lines, filepath.Join(containerDriverRoot, rel))
	}

	content := strings.Join(lines, "\n") + "\n"
	confFile := filepath.Join(confDir, "ix-toolkit.conf")
	return os.WriteFile(confFile, []byte(content), 0644)
}

// ociState is the minimal subset of the OCI container state passed to hooks on stdin.
type ociState struct {
	ID     string `json:"id"`
	Bundle string `json:"bundle"`
	Status string `json:"status"`
	Pid    int    `json:"pid"`
}

func readContainerState(r io.Reader) (*ociState, error) {
	var s ociState
	if err := json.NewDecoder(r).Decode(&s); err != nil {
		return nil, err
	}
	return &s, nil
}

func loadSpec(bundle string) (*specs.Spec, error) {
	specPath := filepath.Join(bundle, "config.json")
	data, err := os.ReadFile(filepath.Clean(specPath))
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", specPath, err)
	}
	var spec specs.Spec
	if err := json.Unmarshal(data, &spec); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", specPath, err)
	}
	return &spec, nil
}
