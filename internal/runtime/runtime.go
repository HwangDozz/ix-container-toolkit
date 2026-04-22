// Package runtime implements accelerator-container-runtime, an OCI runtime shim that
// transparently wraps an underlying OCI runtime (e.g. runc) and injects the
// accelerator-container-hook as a prestart hook for containers that request accelerator
// devices.
//
// The shim intercepts the "create" sub-command. For all other sub-commands
// (start, delete, state, kill, …) it passes through to the underlying runtime
// without modification.
package runtime

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"syscall"

	"github.com/opencontainers/runtime-spec/specs-go"
	"github.com/sirupsen/logrus"
	"golang.org/x/sys/unix"

	"github.com/accelerator-toolkit/accelerator-toolkit/pkg/device"
	"github.com/accelerator-toolkit/accelerator-toolkit/pkg/runtimeview"
)

// Runtime is the OCI runtime shim.
type Runtime struct {
	view *runtimeview.View
	log  *logrus.Logger
}

// New creates a Runtime shim using the provided runtime view and logger.
func New(view *runtimeview.View, log *logrus.Logger) *Runtime {
	return &Runtime{view: view, log: log}
}

// Exec is the entry point: it receives os.Args (the full argv including argv[0])
// and acts as a drop-in replacement for runc.
func (r *Runtime) Exec(args []string) error {
	if len(args) < 2 {
		r.log.WithField("argv", args).Debug("runtime invoked without subcommand")
		return r.delegate(args)
	}

	// Find the "create" sub-command and the bundle path.
	cmd, bundlePath := parseArgs(args[1:])
	r.log.WithFields(logrus.Fields{
		"argv":   args,
		"cmd":    cmd,
		"bundle": bundlePath,
	}).Debug("parsed runtime arguments")

	if cmd != "create" || bundlePath == "" {
		if cmd == "create" && bundlePath == "" {
			r.log.WithField("argv", args).Warn("create command detected without bundle path; skipping hook injection")
		}
		// For everything other than "create", pass through unchanged.
		return r.delegate(args)
	}

	r.log.WithField("bundle", bundlePath).Debug("intercepting container create")

	// Modify the OCI spec to inject our prestart hook.
	if err := r.injectHook(bundlePath); err != nil {
		// Non-fatal: log and proceed without the hook so we don't break
		// containers that don't need GPU access.
		r.log.WithError(err).Warn("failed to inject accelerator-container-hook, proceeding without GPU support")
	}

	return r.delegate(args)
}

// injectHook reads config.json from bundle, injects the accelerator-container-hook
// as a prestart hook if the container requests GPUs, then rewrites config.json.
func (r *Runtime) injectHook(bundle string) error {
	specPath := filepath.Join(bundle, "config.json")
	data, err := os.ReadFile(filepath.Clean(specPath))
	if err != nil {
		return fmt.Errorf("reading %s: %w", specPath, err)
	}

	var spec specs.Spec
	if err := json.Unmarshal(data, &spec); err != nil {
		return fmt.Errorf("parsing %s: %w", specPath, err)
	}

	if !r.containerRequestsGPU(&spec) {
		r.log.Debug("container does not request GPU, skipping hook injection")
		return nil
	}

	hook := specs.Hook{
		Path: r.view.HookPath(),
		// No args needed; the hook reads everything from stdin (OCI state).
	}

	if spec.Hooks == nil {
		spec.Hooks = &specs.Hooks{}
	}
	// Prepend our hook so it runs before any user-defined prestart hooks.
	spec.Hooks.Prestart = append([]specs.Hook{hook}, spec.Hooks.Prestart...) //nolint:staticcheck

	r.injectExtraEnv(&spec)
	if err := r.injectLinuxDevices(&spec); err != nil {
		return err
	}

	modified, err := json.Marshal(&spec)
	if err != nil {
		return fmt.Errorf("marshalling modified spec: %w", err)
	}
	if err := os.WriteFile(specPath, modified, 0644); err != nil {
		return fmt.Errorf("writing %s: %w", specPath, err)
	}

	r.log.WithField("hookPath", r.view.HookPath()).Info("injected accelerator-container-hook as prestart hook")
	return nil
}

