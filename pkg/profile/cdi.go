package profile

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

const CDISpecVersion = "1.1.0"

type CDISpec struct {
	CDIVersion string      `yaml:"cdiVersion"`
	Kind       string      `yaml:"kind"`
	Devices    []CDIDevice `yaml:"devices"`
}

type CDIDevice struct {
	Name           string            `yaml:"name"`
	ContainerEdits CDIContainerEdits `yaml:"containerEdits,omitempty"`
}

type CDIContainerEdits struct {
	Env         []string        `yaml:"env,omitempty"`
	DeviceNodes []CDIDeviceNode `yaml:"deviceNodes,omitempty"`
	Mounts      []CDIMount      `yaml:"mounts,omitempty"`
}

type CDIDeviceNode struct {
	HostPath    string `yaml:"hostPath,omitempty"`
	Path        string `yaml:"path"`
	Permissions string `yaml:"permissions,omitempty"`
}

type CDIMount struct {
	HostPath      string   `yaml:"hostPath"`
	ContainerPath string   `yaml:"containerPath"`
	Options       []string `yaml:"options,omitempty"`
}

// RenderCDISpecYAML renders a node-local CDI spec shape from the profile.
//
// The first implementation intentionally preserves declared profile paths,
// including globs. A future node-local generator can expand those globs into
// concrete CDI device entries after inspecting the host.
func (p *Profile) RenderCDISpecYAML(deviceName string) ([]byte, error) {
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

	spec := CDISpec{
		CDIVersion: CDISpecVersion,
		Kind:       p.cdiKind(),
		Devices: []CDIDevice{
			{
				Name:           name,
				ContainerEdits: p.cdiContainerEdits(),
			},
		},
	}

	data, err := yaml.Marshal(&spec)
	if err != nil {
		return nil, fmt.Errorf("marshal cdi spec: %w", err)
	}
	return data, nil
}

func (p *Profile) cdiKind() string {
	return strings.TrimSpace(p.Kubernetes.ResourceNames[0])
}

func (p *Profile) cdiContainerEdits() CDIContainerEdits {
	return CDIContainerEdits{
		Env:         p.cdiEnv(),
		DeviceNodes: p.cdiDeviceNodes(),
		Mounts:      p.cdiMounts(),
	}
}

func (p *Profile) cdiEnv() []string {
	var env []string
	if p.supportsAllSelector() && len(p.Device.SelectorEnvVars) > 0 {
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

func (p *Profile) supportsAllSelector() bool {
	for _, format := range p.Device.SelectorFormats {
		if strings.EqualFold(strings.TrimSpace(format), "all") {
			return true
		}
	}
	return false
}

func (p *Profile) cdiDeviceNodes() []CDIDeviceNode {
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
	nodes := make([]CDIDeviceNode, 0, len(paths))
	for _, path := range paths {
		nodes = append(nodes, CDIDeviceNode{
			HostPath:    path,
			Path:        path,
			Permissions: "rwm",
		})
	}
	return nodes
}

func (p *Profile) cdiMounts() []CDIMount {
	var mounts []CDIMount
	seen := make(map[string]bool)
	for _, artifact := range p.Inject.Artifacts {
		if artifact.Kind == "device-nodes" {
			continue
		}
		for _, hostPath := range artifact.HostPaths {
			mount := CDIMount{
				HostPath:      hostPath,
				ContainerPath: cdiArtifactContainerPath(p.Inject.ContainerRoot, artifact.ContainerPath, hostPath),
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

func cdiArtifactContainerPath(containerRoot, containerPath, hostPath string) string {
	rel, err := filepath.Rel(containerRoot, hostPath)
	if err != nil || strings.HasPrefix(rel, "..") {
		rel = filepath.Base(hostPath)
	}
	return filepath.Join(containerPath, rel)
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
