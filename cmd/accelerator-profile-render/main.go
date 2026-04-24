package main

import (
	"fmt"
	"os"

	"github.com/urfave/cli/v2"

	"github.com/accelerator-toolkit/accelerator-toolkit/pkg/profile"
)

func main() {
	app := &cli.App{
		Name:  "accelerator-profile-render",
		Usage: "Render Kubernetes manifests from a generic accelerator profile",
		Commands: []*cli.Command{
			{
				Name:  "runtimeclass",
				Usage: "render a RuntimeClass manifest",
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:     "profile",
						Aliases:  []string{"p"},
						Usage:    "path to generic profile YAML",
						Required: true,
					},
				},
				Action: func(ctx *cli.Context) error {
					p, err := profile.Load(ctx.String("profile"))
					if err != nil {
						return fmt.Errorf("loading profile: %w", err)
					}

					data, err := p.RenderRuntimeClassYAML()
					if err != nil {
						return fmt.Errorf("rendering runtimeclass: %w", err)
					}

					if _, err := os.Stdout.Write(data); err != nil {
						return fmt.Errorf("writing output: %w", err)
					}
					return nil
				},
			},
			{
				Name:  "daemonset",
				Usage: "render the installer DaemonSet manifest",
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:     "profile",
						Aliases:  []string{"p"},
						Usage:    "path to generic profile YAML",
						Required: true,
					},
					&cli.StringFlag{
						Name:     "image",
						Aliases:  []string{"i"},
						Usage:    "installer image to deploy",
						Required: true,
					},
				},
				Action: func(ctx *cli.Context) error {
					profilePath := ctx.String("profile")
					p, err := profile.Load(profilePath)
					if err != nil {
						return fmt.Errorf("loading profile: %w", err)
					}

					data, err := p.RenderDaemonSetYAML(ctx.String("image"), profilePath)
					if err != nil {
						return fmt.Errorf("rendering daemonset: %w", err)
					}

					if _, err := os.Stdout.Write(data); err != nil {
						return fmt.Errorf("writing output: %w", err)
					}
					return nil
				},
			},
			{
				Name:  "bundle",
				Usage: "render the full deploy bundle: RBAC + RuntimeClass + DaemonSet",
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:     "profile",
						Aliases:  []string{"p"},
						Usage:    "path to generic profile YAML",
						Required: true,
					},
					&cli.StringFlag{
						Name:     "image",
						Aliases:  []string{"i"},
						Usage:    "installer image to deploy",
						Required: true,
					},
				},
				Action: func(ctx *cli.Context) error {
					profilePath := ctx.String("profile")
					p, err := profile.Load(profilePath)
					if err != nil {
						return fmt.Errorf("loading profile: %w", err)
					}

					data, err := p.RenderBundleYAML(ctx.String("image"), profilePath)
					if err != nil {
						return fmt.Errorf("rendering bundle: %w", err)
					}

					if _, err := os.Stdout.Write(data); err != nil {
						return fmt.Errorf("writing output: %w", err)
					}
					return nil
				},
			},
			{
				Name:  "cdi",
				Usage: "render a CDI spec prototype from a generic accelerator profile",
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:     "profile",
						Aliases:  []string{"p"},
						Usage:    "path to generic profile YAML",
						Required: true,
					},
					&cli.StringFlag{
						Name:  "device-name",
						Usage: "CDI device name to render",
						Value: "all",
					},
				},
				Action: func(ctx *cli.Context) error {
					p, err := profile.Load(ctx.String("profile"))
					if err != nil {
						return fmt.Errorf("loading profile: %w", err)
					}

					data, err := p.RenderCDISpecYAML(ctx.String("device-name"))
					if err != nil {
						return fmt.Errorf("rendering cdi spec: %w", err)
					}

					if _, err := os.Stdout.Write(data); err != nil {
						return fmt.Errorf("writing output: %w", err)
					}
					return nil
				},
			},
		},
	}

	if err := app.Run(os.Args); err != nil {
		fmt.Fprintf(os.Stderr, "accelerator-profile-render: %v\n", err)
		os.Exit(1)
	}
}
