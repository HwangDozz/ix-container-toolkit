// ix-container-hook is an OCI prestart hook that injects Iluvatar GPU devices
// and driver libraries into containers that request them via the
// ILUVATAR_VISIBLE_DEVICES environment variable.
//
// It is called by the OCI runtime with the container state JSON on stdin:
//
//	ix-container-hook [--config /path/to/config.json]
package main

import (
	"fmt"
	"os"

	"github.com/urfave/cli/v2"

	"github.com/ix-toolkit/ix-toolkit/internal/hook"
	"github.com/ix-toolkit/ix-toolkit/pkg/config"
	"github.com/ix-toolkit/ix-toolkit/pkg/logger"
)

func main() {
	app := &cli.App{
		Name:  "ix-container-hook",
		Usage: "OCI prestart hook for Iluvatar GPU injection",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:    "config",
				Aliases: []string{"c"},
				Value:   config.DefaultConfigPath,
				Usage:   "path to ix-toolkit config file",
				EnvVars: []string{"IX_CONFIG_FILE"},
			},
		},
		Action: func(ctx *cli.Context) error {
			cfg, err := config.Load(ctx.String("config"))
			if err != nil {
				return fmt.Errorf("loading config: %w", err)
			}

			log := logger.New(cfg.LogLevel, cfg.LogFile)
			h := hook.New(cfg, log)
			return h.Run(os.Stdin)
		},
	}

	if err := app.Run(os.Args); err != nil {
		fmt.Fprintf(os.Stderr, "ix-container-hook: %v\n", err)
		os.Exit(1)
	}
}
