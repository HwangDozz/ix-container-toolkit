package cdi

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/sirupsen/logrus"

	"github.com/accelerator-toolkit/accelerator-toolkit/pkg/device"
	"github.com/accelerator-toolkit/accelerator-toolkit/pkg/profile"
	"github.com/accelerator-toolkit/accelerator-toolkit/pkg/strutil"
)

// Generator produces node-local CDI specs by inspecting the host filesystem.
type Generator struct {
	profile *profile.Profile
	devices []device.Device
	log     *logrus.Logger
}

// NewGenerator creates a CDI spec generator for the given profile and discovered devices.
func NewGenerator(p *profile.Profile, devs []device.Device, log *logrus.Logger) *Generator {
	return &Generator{profile: p, devices: devs, log: log}
}

// Generate produces a CDI spec with one device entry per physical accelerator device.
func (g *Generator) Generate() (*Spec, error) {
	if err := g.profile.Validate(); err != nil {
		return nil, fmt.Errorf("invalid profile: %w", err)
	}
	if len(g.devices) == 0 {
		return nil, fmt.Errorf("no devices provided for CDI spec generation")
	}

	kind := g.cdiKind()
	commonEdits, err := g.commonContainerEdits()
	if err != nil {
		return nil, fmt.Errorf("generating common container edits: %w", err)
	}

	devEntries := make([]Device, 0, len(g.devices))
	for _, dev := range g.devices {
		name := g.deviceName(dev)
		edits := g.mergeDeviceEdits(commonEdits, dev)
		devEntries = append(devEntries, Device{
			Name:           name,
			ContainerEdits: edits,
		})
	}

	return &Spec{
		CDIVersion: SpecVersion,
		Kind:       kind,
		Devices:    devEntries,
	}, nil
}

// cdiKind returns the CDI kind derived from the first kubernetes resource name.
func (g *Generator) cdiKind() string {
	return strings.TrimSpace(g.profile.Kubernetes.ResourceNames[0])
}

// deviceName returns the CDI device name for a specific device.
// Uses UUID when available, otherwise falls back to "index-N".
// This must match the naming in pkg/dra/resourceslice.go deviceName().
func (g *Generator) deviceName(dev device.Device) string {
	if dev.UUID != "" {
		return dev.UUID
	}
	return fmt.Sprintf("index-%d", dev.Index)
}

// commonContainerEdits generates container edits shared by all device entries:
// environment variables, library mounts, directory mounts, and linker hooks.
func (g *Generator) commonContainerEdits() (ContainerEdits, error) {
	env := g.buildEnv()
	mounts, err := g.buildMounts()
	if err != nil {
		return ContainerEdits{}, err
	}
	hooks := g.buildHooks()

	return ContainerEdits{
		Env:    env,
		Mounts: mounts,
		Hooks:  hooks,
	}, nil
}

// mergeDeviceEdits creates per-device edits by combining common edits with
// device-specific device nodes.
func (g *Generator) mergeDeviceEdits(common ContainerEdits, dev device.Device) ContainerEdits {
	env := append([]string(nil), common.Env...)
	mounts := append([]Mount(nil), common.Mounts...)

	deviceNodes := []DeviceNode{
		{
			HostPath:    dev.Path,
			Path:        dev.Path,
			Permissions: "rwm",
		},
	}

	// Add control device nodes.
	for _, pattern := range g.profile.Device.ControlDeviceGlobs {
		matches, err := filepath.Glob(pattern)
		if err != nil {
			g.log.WithError(err).WithField("pattern", pattern).Warn("invalid control device glob")
			continue
		}
		for _, m := range matches {
			deviceNodes = append(deviceNodes, DeviceNode{
				HostPath:    m,
				Path:        m,
				Permissions: "rwm",
			})
		}
	}

	var hooks []Hook
	if len(common.Hooks) > 0 {
		hooks = append(hooks, common.Hooks...)
	}

	return ContainerEdits{
		Env:         env,
		DeviceNodes: deviceNodes,
		Mounts:      mounts,
		Hooks:       hooks,
	}
}

// buildEnv returns the sorted environment variable list from the profile.
func (g *Generator) buildEnv() []string {
	var env []string

	// Inject the selector env var with "all" value so containers see all devices.
	if len(g.profile.Device.SelectorEnvVars) > 0 {
		env = append(env, g.profile.Device.SelectorEnvVars[0]+"=all")
	}

	// Inject extra env vars from the profile.
	keys := make([]string, 0, len(g.profile.Inject.ExtraEnv))
	for k := range g.profile.Inject.ExtraEnv {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		env = append(env, k+"="+g.profile.Inject.ExtraEnv[k])
	}

	return env
}

// buildMounts generates CDI mounts for all non-device artifacts in the profile.
func (g *Generator) buildMounts() ([]Mount, error) {
	var mounts []Mount
	seen := make(map[string]bool)

	for _, artifact := range g.profile.Inject.Artifacts {
		if artifact.Kind == "device-nodes" {
			continue
		}

		resolvedPaths, err := resolveHostPaths(artifact.HostPaths, artifact.Optional, g.log)
		if err != nil {
			return nil, fmt.Errorf("artifact %q: %w", artifact.Name, err)
		}

		for _, hp := range resolvedPaths {
			artifactMounts, err := g.buildArtifactMounts(artifact, hp)
			if err != nil {
				return nil, fmt.Errorf("artifact %q path %s: %w", artifact.Name, hp, err)
			}
			for _, m := range artifactMounts {
				key := m.HostPath + "\x00" + m.ContainerPath
				if seen[key] {
					continue
				}
				seen[key] = true
				mounts = append(mounts, m)
			}
		}
	}

	return mounts, nil
}

