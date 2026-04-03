// accelerator-container-hook is an OCI prestart hook that injects accelerator devices
// and driver artifacts into containers that request them via the configured
// selector environment variable.
//
// It is called by the OCI runtime with the container state JSON on stdin:
//
//	accelerator-container-hook [--config /path/to/config.json]
package main

import (
	"fmt"
	"os"

	"github.com/urfave/cli/v2"

	"github.com/accelerator-toolkit/accelerator-toolkit/internal/hook"
	"github.com/accelerator-toolkit/accelerator-toolkit/pkg/logger"
	"github.com/accelerator-toolkit/accelerator-toolkit/pkg/runtimeview"
)

func main() {
	app := &cli.App{
		Name:  "accelerator-container-hook",
		Usage: "OCI prestart hook for accelerator injection",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:    "config",
				Aliases: []string{"c"},
				Value:   "/etc/accelerator-toolkit/config.json",
				Usage:   "path to accelerator-toolkit config file",
				EnvVars: []string{"ACCELERATOR_CONFIG_FILE"},
			},
			&cli.StringFlag{
				Name:    "profile",
				Aliases: []string{"p"},
				Usage:   "path to generic profile YAML",
				EnvVars: []string{"ACCELERATOR_PROFILE_FILE"},
			},
		},
		Action: func(ctx *cli.Context) error {
			view, err := runtimeview.Load(ctx.String("config"), ctx.String("profile"))
			if err != nil {
				return fmt.Errorf("loading runtime view: %w", err)
			}

			log := logger.New(view.Config().LogLevel, view.Config().LogFile)
			h := hook.New(view, log)
			return h.Run(os.Stdin)
		},
	}

	if err := app.Run(os.Args); err != nil {
		fmt.Fprintf(os.Stderr, "accelerator-container-hook: %v\n", err)
		os.Exit(1)
	}
}
