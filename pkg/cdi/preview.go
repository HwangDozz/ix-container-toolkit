package cdi

import (
	"fmt"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/accelerator-toolkit/accelerator-toolkit/pkg/profile"
)

// RenderPreviewSpec renders a CDI spec prototype directly from a profile,
// without inspecting the host filesystem. It produces a single device entry
// with the given name (defaulting to "all") and is useful for previewing
// CDI spec structure on non-GPU nodes.
//
// For node-local CDI specs with per-device entries, use Generator instead.
func RenderPreviewSpec(p *profile.Profile, deviceName string) ([]byte, error) {
	if p == nil {
		return nil, fmt.Errorf("profile is nil")
	}
	if err := p.Validate(); err != nil {
		return nil, fmt.Errorf("validate profile before rendering cdi spec: %w", err)
	}

	name := strings.TrimSpace(deviceName)
	if name == "" {
		name = "all"
	}

	spec := Spec{
		CDIVersion: SpecVersion,
		Kind:       previewCDIKind(p),
		Devices: []Device{
			{
				Name:           name,
				ContainerEdits: previewContainerEdits(p),
			},
		},
	}

	data, err := yaml.Marshal(&spec)
	if err != nil {
		return nil, fmt.Errorf("marshal cdi spec: %w", err)
	}
	return data, nil
}

func previewCDIKind(p *profile.Profile) string {
	return strings.TrimSpace(p.Kubernetes.ResourceNames[0])
}

func previewContainerEdits(p *profile.Profile) ContainerEdits {
	return ContainerEdits{
		Env:         previewEnv(p),
		DeviceNodes: previewDeviceNodes(p),
		Mounts:      previewMounts(p),
	}
}

func previewEnv(p *profile.Profile) []string {
	var env []string
	if supportsAllSelector(p) && len(p.Device.SelectorEnvVars) > 0 {
		env = append(env, p.Device.SelectorEnvVars[0]+"=all")
	}

	keys := make([]string, 0, len(p.Inject.ExtraEnv))
	for key := range p.Inject.ExtraEnv {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		env = append(env, key+"="+p.Inject.ExtraEnv[key])
	}
	return env
}

func supportsAllSelector(p *profile.Profile) bool {
	for _, format := range p.Device.SelectorFormats {
		if strings.EqualFold(strings.TrimSpace(format), "all") {
			return true
		}
	}
	return false
}

func previewDeviceNodes(p *profile.Profile) []DeviceNode {
	var paths []string
	for _, artifact := range p.Inject.Artifacts {
		if artifact.Kind != "device-nodes" {
			continue
		}
		paths = append(paths, artifact.HostPaths...)
	}
	if len(paths) == 0 {
		paths = append(paths, p.Device.DeviceGlobs...)
		paths = append(paths, p.Device.ControlDeviceGlobs...)
	}

	paths = uniqueStrings(paths)
	nodes := make([]DeviceNode, 0, len(paths))
	for _, path := range paths {
		nodes = append(nodes, DeviceNode{
			HostPath:    path,
			Path:        path,
			Permissions: "rwm",
		})
	}
	return nodes
}

func previewMounts(p *profile.Profile) []Mount {
	var mounts []Mount
	seen := make(map[string]bool)
	for _, artifact := range p.Inject.Artifacts {
		if artifact.Kind == "device-nodes" {
			continue
		}
		for _, hostPath := range artifact.HostPaths {
			mount := Mount{
				HostPath:      hostPath,
				ContainerPath: artifactContainerPath(p.Inject.ContainerRoot, artifact.ContainerPath, hostPath),
				Options:       []string{"rbind", "ro"},
			}
			key := mount.HostPath + "\x00" + mount.ContainerPath
			if seen[key] {
				continue
			}
			seen[key] = true
			mounts = append(mounts, mount)
		}
	}
	return mounts
}

func uniqueStrings(in []string) []string {
	seen := make(map[string]bool, len(in))
	out := make([]string, 0, len(in))
	for _, value := range in {
		if seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	return out
}