func (r *Runtime) injectExtraEnv(spec *specs.Spec) {
	extraEnv := r.view.ExtraEnv()
	if spec.Process == nil || len(extraEnv) == 0 {
		return
	}

	existing := make(map[string]bool, len(spec.Process.Env))
	for _, env := range spec.Process.Env {
		if idx := strings.IndexByte(env, '='); idx > 0 {
			existing[env[:idx]] = true
		}
	}

	keys := make([]string, 0, len(extraEnv))
	for key := range extraEnv {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	injected := 0
	for _, key := range keys {
		if existing[key] {
			continue
		}
		spec.Process.Env = append(spec.Process.Env, key+"="+extraEnv[key])
		injected++
	}
	if injected > 0 {
		r.log.WithField("count", injected).Info("injected extra OCI env from profile")
	}
}

func (r *Runtime) injectLinuxDevices(spec *specs.Spec) error {
	visibleDevices := r.visibleDevices(spec)
	if strings.EqualFold(strings.TrimSpace(visibleDevices), "none") {
		return nil
	}

	devs, err := device.DiscoverWithProfile(visibleDevices, r.view.Profile(), r.log)
	if err != nil {
		return fmt.Errorf("discovering devices for OCI spec injection: %w", err)
	}
	if len(devs) == 0 {
		if r.view.DisableRequire() {
			r.log.Warn("no matching accelerator devices found for OCI spec injection, continuing because disableRequire=true")
			return nil
		}
		return fmt.Errorf("no matching accelerator devices found for selector value %q", visibleDevices)
	}

	paths := make([]string, 0, len(devs)+len(r.view.Profile().Device.ControlDeviceGlobs))
	for _, dev := range devs {
		paths = append(paths, dev.Path)
	}
	paths = append(paths, r.controlDevicePaths()...)

	injected := 0
	for _, path := range uniqueStrings(paths) {
		added, err := injectLinuxDevicePath(spec, path)
		if err != nil {
			return fmt.Errorf("injecting OCI device %s: %w", path, err)
		}
		if added {
			injected++
		}
	}
	if injected > 0 {
		r.log.WithField("count", injected).Info("injected OCI linux devices from profile")
	}
	return nil
}

func (r *Runtime) visibleDevices(spec *specs.Spec) string {
	if spec.Process == nil {
		return ""
	}
	for _, selectorEnv := range r.view.SelectorEnvVars() {
		prefix := selectorEnv + "="
		for _, env := range spec.Process.Env {
			if strings.HasPrefix(env, prefix) {
				return strings.TrimPrefix(env, prefix)
			}
		}
	}
	return ""
}

func (r *Runtime) controlDevicePaths() []string {
	var paths []string
	for _, pattern := range r.view.Profile().Device.ControlDeviceGlobs {
		matches, err := filepath.Glob(pattern)
		if err != nil {
			r.log.WithError(err).WithField("pattern", pattern).Warn("invalid control device glob, skipping")
			continue
		}
		paths = append(paths, matches...)
	}
	return paths
}

func injectLinuxDevicePath(spec *specs.Spec, path string) (bool, error) {
	info, err := os.Stat(path)
	if err != nil {
		return false, err
	}
	if info.Mode()&os.ModeDevice == 0 {
		return false, fmt.Errorf("not a device node")
	}

	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return false, fmt.Errorf("unexpected stat type %T", info.Sys())
	}

	deviceType := "b"
	if info.Mode()&os.ModeCharDevice != 0 {
		deviceType = "c"
	}
	major := int64(unix.Major(uint64(stat.Rdev)))
	minor := int64(unix.Minor(uint64(stat.Rdev)))
	fileMode := info.Mode().Perm()
	uid := stat.Uid
	gid := stat.Gid

	if spec.Linux == nil {
		spec.Linux = &specs.Linux{}
	}
	for _, existing := range spec.Linux.Devices {
		if existing.Path == path {
			ensureDeviceCgroup(spec, deviceType, major, minor)
			return false, nil
		}
	}

	spec.Linux.Devices = append(spec.Linux.Devices, specs.LinuxDevice{
		Path:     path,
		Type:     deviceType,
		Major:    major,
		Minor:    minor,
		FileMode: &fileMode,
		UID:      &uid,
		GID:      &gid,
	})
	ensureDeviceCgroup(spec, deviceType, major, minor)
	return true, nil
}

