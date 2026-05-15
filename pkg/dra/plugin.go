package dra

import (
	"context"
	"fmt"
	"sync"

	"github.com/sirupsen/logrus"
	resourceapi "k8s.io/api/resource/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/dynamic-resource-allocation/kubeletplugin"
	"k8s.io/dynamic-resource-allocation/resourceslice"

	"github.com/accelerator-toolkit/accelerator-toolkit/pkg/cdi"
	"github.com/accelerator-toolkit/accelerator-toolkit/pkg/device"
	"github.com/accelerator-toolkit/accelerator-toolkit/pkg/profile"
)

// Plugin implements the kubeletplugin.DRAPlugin interface.
// It handles PrepareResourceClaims and UnprepareResourceClaims
// by generating CDI specs on-the-fly.
type Plugin struct {
	log     *logrus.Logger
	profile *profile.Profile
	devices []device.Device

	// cdiDir is where CDI spec files are written.
	cdiDir string

	// mu protects preparedClaims.
	mu sync.Mutex
	// preparedClaims tracks which claims have been prepared,
	// keyed by claim UID. Value is the list of CDI device IDs
	// that were generated for the claim.
	preparedClaims map[types.UID][]string
}

// PluginConfig holds configuration for creating a Plugin.
type PluginConfig struct {
	Profile *profile.Profile
	Devices []device.Device
	CDIDir  string
}

// NewPlugin creates a new DRA plugin.
func NewPlugin(cfg PluginConfig, log *logrus.Logger) *Plugin {
	cdiDir := cfg.CDIDir
	if cdiDir == "" {
		cdiDir = "/etc/cdi"
	}
	return &Plugin{
		log:            log,
		profile:        cfg.Profile,
		devices:        cfg.Devices,
		cdiDir:         cdiDir,
		preparedClaims: make(map[types.UID][]string),
	}
}

// PrepareResourceClaims implements kubeletplugin.DRAPlugin.
// For each allocated claim, it generates a CDI spec and returns
// CDI device IDs.
func (p *Plugin) PrepareResourceClaims(ctx context.Context, claims []*resourceapi.ResourceClaim) (map[types.UID]kubeletplugin.PrepareResult, error) {
	result := make(map[types.UID]kubeletplugin.PrepareResult, len(claims))

	for _, claim := range claims {
		cdiIDs, err := p.prepareClaim(claim)
		if err != nil {
			p.log.WithError(err).WithField("claim", claim.Name).Error("Failed to prepare claim")
			result[claim.UID] = kubeletplugin.PrepareResult{Err: err}
			continue
		}

		p.mu.Lock()
		p.preparedClaims[claim.UID] = cdiIDs
		p.mu.Unlock()

		devices := make([]kubeletplugin.Device, 0, len(cdiIDs))
		for _, cdiID := range cdiIDs {
			devices = append(devices, kubeletplugin.Device{
				PoolName:     claim.Name, // Use claim name as pool context
				DeviceName:   cdiID,
				CDIDeviceIDs: []string{cdiID},
			})
		}

		result[claim.UID] = kubeletplugin.PrepareResult{Devices: devices}
		p.log.WithFields(logrus.Fields{
			"claim":      claim.Name,
			"cdiDevices": cdiIDs,
		}).Info("Prepared resource claim")
	}

	return result, nil
}

// UnprepareResourceClaims implements kubeletplugin.DRAPlugin.
func (p *Plugin) UnprepareResourceClaims(ctx context.Context, claims []kubeletplugin.NamespacedObject) (map[types.UID]error, error) {
	result := make(map[types.UID]error, len(claims))

	for _, claim := range claims {
		p.mu.Lock()
		delete(p.preparedClaims, claim.UID)
		p.mu.Unlock()

		result[claim.UID] = nil
		p.log.WithField("claim", claim.Name).Info("Unprepared resource claim")
	}

	return result, nil
}

// HandleError implements kubeletplugin.DRAPlugin.
func (p *Plugin) HandleError(ctx context.Context, err error, msg string) {
	if kubeletplugin.ErrRecoverable != nil && isRecoverable(err) {
		p.log.WithError(err).Warn(msg)
		return
	}
	p.log.WithError(err).Error(msg)
}

// prepareClaim generates CDI spec for a single ResourceClaim.
// It uses the existing CDI generator to produce a spec, writes it
// to the CDI directory, and returns the CDI device IDs.
func (p *Plugin) prepareClaim(claim *resourceapi.ResourceClaim) ([]string, error) {
	if claim.Status.Allocation == nil {
		return nil, fmt.Errorf("claim %s is not allocated", claim.Name)
	}

	// Determine which devices were allocated to this claim.
	allocatedDevices := p.findAllocatedDevices(claim)
	if len(allocatedDevices) == 0 {
		return nil, fmt.Errorf("no matching devices found for claim %s", claim.Name)
	}

	// Generate CDI spec for the allocated devices.
	gen := cdi.NewGenerator(p.profile, allocatedDevices, p.log)
	spec, err := gen.Generate()
	if err != nil {
		return nil, fmt.Errorf("generate CDI spec: %w", err)
	}

	// Write CDI spec to disk.
	path, err := cdi.WriteSpec(spec, p.cdiDir)
	if err != nil {
		return nil, fmt.Errorf("write CDI spec: %w", err)
	}
	p.log.WithField("path", path).Debug("Wrote CDI spec")

	// Build CDI device IDs.
	cdiIDs := make([]string, 0, len(allocatedDevices))
	for _, dev := range allocatedDevices {
		cdiIDs = append(cdiIDs, CDIDeviceID(dev, p.profile))
	}

	return cdiIDs, nil
}

// findAllocatedDevices matches allocated device names from the claim
// to the discovered devices on this node.
func (p *Plugin) findAllocatedDevices(claim *resourceapi.ResourceClaim) []device.Device {
	if claim.Status.Allocation == nil {
		return nil
	}

	// Collect all allocated device names for our driver.
	allocatedNames := make(map[string]bool)
	for _, result := range claim.Status.Allocation.Devices.Results {
		if result.Driver != DriverName {
			continue
		}
		allocatedNames[result.Device] = true
	}

	if len(allocatedNames) == 0 {
		return nil
	}

	// Match against discovered devices.
	var matched []device.Device
	for _, dev := range p.devices {
		name := deviceName(dev)
		if allocatedNames[name] {
			matched = append(matched, dev)
		}
	}

	return matched
}

// isRecoverable checks if an error is recoverable.
func isRecoverable(err error) bool {
	type recoverable interface {
		Is(error) bool
	}
	if r, ok := err.(recoverable); ok {
		return r.Is(kubeletplugin.ErrRecoverable)
	}
	return false
}

// BuildPublishableResources returns the DriverResources for publishing
// via the DRA helper's PublishResources method.
func (p *Plugin) BuildPublishableResources(nodeName string) resourceslice.DriverResources {
	return BuildDriverResources(p.devices, p.profile, nodeName)
}
