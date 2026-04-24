package profile

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

const HookStagePrestart = "prestart"
const UnifiedRuntimeName = "xpu-runtime"
const InjectModeDelegateOnly = "delegate-only"

var validArtifactKinds = map[string]bool{
	"device-nodes":     true,
	"shared-libraries": true,
	"directory":        true,
}

var validArtifactModes = map[string]bool{
	"bind":    true,
	"copy":    true,
	"so-only": true,
}

type Profile struct {
	Metadata   Metadata   `yaml:"metadata"`
	Runtime    Runtime    `yaml:"runtime"`
	Kubernetes Kubernetes `yaml:"kubernetes"`
	Device     Device     `yaml:"device"`
	Inject     Inject     `yaml:"inject"`
}

type Metadata struct {
	Name        string `yaml:"name"`
	Vendor      string `yaml:"vendor"`
	ModelFamily string `yaml:"modelFamily"`
	Version     string `yaml:"version"`
}

type Runtime struct {
	UnderlyingRuntime string `yaml:"underlyingRuntime"`
	HookStage         string `yaml:"hookStage"`
	HookBinary        string `yaml:"hookBinary"`
	InjectMode        string `yaml:"injectMode"`
}

type Kubernetes struct {
	ResourceNames          []string               `yaml:"resourceNames"`
	NodeLabels             map[string]string      `yaml:"nodeLabels"`
	RuntimeClassScheduling RuntimeClassScheduling `yaml:"runtimeClassScheduling"`
}

type RuntimeClassScheduling struct {
	NodeSelector map[string]string `yaml:"nodeSelector"`
	Tolerations  []Toleration      `yaml:"tolerations"`
}

type Toleration struct {
	Key      string `yaml:"key,omitempty"`
	Operator string `yaml:"operator"`
	Effect   string `yaml:"effect"`
	Value    string `yaml:"value,omitempty"`
}

type Device struct {
	SelectorEnvVars    []string      `yaml:"selectorEnvVars"`
	SelectorFormats    []string      `yaml:"selectorFormats"`
	DeviceGlobs        []string      `yaml:"deviceGlobs"`
	ControlDeviceGlobs []string      `yaml:"controlDeviceGlobs"`
	Mapping            DeviceMapping `yaml:"mapping"`
	FallbackPolicy     []string      `yaml:"fallbackPolicy"`
}

type DeviceMapping struct {
	Strategy MappingStrategy `yaml:"strategy"`
	Command  MappingCommand  `yaml:"command"`
	Parser   string          `yaml:"parser"`
}

type MappingStrategy struct {
	Primary  string `yaml:"primary"`
	Fallback string `yaml:"fallback"`
}

type MappingCommand struct {
	PathCandidates []string          `yaml:"pathCandidates"`
	Args           []string          `yaml:"args"`
	Env            map[string]string `yaml:"env"`
}

type Inject struct {
	ContainerRoot string            `yaml:"containerRoot"`
	Artifacts     []Artifact        `yaml:"artifacts"`
	Linker        Linker            `yaml:"linker"`
	ExtraEnv      map[string]string `yaml:"extraEnv"`
}

type Artifact struct {
	Name          string   `yaml:"name"`
	Kind          string   `yaml:"kind"`
	HostPaths     []string `yaml:"hostPaths"`
	ContainerPath string   `yaml:"containerPath"`
	Mode          string   `yaml:"mode"`
	ExcludeDirs   []string `yaml:"excludeDirs"`
	Optional      bool     `yaml:"optional"`
}

type Linker struct {
	Strategy    string   `yaml:"strategy"`
	ConfigPath  string   `yaml:"configPath"`
	Paths       []string `yaml:"paths"`
	RunLdconfig bool     `yaml:"runLdconfig"`
}

