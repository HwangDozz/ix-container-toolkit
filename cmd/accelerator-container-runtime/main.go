// accelerator-container-runtime is a thin OCI runtime shim that wraps runc (or another
// configured OCI runtime). For "create" operations it injects accelerator-container-hook
// as a prestart hook into the container's OCI spec so that requested
// accelerator devices and driver artifacts are automatically available inside
// the container.
//
// Usage (containerd / CRI-O config):
//
//	[plugins."io.containerd.grpc.v1.cri".containerd.runtimes.accelerator]
//	  runtime_type = "io.containerd.runc.v2"
//	  [plugins."io.containerd.grpc.v1.cri".containerd.runtimes.accelerator.options]
//	    BinaryName = "/usr/bin/accelerator-container-runtime"
package main

import (
	"fmt"
	"os"

	"github.com/accelerator-toolkit/accelerator-toolkit/internal/runtime"
	"github.com/accelerator-toolkit/accelerator-toolkit/pkg/logger"
	"github.com/accelerator-toolkit/accelerator-toolkit/pkg/runtimeview"
)

func main() {
	// Unlike a typical CLI app, accelerator-container-runtime must preserve the exact
	// argv that runc expects, so we do NOT use a flag parser here. The only
	// environment variables we honour are ACCELERATOR_CONFIG_FILE and ACCELERATOR_PROFILE_FILE.
	configPath := os.Getenv("ACCELERATOR_CONFIG_FILE")
	view, err := runtimeview.Load(configPath, os.Getenv("ACCELERATOR_PROFILE_FILE"))
	if err != nil {
		fmt.Fprintf(os.Stderr, "accelerator-container-runtime: loading runtime view: %v\n", err)
		os.Exit(1)
	}

	log := logger.New(view.Config().LogLevel, view.Config().LogFile)
	rt := runtime.New(view, log)

	if err := rt.Exec(os.Args); err != nil {
		fmt.Fprintf(os.Stderr, "accelerator-container-runtime: %v\n", err)
		os.Exit(1)
	}
}