// buildArtifactMounts generates mounts for a single artifact host path.
func (g *Generator) buildArtifactMounts(artifact profile.Artifact, hostPath string) ([]Mount, error) {
	info, err := os.Stat(hostPath)
	if err != nil {
		return nil, fmt.Errorf("stat %s: %w", hostPath, err)
	}

	if !info.IsDir() {
		// Single file artifact.
		containerPath := artifactContainerPath(g.profile.Inject.ContainerRoot, artifact.ContainerPath, hostPath)
		return []Mount{{
			HostPath:      hostPath,
			ContainerPath: containerPath,
			Options:       []string{"rbind", "ro"},
		}}, nil
	}

	switch artifact.Mode {
	case "bind":
		containerPath := artifactContainerPath(g.profile.Inject.ContainerRoot, artifact.ContainerPath, hostPath)
		return []Mount{{
			HostPath:      hostPath,
			ContainerPath: containerPath,
			Options:       []string{"rbind", "ro"},
		}}, nil

	case "so-only":
		return g.buildSoOnlyMounts(artifact, hostPath)

	case "copy":
		// For copy mode, bind-mount is the closest CDI equivalent.
		// The runtime will see the host directory as-is.
		containerPath := artifactContainerPath(g.profile.Inject.ContainerRoot, artifact.ContainerPath, hostPath)
		return []Mount{{
			HostPath:      hostPath,
			ContainerPath: containerPath,
			Options:       []string{"rbind", "ro"},
		}}, nil

	default:
		return nil, fmt.Errorf("unsupported artifact mode %q", artifact.Mode)
	}
}

// buildSoOnlyMounts scans a host directory and produces mounts for .so files
// and non-excluded subdirectories.
func (g *Generator) buildSoOnlyMounts(artifact profile.Artifact, hostDir string) ([]Mount, error) {
	excludeSet := make(map[string]bool, len(artifact.ExcludeDirs))
	for _, d := range artifact.ExcludeDirs {
		excludeSet[d] = true
	}

	entries, err := os.ReadDir(hostDir)
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", hostDir, err)
	}

	baseContainerPath := artifactContainerPath(g.profile.Inject.ContainerRoot, artifact.ContainerPath, hostDir)
	var mounts []Mount

	for _, entry := range entries {
		name := entry.Name()

		if entry.IsDir() {
			if excludeSet[name] {
				continue
			}
			subHost := filepath.Join(hostDir, name)
			subContainer := filepath.Join(baseContainerPath, name)
			mounts = append(mounts, Mount{
				HostPath:      subHost,
				ContainerPath: subContainer,
				Options:       []string{"rbind", "ro"},
			})
			continue
		}

		if !strutil.IsSharedLibrary(name) {
			continue
		}

		src := filepath.Join(hostDir, name)
		dst := filepath.Join(baseContainerPath, name)

		// Resolve symlinks to get the real host path for the mount.
		resolved := src
		if info, err := os.Lstat(src); err == nil && info.Mode()&os.ModeSymlink != 0 {
			if real, err := filepath.EvalSymlinks(src); err == nil {
				resolved = real
			}
		}

		if info, err := os.Stat(resolved); err == nil && info.Size() == 0 {
			g.log.WithField("path", resolved).Debug("skipping empty shared library")
			continue
		}

		mounts = append(mounts, Mount{
			HostPath:      resolved,
			ContainerPath: dst,
			Options:       []string{"rbind", "ro"},
		})
	}

	g.log.WithFields(logrus.Fields{
		"dir":    hostDir,
		"mounts": len(mounts),
	}).Debug("expanded so-only artifact into individual mounts")

	return mounts, nil
}

// buildHooks generates CDI hooks for ldconfig if the profile linker requires it.
func (g *Generator) buildHooks() []Hook {
	linker := g.profile.Inject.Linker
	if linker.ConfigPath == "" {
		return nil
	}

	var hooks []Hook

	// Prestart hook to write ld.so.conf.d snippet and run ldconfig.
	if linker.RunLdconfig {
		hooks = append(hooks, Hook{
			HookName: "prestart",
			Path:     "/sbin/ldconfig",
			Args:     []string{"ldconfig"},
		})
	}

	return hooks
}

// artifactContainerPath computes the container path for an artifact mount.
func artifactContainerPath(containerRoot, containerPath, hostPath string) string {
	rel, err := filepath.Rel(containerRoot, hostPath)
	if err != nil || strings.HasPrefix(rel, "..") {
		rel = filepath.Base(hostPath)
	}
	return filepath.Join(containerPath, rel)
}

// resolveHostPaths resolves symlinks and filters optional missing paths.
func resolveHostPaths(paths []string, optional bool, log *logrus.Logger) ([]string, error) {
	seen := make(map[string]bool, len(paths))
	var resolved []string

	for _, p := range paths {
		if seen[p] {
			continue
		}
		seen[p] = true

		if _, err := os.Stat(p); os.IsNotExist(err) {
			if optional {
				log.WithField("path", p).Debug("optional path not found, skipping")
				continue
			}
			return nil, fmt.Errorf("required path %s not found", p)
		}

		resolved = append(resolved, p)
	}

	return resolved, nil
}