func Load(path string) (*Profile, error) {
	if path == "" {
		return nil, fmt.Errorf("profile path is required")
	}

	data, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		return nil, fmt.Errorf("reading profile %s: %w", path, err)
	}

	var p Profile
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	decoder.KnownFields(true)
	if err := decoder.Decode(&p); err != nil {
		return nil, fmt.Errorf("parsing profile %s: %w", path, err)
	}

	if err := p.Validate(); err != nil {
		return nil, fmt.Errorf("validating profile %s: %w", path, err)
	}

	return &p, nil
}

func (p *Profile) Validate() error {
	if p.Metadata.Name == "" {
		return fmt.Errorf("metadata.name is required")
	}
	if p.Metadata.Vendor == "" {
		return fmt.Errorf("metadata.vendor is required")
	}
	if p.Metadata.Version == "" {
		return fmt.Errorf("metadata.version is required")
	}
	if p.Runtime.UnderlyingRuntime == "" {
		return fmt.Errorf("runtime.underlyingRuntime is required")
	}
	if p.Runtime.HookStage != HookStagePrestart {
		return fmt.Errorf("runtime.hookStage must be %q", HookStagePrestart)
	}
	if p.Runtime.HookBinary == "" {
		return fmt.Errorf("runtime.hookBinary is required")
	}
	if p.Runtime.InjectMode != "" && p.Runtime.InjectMode != InjectModeDelegateOnly {
		return fmt.Errorf("runtime.injectMode must be empty or %q", InjectModeDelegateOnly)
	}
	if len(p.Kubernetes.ResourceNames) == 0 {
		return fmt.Errorf("kubernetes.resourceNames must not be empty")
	}
	if len(p.Device.SelectorEnvVars) == 0 {
		return fmt.Errorf("device.selectorEnvVars must not be empty")
	}
	if len(p.Device.DeviceGlobs) == 0 {
		return fmt.Errorf("device.deviceGlobs must not be empty")
	}
	if p.Device.Mapping.Strategy.Primary == "" {
		return fmt.Errorf("device.mapping.strategy.primary is required")
	}
	if mappingCommandRequired(p.Device.Mapping.Strategy) {
		if p.Device.Mapping.Parser == "" {
			return fmt.Errorf("device.mapping.parser is required")
		}
		if len(p.Device.Mapping.Command.PathCandidates) == 0 {
			return fmt.Errorf("device.mapping.command.pathCandidates must not be empty")
		}
		if len(p.Device.Mapping.Command.Args) == 0 {
			return fmt.Errorf("device.mapping.command.args must not be empty")
		}
	}
	if p.Inject.ContainerRoot == "" {
		return fmt.Errorf("inject.containerRoot is required")
	}
	if len(p.Inject.Artifacts) == 0 {
		return fmt.Errorf("inject.artifacts must not be empty")
	}
	for i, artifact := range p.Inject.Artifacts {
		if artifact.Name == "" {
			return fmt.Errorf("inject.artifacts[%d].name is required", i)
		}
		if !validArtifactKinds[artifact.Kind] {
			return fmt.Errorf("inject.artifacts[%d].kind %q is invalid", i, artifact.Kind)
		}
		if len(artifact.HostPaths) == 0 {
			return fmt.Errorf("inject.artifacts[%d].hostPaths must not be empty", i)
		}
		if artifact.ContainerPath == "" {
			return fmt.Errorf("inject.artifacts[%d].containerPath is required", i)
		}
		if !validArtifactModes[artifact.Mode] {
			return fmt.Errorf("inject.artifacts[%d].mode %q is invalid", i, artifact.Mode)
		}
	}
	if p.Inject.Linker.ConfigPath == "" {
		return fmt.Errorf("inject.linker.configPath is required")
	}
	return nil
}

func mappingCommandRequired(strategy MappingStrategy) bool {
	return strategyRequiresCommand(strategy.Primary) || strategyRequiresCommand(strategy.Fallback)
}

func strategyRequiresCommand(strategy string) bool {
	return strings.HasPrefix(strings.TrimSpace(strategy), "command")
}
