// Package hook implements the OCI prestart hook that injects accelerator
// devices and driver artifacts into a container.
//
// The hook is called by the OCI runtime (e.g. runc) right before the container
// process starts, with stdin containing the OCI container state JSON. It:
//
//  1. Reads the container state to find the container bundle/rootfs.
//  2. Inspects the container spec to determine which accelerator devices to expose via the
//     configured device selector environment variable.
//  3. Discovers matching device nodes on the host for the requested selectors.
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

	"github.com/accelerator-toolkit/accelerator-toolkit/pkg/device"
	"github.com/accelerator-toolkit/accelerator-toolkit/pkg/profile"
	"github.com/accelerator-toolkit/accelerator-toolkit/pkg/runtimeview"
)

// Hook is the main hook executor.
type Hook struct {
	view *runtimeview.View
	log  *logrus.Logger
}

// New creates a Hook using the provided runtime view and logger.
func New(view *runtimeview.View, log *logrus.Logger) *Hook {
	return &Hook{view: view, log: log}
}

// Run executes the OCI prestart hook. It reads the OCI container state from r
// and injects accelerator devices and driver libraries into the container rootfs.
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

	// 3. Determine which accelerator devices the container requested.
	visibleDevices := h.visibleDevices(spec)
	if visibleDevices == "" && !h.view.DisableRequire() {
		// Container did not request any GPU — nothing to do.
		h.log.WithField("selectorEnvVars", h.view.SelectorEnvVars()).Debug("no accelerator requested, skipping injection")
		return nil
	}
	if strings.EqualFold(strings.TrimSpace(visibleDevices), "none") {
		h.log.WithField("selectorEnvVars", h.view.SelectorEnvVars()).Debug("selector explicitly disabled device injection")
		return nil
	}

	rootfs := spec.Root.Path
	if !filepath.IsAbs(rootfs) {
		rootfs = filepath.Join(state.Bundle, rootfs)
	}
	h.log.WithFields(logrus.Fields{
		"rootfs":         rootfs,
		"visibleDevices": visibleDevices,
	}).Info("injecting accelerator artifacts into container")

	// 4. Discover matching device nodes on the host.
	devs, err := device.DiscoverWithProfile(visibleDevices, h.view.Profile(), h.log)
	if err != nil {
		return fmt.Errorf("discovering devices: %w", err)
	}
	if len(devs) == 0 {
		if h.view.DisableRequire() {
			h.log.Warn("no matching accelerator devices found, continuing because disableRequire=true")
			return nil
		}
		return fmt.Errorf("no matching accelerator devices found for selector value %q", visibleDevices)
	}

	// 5. Inject profile artifacts.
	if err := h.injectArtifacts(rootfs, devs); err != nil {
		return fmt.Errorf("injecting profile artifacts: %w", err)
	}

	return nil
}

// visibleDevices returns the value of the configured selector environment
// variable in the container spec.
func (h *Hook) visibleDevices(spec *specs.Spec) string {
	if spec.Process == nil {
		return ""
	}
	for _, selectorEnv := range h.view.SelectorEnvVars() {
		prefix := selectorEnv + "="
		for _, env := range spec.Process.Env {
			if strings.HasPrefix(env, prefix) {
				return strings.TrimPrefix(env, prefix)
			}
		}
	}
	return ""
}

