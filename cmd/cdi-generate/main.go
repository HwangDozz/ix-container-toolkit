package main

import (
	"fmt"
	"os"

	"github.com/sirupsen/logrus"

	"github.com/accelerator-toolkit/accelerator-toolkit/pkg/cdi"
	"github.com/accelerator-toolkit/accelerator-toolkit/pkg/device"
	"github.com/accelerator-toolkit/accelerator-toolkit/pkg/profile"
)

func main() {
	log := logrus.New()
	log.SetLevel(logrus.InfoLevel)

	profilePath := "profiles/ascend-910b.yaml"
	if len(os.Args) > 2 && os.Args[1] == "--profile" {
		profilePath = os.Args[2]
	} else if len(os.Args) > 1 {
		profilePath = os.Args[1]
	}

	p, err := profile.Load(profilePath)
	if err != nil {
		log.Fatalf("loading profile: %v", err)
	}

	visibleDevices := "all"
	for _, f := range p.Device.SelectorFormats {
		if f == "all" {
			visibleDevices = "all"
			break
		}
	}

	devs, err := device.DiscoverWithProfile(visibleDevices, p, log)
	if err != nil {
		log.Fatalf("discovering devices: %v", err)
	}
	if len(devs) == 0 {
		log.Fatal("no devices found")
	}
	log.Infof("Discovered %d devices", len(devs))

	gen := cdi.NewGenerator(p, devs, log)
	spec, err := gen.Generate()
	if err != nil {
		log.Fatalf("generating CDI spec: %v", err)
	}

	path, err := cdi.WriteSpec(spec, "/etc/cdi")
	if err != nil {
		log.Fatalf("writing CDI spec: %v", err)
	}

	fmt.Printf("CDI spec written to: %s\n", path)
	fmt.Printf("CDI version: %s\n", spec.CDIVersion)
	fmt.Printf("Kind: %s\n", spec.Kind)
	fmt.Printf("Devices: %d\n", len(spec.Devices))
}
