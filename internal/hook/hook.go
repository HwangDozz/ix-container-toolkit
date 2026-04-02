// Package hook implements the OCI prestart hook that injects Iluvatar GPU
// devices and driver libraries into a container.
//
// The hook is called by the OCI runtime (e.g. runc) right before the container
// process starts, with stdin containing the OCI container state JSON. It:
//
//  1. Reads the container state to find the container bundle/rootfs.
//  2. Inspects the container spec to determine which GPUs to expose via the
//     ILUVATAR_VISIBLE_DEVICES environment variable.
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
		h.log.Debug("no GPU requested (ILUVATAR_VISIBLE_DEVICES not set), skipping")
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
		return fmt.Errorf("no Iluvatar devices found for ILUVATAR_VISIBLE_DEVICES=%q", visibleDevices)
	}

	// 5. Inject devices and driver libraries.
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

// visibleDevices returns the value of the ILUVATAR_VISIBLE_DEVICES (or
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

// injectDriverLibraries bind-mounts host driver library directories into the
// container and registers them with the dynamic linker.
func (h *Hook) injectDriverLibraries(rootfs string) error {
	containerRoot := h.cfg.Hook.ContainerDriverRoot

	for _, hostPath := range h.cfg.Hook.DriverLibraryPaths {
		if _, err := os.Stat(hostPath); os.IsNotExist(err) {
			h.log.WithField("path", hostPath).Debug("driver library path not found on host, skipping")
			continue
		}

		// Map the host path under ContainerDriverRoot inside the container.
		// e.g. /usr/local/corex/lib64 → <rootfs>/usr/local/corex/lib64
		rel, err := filepath.Rel(containerRoot, hostPath)
		if err != nil || strings.HasPrefix(rel, "..") {
			// If the host path doesn't fall under containerRoot, mount it directly.
			rel = filepath.Base(hostPath)
		}
		target := filepath.Join(rootfs, containerRoot, rel)

		if err := os.MkdirAll(target, 0755); err != nil {
			return fmt.Errorf("creating lib dir %s: %w", target, err)
		}
		if err := bindMount(hostPath, target); err != nil {
			return fmt.Errorf("bind-mounting lib dir %s: %w", hostPath, err)
		}
		h.log.WithField("path", hostPath).Debug("driver library dir injected")
	}

	// Add an ld.so.conf.d entry so that the dynamic linker inside the container
	// discovers the freshly mounted libraries.
	if err := h.injectLdSoConf(rootfs, containerRoot); err != nil {
		h.log.WithError(err).Warn("failed to inject ld.so.conf.d entry (non-fatal)")
	}

	return nil
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
