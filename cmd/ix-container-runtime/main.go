// ix-container-runtime is a thin OCI runtime shim that wraps runc (or another
// configured OCI runtime). For "create" operations it injects ix-container-hook
// as a prestart hook into the container's OCI spec so that Iluvatar GPU devices
// and driver libraries are automatically available inside the container.
//
// Usage (containerd / CRI-O config):
//
//	[plugins."io.containerd.grpc.v1.cri".containerd.runtimes.ix]
//	  runtime_type = "io.containerd.runc.v2"
//	  [plugins."io.containerd.grpc.v1.cri".containerd.runtimes.ix.options]
//	    BinaryName = "/usr/bin/ix-container-runtime"
package main

import (
	"fmt"
	"os"

	"github.com/ix-toolkit/ix-toolkit/internal/runtime"
	"github.com/ix-toolkit/ix-toolkit/pkg/config"
	"github.com/ix-toolkit/ix-toolkit/pkg/logger"
)

func main() {
	// Unlike a typical CLI app, ix-container-runtime must preserve the exact
	// argv that runc expects, so we do NOT use a flag parser here. The only
	// environment variable we honour is IX_CONFIG_FILE.
	configPath := os.Getenv("IX_CONFIG_FILE")
	cfg, err := config.Load(configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ix-container-runtime: loading config: %v\n", err)
		os.Exit(1)
	}

	log := logger.New(cfg.LogLevel, cfg.LogFile)
	rt := runtime.New(cfg, log)

	if err := rt.Exec(os.Args); err != nil {
		fmt.Fprintf(os.Stderr, "ix-container-runtime: %v\n", err)
		os.Exit(1)
	}
}
