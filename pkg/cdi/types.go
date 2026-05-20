// Package cdi provides node-local CDI (Container Device Interface) spec generation.
//
// Unlike the static CDI renderer in pkg/profile/cdi.go, this package generates
// concrete CDI specs by inspecting the host filesystem: it expands device globs
// into actual device paths, filters shared libraries by file extension, and
// produces one CDI device entry per physical accelerator device.
package cdi

const SpecVersion = "0.8.0"

// Spec is the top-level CDI spec file structure.
type Spec struct {
	CDIVersion string   `yaml:"cdiVersion"`
	Kind       string   `yaml:"kind"`
	Devices    []Device `yaml:"devices"`
}

// Device represents a single CDI device entry.
type Device struct {
	Name           string         `yaml:"name"`
	ContainerEdits ContainerEdits `yaml:"containerEdits,omitempty"`
}

// ContainerEdits describes modifications applied to a container when a CDI device is injected.
type ContainerEdits struct {
	Env         []string     `yaml:"env,omitempty"`
	DeviceNodes []DeviceNode `yaml:"deviceNodes,omitempty"`
	Mounts      []Mount      `yaml:"mounts,omitempty"`
	Hooks       []Hook       `yaml:"hooks,omitempty"`
}

// DeviceNode describes a device node to expose in the container.
type DeviceNode struct {
	HostPath    string `yaml:"hostPath,omitempty"`
	Path        string `yaml:"path"`
	Permissions string `yaml:"permissions,omitempty"`
}

// Mount describes a file or directory mount.
type Mount struct {
	HostPath      string   `yaml:"hostPath"`
	ContainerPath string   `yaml:"containerPath"`
	Type          string   `yaml:"type,omitempty"`
	Options       []string `yaml:"options,omitempty"`
}

// Hook describes a single OCI lifecycle hook in CDI format.
// The hookName field indicates the lifecycle stage (e.g., "prestart", "createContainer").
type Hook struct {
	HookName string   `yaml:"hookName"`
	Path     string   `yaml:"path"`
	Args     []string `yaml:"args,omitempty"`
	Env      []string `yaml:"env,omitempty"`
	Timeout  int      `yaml:"timeout,omitempty"`
}