func ensureDeviceCgroup(spec *specs.Spec, deviceType string, major, minor int64) {
	if spec.Linux.Resources == nil {
		spec.Linux.Resources = &specs.LinuxResources{}
	}
	for _, existing := range spec.Linux.Resources.Devices {
		if existing.Allow &&
			existing.Type == deviceType &&
			existing.Major != nil && *existing.Major == major &&
			existing.Minor != nil && *existing.Minor == minor &&
			existing.Access == "rwm" {
			return
		}
	}
	spec.Linux.Resources.Devices = append(spec.Linux.Resources.Devices, specs.LinuxDeviceCgroup{
		Allow:  true,
		Type:   deviceType,
		Major:  &major,
		Minor:  &minor,
		Access: "rwm",
	})
}

func uniqueStrings(in []string) []string {
	seen := make(map[string]bool, len(in))
	out := make([]string, 0, len(in))
	for _, value := range in {
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	return out
}

// containerRequestsGPU returns true if the container spec contains any
// configured device selector environment variable.
func (r *Runtime) containerRequestsGPU(spec *specs.Spec) bool {
	if spec.Process == nil {
		return false
	}

	for _, selectorEnv := range r.view.SelectorEnvVars() {
		envKey := selectorEnv + "="
		for _, env := range spec.Process.Env {
			if len(env) >= len(envKey) && env[:len(envKey)] == envKey {
				return true
			}
		}
	}
	return false
}

// delegate exec-replaces the current process with the underlying OCI runtime.
func (r *Runtime) delegate(args []string) error {
	underlying, err := exec.LookPath(r.view.UnderlyingRuntime())
	if err != nil {
		return fmt.Errorf("looking up underlying runtime %q: %w", r.view.UnderlyingRuntime(), err)
	}

	r.log.WithFields(logrus.Fields{
		"runtime": underlying,
		"args":    args[1:],
	}).Debug("delegating to underlying runtime")

	cmd := exec.Command(underlying, args[1:]...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			os.Exit(exitErr.ExitCode())
		}
		return err
	}
	return nil
}

// parseArgs extracts the sub-command and --bundle flag value from argv
// (without argv[0]).
func parseArgs(argv []string) (cmd, bundle string) {
	skipNext := false
	for i, arg := range argv {
		if skipNext {
			skipNext = false
			continue
		}
		if arg == "--root" || arg == "-root" || arg == "--log" || arg == "--log-format" {
			skipNext = true
			continue
		}
		if !strings.HasPrefix(arg, "-") && cmd == "" {
			cmd = arg
			continue
		}
		if arg == "--bundle" || arg == "-bundle" {
			if i+1 < len(argv) {
				bundle = argv[i+1]
			}
		} else if arg == "-b" {
			if i+1 < len(argv) {
				bundle = argv[i+1]
			}
		} else if strings.HasPrefix(arg, "--bundle=") {
			bundle = strings.TrimPrefix(arg, "--bundle=")
		} else if strings.HasPrefix(arg, "-bundle=") {
			bundle = strings.TrimPrefix(arg, "-bundle=")
		} else if strings.HasPrefix(arg, "-b=") {
			bundle = strings.TrimPrefix(arg, "-b=")
		}
	}
	return
}
