// Package runtime implements ix-container-runtime, an OCI runtime shim that
// transparently wraps an underlying OCI runtime (e.g. runc) and injects the
// ix-container-hook as a prestart hook for containers that request Iluvatar GPUs.
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
	"strings"

	"github.com/opencontainers/runtime-spec/specs-go"
	"github.com/sirupsen/logrus"

	"github.com/ix-toolkit/ix-toolkit/pkg/config"
)

// Runtime is the OCI runtime shim.
type Runtime struct {
	cfg *config.Config
	log *logrus.Logger
}

// New creates a Runtime shim.
func New(cfg *config.Config, log *logrus.Logger) *Runtime {
	return &Runtime{cfg: cfg, log: log}
}

// Exec is the entry point: it receives os.Args (the full argv including argv[0])
// and acts as a drop-in replacement for runc.
func (r *Runtime) Exec(args []string) error {
	if len(args) < 2 {
		return r.delegate(args)
	}

	// Find the "create" sub-command and the bundle path.
	cmd, bundlePath := parseArgs(args[1:])

	if cmd != "create" || bundlePath == "" {
		// For everything other than "create", pass through unchanged.
		return r.delegate(args)
	}

	r.log.WithField("bundle", bundlePath).Debug("intercepting container create")

	// Modify the OCI spec to inject our prestart hook.
	if err := r.injectHook(bundlePath); err != nil {
		// Non-fatal: log and proceed without the hook so we don't break
		// containers that don't need GPU access.
		r.log.WithError(err).Warn("failed to inject ix-container-hook, proceeding without GPU support")
	}

	return r.delegate(args)
}

// injectHook reads config.json from bundle, injects the ix-container-hook
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
		Path: r.cfg.HookPath,
		// No args needed; the hook reads everything from stdin (OCI state).
	}

	if spec.Hooks == nil {
		spec.Hooks = &specs.Hooks{}
	}
	// Prepend our hook so it runs before any user-defined prestart hooks.
	spec.Hooks.Prestart = append([]specs.Hook{hook}, spec.Hooks.Prestart...) //nolint:staticcheck

	modified, err := json.Marshal(&spec)
	if err != nil {
		return fmt.Errorf("marshalling modified spec: %w", err)
	}
	if err := os.WriteFile(specPath, modified, 0644); err != nil {
		return fmt.Errorf("writing %s: %w", specPath, err)
	}

	r.log.WithField("hookPath", r.cfg.HookPath).Info("injected ix-container-hook as prestart hook")
	return nil
}

// containerRequestsGPU returns true if the container spec contains any
// Iluvatar GPU resources in its Linux devices or environment variables.
func (r *Runtime) containerRequestsGPU(spec *specs.Spec) bool {
	if spec.Process == nil {
		return false
	}
	envKey := r.cfg.Hook.DeviceListEnvvar + "="
	for _, env := range spec.Process.Env {
		if len(env) >= len(envKey) && env[:len(envKey)] == envKey {
			return true
		}
	}
	return false
}

// delegate exec-replaces the current process with the underlying OCI runtime.
func (r *Runtime) delegate(args []string) error {
	underlying, err := exec.LookPath(r.cfg.UnderlyingRuntime)
	if err != nil {
		return fmt.Errorf("looking up underlying runtime %q: %w", r.cfg.UnderlyingRuntime, err)
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
	for i, arg := range argv {
		if !strings.HasPrefix(arg, "-") && cmd == "" {
			cmd = arg
			continue
		}
		if arg == "--bundle" || arg == "-bundle" {
			if i+1 < len(argv) {
				bundle = argv[i+1]
			}
		} else if strings.HasPrefix(arg, "--bundle=") {
			bundle = strings.TrimPrefix(arg, "--bundle=")
		} else if strings.HasPrefix(arg, "-bundle=") {
			bundle = strings.TrimPrefix(arg, "-bundle=")
		}
	}
	return
}