// mountSharedLibraries walks hostDir (non-recursively) and bind-mounts only
// shared library files (.so, .so.1, .so.1.2.3, etc.) into targetDir, skipping
// subdirectories listed in excludeDirs.
func (h *Hook) mountSharedLibraries(hostDir, targetDir string, excludeDirs []string) error {
	if err := os.MkdirAll(targetDir, 0755); err != nil {
		return fmt.Errorf("creating target dir %s: %w", targetDir, err)
	}

	excludeSet := make(map[string]bool)
	for _, d := range excludeDirs {
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

		info, err := os.Lstat(src)
		if err != nil {
			return fmt.Errorf("stat %s: %w", src, err)
		}
		if info.Mode()&os.ModeSymlink != 0 {
			linkTarget, err := os.Readlink(src)
			if err != nil {
				return fmt.Errorf("readlink %s: %w", src, err)
			}
			if err := os.RemoveAll(dst); err != nil && !os.IsNotExist(err) {
				return err
			}
			if err := os.Symlink(linkTarget, dst); err != nil {
				return fmt.Errorf("symlink %s -> %s: %w", dst, linkTarget, err)
			}
		} else {
			if err := copyFile(src, dst, info.Mode().Perm()); err != nil {
				return fmt.Errorf("copying library %s: %w", src, err)
			}
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

func (h *Hook) injectArtifacts(rootfs string, devs []device.Device) error {
	for _, artifact := range h.view.Artifacts() {
		switch artifact.Kind {
		case "device-nodes":
			if err := h.injectDeviceArtifact(rootfs, devs, artifact); err != nil {
				return fmt.Errorf("artifact %s: %w", artifact.Name, err)
			}
		case "shared-libraries":
			if err := h.injectLibraryArtifact(rootfs, artifact); err != nil {
				return fmt.Errorf("artifact %s: %w", artifact.Name, err)
			}
		case "directory":
			if err := h.injectDirectoryArtifact(rootfs, artifact); err != nil {
				return fmt.Errorf("artifact %s: %w", artifact.Name, err)
			}
		default:
			return fmt.Errorf("unsupported artifact kind %q", artifact.Kind)
		}
	}

	if err := h.injectProfileLinker(rootfs); err != nil {
		h.log.WithError(err).Warn("profile linker injection failed (non-fatal)")
	}
	return nil
}

func (h *Hook) injectDeviceArtifact(rootfs string, devs []device.Device, artifact profile.Artifact) error {
	mounted := make(map[string]bool)
	for _, dev := range devs {
		if err := mountDevicePath(rootfs, dev.Path); err != nil {
			return err
		}
		mounted[dev.Path] = true
	}

	for _, controlPath := range h.controlDevicePaths() {
		if mounted[controlPath] {
			continue
		}
		if err := mountDevicePath(rootfs, controlPath); err != nil {
			return err
		}
		mounted[controlPath] = true
	}
	return nil
}

func mountDevicePath(rootfs, hostPath string) error {
	target := filepath.Join(rootfs, hostPath)
	if err := ensureFile(target); err != nil {
		return fmt.Errorf("creating device placeholder %s: %w", target, err)
	}
	if err := bindMount(hostPath, target); err != nil {
		return fmt.Errorf("bind-mounting device %s: %w", hostPath, err)
	}
	return nil
}

func (h *Hook) controlDevicePaths() []string {
	seen := make(map[string]bool)
	var resolved []string
	for _, pattern := range h.view.Profile().Device.ControlDeviceGlobs {
		matches, err := filepath.Glob(pattern)
		if err != nil {
			h.log.WithError(err).WithField("pattern", pattern).Warn("invalid control device glob, skipping")
			continue
		}
		for _, match := range matches {
			if seen[match] {
				continue
			}
			seen[match] = true
			resolved = append(resolved, match)
		}
	}
	return resolved
}

func (h *Hook) injectLibraryArtifact(rootfs string, artifact profile.Artifact) error {
	for _, hostPath := range resolveArtifactHostPaths(artifact.HostPaths) {
		if _, err := os.Stat(hostPath.Source); os.IsNotExist(err) {
			if artifact.Optional {
				h.log.WithField("path", hostPath.Declared).Debug("optional library path not found on host, skipping")
				continue
			}
			return fmt.Errorf("library path %s not found on host", hostPath.Declared)
		}

		targetDir := artifactTargetDir(rootfs, h.view.ContainerRoot(), artifact.ContainerPath, hostPath.Declared)
		switch artifact.Mode {
		case "bind":
			if err := os.MkdirAll(targetDir, 0755); err != nil {
				return fmt.Errorf("creating lib dir %s: %w", targetDir, err)
			}
			if err := bindMount(hostPath.Source, targetDir); err != nil {
				return fmt.Errorf("bind-mounting lib dir %s: %w", hostPath.Source, err)
			}
		case "so-only":
			if err := h.mountSharedLibraries(hostPath.Source, targetDir, artifact.ExcludeDirs); err != nil {
				return fmt.Errorf("mounting shared libraries from %s: %w", hostPath.Source, err)
			}
		default:
			return fmt.Errorf("unsupported library artifact mode %q", artifact.Mode)
		}
	}
	return nil
}

func (h *Hook) injectDirectoryArtifact(rootfs string, artifact profile.Artifact) error {
	for _, hostPath := range resolveArtifactHostPaths(artifact.HostPaths) {
		if _, err := os.Stat(hostPath.Source); os.IsNotExist(err) {
			if artifact.Optional {
				h.log.WithField("path", hostPath.Declared).Debug("optional directory path not found on host, skipping")
				continue
			}
			return fmt.Errorf("directory path %s not found on host", hostPath.Declared)
		}

		target := artifactTargetDir(rootfs, h.view.ContainerRoot(), artifact.ContainerPath, hostPath.Declared)
		if err := os.MkdirAll(target, 0755); err != nil {
			return fmt.Errorf("creating dir %s: %w", target, err)
		}

		switch artifact.Mode {
		case "bind":
			if err := h.mountDriverBinaries(hostPath.Source, target); err != nil {
				return fmt.Errorf("mounting directory artifact from %s: %w", hostPath.Source, err)
			}
		case "copy":
			if err := copyDir(hostPath.Source, target); err != nil {
				return fmt.Errorf("copying directory artifact from %s: %w", hostPath.Source, err)
			}
		default:
			return fmt.Errorf("unsupported directory artifact mode %q", artifact.Mode)
		}
	}
	return nil
}

func (h *Hook) injectProfileLinker(rootfs string) error {
	linker := h.view.Linker()
	if linker.ConfigPath == "" {
		return nil
	}

	confFile := filepath.Join(rootfs, linker.ConfigPath)
	if err := os.MkdirAll(filepath.Dir(confFile), 0755); err != nil {
		return err
	}

	content := strings.Join(linker.Paths, "\n") + "\n"
	if err := os.WriteFile(confFile, []byte(content), 0644); err != nil {
		return err
	}

	if err := h.ensureLibrarySymlink(rootfs, h.view.ContainerRoot()); err != nil {
		h.log.WithError(err).Warn("failed to create library symlink (non-fatal)")
	}

	if linker.RunLdconfig {
		if err := runLdconfig(rootfs); err != nil {
			return err
		}
	}
	return nil
}

func artifactTargetDir(rootfs, containerRoot, containerPath, hostPath string) string {
	rel, err := filepath.Rel(containerRoot, hostPath)
	if err != nil || strings.HasPrefix(rel, "..") {
		rel = filepath.Base(hostPath)
	}
	return filepath.Join(rootfs, containerPath, rel)
}

type artifactHostPath struct {
	Declared string
	Source   string
}

func resolveArtifactHostPaths(paths []string) []artifactHostPath {
	seen := make(map[string]bool, len(paths))
	resolved := make([]artifactHostPath, 0, len(paths))
	for _, declared := range paths {
		if seen[declared] {
			continue
		}
		seen[declared] = true

		source := declared
		if real, err := filepath.EvalSymlinks(declared); err == nil {
			source = real
		}
		resolved = append(resolved, artifactHostPath{
			Declared: declared,
			Source:   source,
		})
	}
	return resolved
}

func (h *Hook) ensureLibrarySymlink(rootfs, containerRoot string) error {
	hostLib := filepath.Join(containerRoot, "lib")
	info, err := os.Lstat(hostLib)
	if err != nil || info.Mode()&os.ModeSymlink == 0 {
		return nil
	}

	target, err := os.Readlink(hostLib)
	if err != nil {
		return err
	}

	containerLib := filepath.Join(rootfs, containerRoot, "lib")
	if err := os.MkdirAll(filepath.Dir(containerLib), 0755); err != nil {
		return err
	}
	if err := os.RemoveAll(containerLib); err != nil && !os.IsNotExist(err) {
		return err
	}
	return os.Symlink(target, containerLib)
}

func (h *Hook) mountDriverBinaries(hostDir, targetDir string) error {
	entries, err := os.ReadDir(hostDir)
	if err != nil {
		return fmt.Errorf("reading directory %s: %w", hostDir, err)
	}

	for _, entry := range entries {
		name := entry.Name()
		src := filepath.Join(hostDir, name)
		dst := filepath.Join(targetDir, name)

		info, err := os.Lstat(src)
		if err != nil {
			return fmt.Errorf("stat %s: %w", src, err)
		}

		switch mode := info.Mode(); {
		case mode&os.ModeSymlink != 0:
			linkTarget, err := os.Readlink(src)
			if err != nil {
				return fmt.Errorf("readlink %s: %w", src, err)
			}
			if err := os.RemoveAll(dst); err != nil && !os.IsNotExist(err) {
				return err
			}
			if err := os.Symlink(linkTarget, dst); err != nil {
				return fmt.Errorf("symlink %s -> %s: %w", dst, linkTarget, err)
			}
		case entry.IsDir():
			if err := os.MkdirAll(dst, 0755); err != nil {
				return fmt.Errorf("creating bin subdir %s: %w", dst, err)
			}
			if err := copyDir(src, dst); err != nil {
				return fmt.Errorf("copying bin subdir %s: %w", src, err)
			}
		default:
			if err := copyFile(src, dst, info.Mode().Perm()); err != nil {
				return fmt.Errorf("copying binary %s: %w", src, err)
			}
		}
	}

	return nil
}

func copyDir(srcDir, dstDir string) error {
	entries, err := os.ReadDir(srcDir)
	if err != nil {
		return err
	}

	for _, entry := range entries {
		src := filepath.Join(srcDir, entry.Name())
		dst := filepath.Join(dstDir, entry.Name())

		info, err := os.Lstat(src)
		if err != nil {
			return err
		}

		switch mode := info.Mode(); {
		case mode&os.ModeSymlink != 0:
			linkTarget, err := os.Readlink(src)
			if err != nil {
				return err
			}
			if err := os.RemoveAll(dst); err != nil && !os.IsNotExist(err) {
				return err
			}
			if err := os.Symlink(linkTarget, dst); err != nil {
				return err
			}
		case entry.IsDir():
			if err := os.MkdirAll(dst, 0755); err != nil {
				return err
			}
			if err := copyDir(src, dst); err != nil {
				return err
			}
		default:
			if err := copyFile(src, dst, info.Mode().Perm()); err != nil {
				return err
			}
		}
	}

	return nil
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

func uniquePreserveOrder(paths []string) []string {
	seen := make(map[string]struct{}, len(paths))
	out := make([]string, 0, len(paths))
	for _, p := range paths {
		if _, ok := seen[p]; ok {
			continue
		}
		seen[p] = struct{}{}
		out = append(out, p)
	}
	return out
}
