package runtimeview

import (
	"github.com/accelerator-toolkit/accelerator-toolkit/pkg/config"
	"github.com/accelerator-toolkit/accelerator-toolkit/pkg/device"
	"github.com/accelerator-toolkit/accelerator-toolkit/pkg/profile"
)

// View presents the runtime-facing configuration derived from the active
// profile together with process-level settings loaded from config.json.
type View struct {
	cfg     *config.Config
	profile *profile.Profile
}

func New(cfg *config.Config, p *profile.Profile) *View {
	return &View{cfg: cfg, profile: p}
}

func Load(configPath, profilePath string) (*View, error) {
	resolvedProfilePath := config.ResolveProfilePath(profilePath)
	if resolvedProfilePath == "" {
		return nil, config.ErrProfileRequired
	}

	prof, err := profile.Load(resolvedProfilePath)
	if err != nil {
		return nil, err
	}

	cfg, err := config.LoadWithLoadedProfile(configPath, prof)
	if err != nil {
		return nil, err
	}
	return New(cfg, prof), nil
}

func (v *View) Config() *config.Config {
	return v.cfg
}

func (v *View) Profile() *profile.Profile {
	return v.profile
}

func (v *View) HookPath() string {
	return v.cfg.HookPath
}

func (v *View) UnderlyingRuntime() string {
	return v.cfg.UnderlyingRuntime
}

func (v *View) HandlerName() string {
	return profile.UnifiedRuntimeName
}

func (v *View) RuntimeClassName() string {
	return profile.UnifiedRuntimeName
}

func (v *View) NodeLabels() map[string]string {
	return v.profile.Kubernetes.NodeLabels
}

func (v *View) SelectorEnvVars() []string {
	return v.profile.Device.SelectorEnvVars
}

func (v *View) ExtraEnv() map[string]string {
	return v.profile.Inject.ExtraEnv
}

func (v *View) DisableRequire() bool {
	return v.cfg.Hook.DisableRequire
}

func (v *View) DeviceResolverConfig() device.ResolverConfig {
	return device.ResolverConfigFromProfile(v.profile)
}

func (v *View) Artifacts() []profile.Artifact {
	return v.profile.Inject.Artifacts
}

func (v *View) ContainerRoot() string {
	return v.profile.Inject.ContainerRoot
}

func (v *View) Linker() profile.Linker {
	return v.profile.Inject.Linker
}
