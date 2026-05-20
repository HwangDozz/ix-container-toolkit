package dra

import (
	"context"
	"errors"
	"fmt"
	"strings"
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
	log      *logrus.Logger
	profile  *profile.Profile
	devices  []device.Device
	poolName string // typically the node name, matches ResourceSlice pool key

	// cdiDir is where CDI spec files are written.
	cdiDir string

	// mu protects preparedClaims.
	// Note: the DRA helper serializes gRPC calls by default, so this mutex
	// is primarily a defensive measure for in-process safety.
	mu sync.Mutex
	// preparedClaims tracks which claims have been prepared,
	// keyed by claim UID. Value stores both CDI device IDs and
	// the allocated device info for each prepared device.
	preparedClaims map[types.UID][]preparedDevice
}

// preparedDevice pairs a CDI device ID with the allocated device info
// so we can reconstruct PrepareResult without reverse-parsing the CDI ID.
type preparedDevice struct {
	cdiID       string
	allocDevice allocatedDevice
}

// PluginConfig holds configuration for creating a Plugin.
type PluginConfig struct {
	Profile  *profile.Profile
	Devices  []device.Device
	CDIDir   string
	PoolName string // typically the node name
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
		poolName:       cfg.PoolName,
		cdiDir:         cdiDir,
		preparedClaims: make(map[types.UID][]preparedDevice),
	}
}

// PrepareResourceClaims implements kubeletplugin.DRAPlugin.
// It is idempotent: if a claim is already prepared, it returns the
// existing CDI device IDs without regenerating the CDI spec.
func (p *Plugin) PrepareResourceClaims(ctx context.Context, claims []*resourceapi.ResourceClaim) (map[types.UID]kubeletplugin.PrepareResult, error) {
	result := make(map[types.UID]kubeletplugin.PrepareResult, len(claims))

	for _, claim := range claims {
		// Idempotency: if already prepared, return existing results.
		p.mu.Lock()
		if existing, ok := p.preparedClaims[claim.UID]; ok {
			p.mu.Unlock()
			devices := make([]kubeletplugin.Device, 0, len(existing))
			for _, pd := range existing {
				devices = append(devices, kubeletplugin.Device{
					Requests:     []string{pd.allocDevice.requestName},
					PoolName:     p.poolName,
					DeviceName:   deviceName(pd.allocDevice.dev),
					CDIDeviceIDs: []string{pd.cdiID},
				})
			}
			result[claim.UID] = kubeletplugin.PrepareResult{Devices: devices}
			p.log.WithField("claim", claim.Name).Debug("Claim already prepared, returning existing result")
			continue
		}
		p.mu.Unlock()

		// Prepare the claim (generates CDI spec, returns CDI device IDs).
		cdiIDs, allocatedDevs, err := p.prepareClaim(claim)
		if err != nil {
			p.log.WithError(err).WithField("claim", claim.Name).Error("Failed to prepare claim")
			result[claim.UID] = kubeletplugin.PrepareResult{Err: err}
			continue
		}

		prepared := make([]preparedDevice, len(cdiIDs))
		for i, cdiID := range cdiIDs {
			prepared[i] = preparedDevice{cdiID: cdiID, allocDevice: allocatedDevs[i]}
		}
		p.mu.Lock()
		p.preparedClaims[claim.UID] = prepared
		p.mu.Unlock()

		devices := make([]kubeletplugin.Device, 0, len(cdiIDs))
		for i, cdiID := range cdiIDs {
			devices = append(devices, kubeletplugin.Device{
				Requests:     []string{allocatedDevs[i].requestName},
				PoolName:     p.poolName,
				DeviceName:   deviceName(allocatedDevs[i].dev),
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
// It is idempotent: unpreparing an unknown claim UID is a no-op.
func (p *Plugin) UnprepareResourceClaims(ctx context.Context, claims []kubeletplugin.NamespacedObject) (map[types.UID]error, error) {
	result := make(map[types.UID]error, len(claims))

	for _, claim := range claims {
		p.mu.Lock()
		prepared, exists := p.preparedClaims[claim.UID]
		if exists {
			delete(p.preparedClaims, claim.UID)
		}
		p.mu.Unlock()

		if !exists {
			if err := p.cleanupCDIEntriesForClaimUID(claim.UID); err != nil {
				p.log.WithError(err).WithField("claim", claim.Name).Warn("Failed to clean CDI entries for unknown prepared claim")
			}
			result[claim.UID] = nil
			continue
		}

		cdiIDs := make([]string, len(prepared))
		for i, pd := range prepared {
			cdiIDs[i] = pd.cdiID
		}
		if err := p.cleanupCDIEntries(cdiIDs); err != nil {
			p.log.WithError(err).WithField("claim", claim.Name).Warn("Failed to clean CDI entries")
		}

		result[claim.UID] = nil
		p.log.WithField("claim", claim.Name).Info("Unprepared resource claim")
	}

	return result, nil
}

// cleanupCDIEntries removes device entries from the CDI spec file.
// If no devices remain, the spec file is deleted.
func (p *Plugin) cleanupCDIEntries(cdiIDs []string) error {
	if len(cdiIDs) == 0 {
		return nil
	}

	kind := p.profile.Kubernetes.ResourceNames[0]
	existing, err := cdi.ReadSpec(p.cdiDir, kind)
	if err != nil {
		return fmt.Errorf("read CDI spec: %w", err)
	}
	if existing == nil {
		return nil
	}

	deviceNames := extractCDIDeviceNames(cdiIDs)
	remaining := cdi.RemoveDevices(existing, deviceNames)
	if remaining == nil {
		return cdi.DeleteSpecFile(p.cdiDir, kind)
	}

	_, err = cdi.WriteSpec(remaining, p.cdiDir)
	return err
}

func (p *Plugin) cleanupCDIEntriesForClaimUID(claimUID types.UID) error {
	kind := p.profile.Kubernetes.ResourceNames[0]
	existing, err := cdi.ReadSpec(p.cdiDir, kind)
	if err != nil {
		return fmt.Errorf("read CDI spec: %w", err)
	}
	if existing == nil {
		return nil
	}

	suffix := "-" + string(claimUID)
	var deviceNames []string
	for _, dev := range existing.Devices {
		if strings.HasSuffix(dev.Name, suffix) {
			deviceNames = append(deviceNames, dev.Name)
		}
	}
	if len(deviceNames) == 0 {
		return nil
	}

	remaining := cdi.RemoveDevices(existing, deviceNames)
	if remaining == nil {
		return cdi.DeleteSpecFile(p.cdiDir, kind)
	}
	_, err = cdi.WriteSpec(remaining, p.cdiDir)
	return err
}

// extractCDIDeviceNames parses CDI device IDs ("kind=name") and returns the name parts.
func extractCDIDeviceNames(cdiIDs []string) []string {
	names := make([]string, 0, len(cdiIDs))
	for _, id := range cdiIDs {
		if idx := strings.LastIndex(id, "="); idx >= 0 {
			names = append(names, id[idx+1:])
		}
	}
	return names
}

// HandleError implements kubeletplugin.DRAPlugin.
func (p *Plugin) HandleError(ctx context.Context, err error, msg string) {
	if errors.Is(err, kubeletplugin.ErrRecoverable) {
		p.log.WithError(err).Warn(msg)
		return
	}
	p.log.WithError(err).Error(msg)
}

// allocatedDevice pairs a discovered device with the request name from the claim.
type allocatedDevice struct {
	dev         device.Device
	requestName string
}

// prepareClaim generates CDI spec for a single ResourceClaim.
// It merges new device entries into the existing CDI spec file
// so that previously prepared claims are not lost.
func (p *Plugin) prepareClaim(claim *resourceapi.ResourceClaim) ([]string, []allocatedDevice, error) {
	if claim.Status.Allocation == nil {
		return nil, nil, fmt.Errorf("claim %s is not allocated", claim.Name)
	}

	// Determine which devices were allocated to this claim.
	allocatedDevs := p.findAllocatedDevices(claim)
	if len(allocatedDevs) == 0 {
		return nil, nil, fmt.Errorf("no matching devices found for claim %s", claim.Name)
	}

	// Verify no device is already in use by another claim (defensive check).
	if err := p.checkDeviceConflicts(claim.UID, allocatedDevs); err != nil {
		return nil, nil, err
	}

	// Collect the physical devices for CDI generation.
	physDevices := make([]device.Device, 0, len(allocatedDevs))
	for _, ad := range allocatedDevs {
		physDevices = append(physDevices, ad.dev)
	}

	// Generate CDI device entries for the allocated devices.
	gen := cdi.NewGenerator(p.profile, physDevices, p.log)
	newSpec, err := gen.Generate()
	if err != nil {
		return nil, nil, fmt.Errorf("generate CDI spec: %w", err)
	}

	// Overwrite CDI device names to include claim UID for cache uniqueness.
	for i := range newSpec.Devices {
		newSpec.Devices[i].Name = newSpec.Devices[i].Name + "-" + string(claim.UID)
	}

	// Read existing CDI spec and merge to avoid overwriting other claims' devices.
	kind := p.profile.Kubernetes.ResourceNames[0]
	existing, err := cdi.ReadSpec(p.cdiDir, kind)
	if err != nil {
		return nil, nil, fmt.Errorf("read existing CDI spec: %w", err)
	}
	merged := cdi.MergeDevices(existing, newSpec.Devices, kind)

	// Write merged CDI spec to disk.
	path, err := cdi.WriteSpec(merged, p.cdiDir)
	if err != nil {
		return nil, nil, fmt.Errorf("write CDI spec: %w", err)
	}
	p.log.WithField("path", path).Debug("Wrote CDI spec")

	// Build CDI device IDs.
	cdiIDs := make([]string, 0, len(allocatedDevs))
	for _, ad := range allocatedDevs {
		cdiIDs = append(cdiIDs, CDIDeviceID(ad.dev, p.profile, claim.UID))
	}

	return cdiIDs, allocatedDevs, nil
}

// checkDeviceConflicts verifies that none of the devices are already
// allocated to a different claim. This is a defensive check — Kubernetes
// normally prevents this, but the DRA driver is the last line of defense.
func (p *Plugin) checkDeviceConflicts(claimUID types.UID, allocatedDevs []allocatedDevice) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	// Build a set of physical device names being allocated.
	allocNames := make(map[string]bool, len(allocatedDevs))
	for _, ad := range allocatedDevs {
		allocNames[deviceName(ad.dev)] = true
	}

	for uid, prepared := range p.preparedClaims {
		if uid == claimUID {
			continue
		}
		for _, pd := range prepared {
			name := deviceName(pd.allocDevice.dev)
			if allocNames[name] {
				return fmt.Errorf("device %s already in use by claim %s", name, uid)
			}
		}
	}
	return nil
}

// findAllocatedDevices matches allocated device names from the claim
// to the discovered devices on this node, returning both the device
// and the request name from the allocation result.
func (p *Plugin) findAllocatedDevices(claim *resourceapi.ResourceClaim) []allocatedDevice {
	if claim.Status.Allocation == nil {
		return nil
	}

	// Collect allocated device names and their request names.
	type allocEntry struct {
		deviceName  string
		requestName string
	}
	allocated := make(map[string]allocEntry)
	for _, result := range claim.Status.Allocation.Devices.Results {
		if result.Driver != DriverName {
			continue
		}
		allocated[result.Device] = allocEntry{
			deviceName:  result.Device,
			requestName: result.Request,
		}
	}

	if len(allocated) == 0 {
		return nil
	}

	// Match against discovered devices.
	var matched []allocatedDevice
	for _, dev := range p.devices {
		name := deviceName(dev)
		if entry, ok := allocated[name]; ok {
			matched = append(matched, allocatedDevice{
				dev:         dev,
				requestName: entry.requestName,
			})
		}
	}

	return matched
}

// BuildPublishableResources returns the DriverResources for publishing
// via the DRA helper's PublishResources method.
func (p *Plugin) BuildPublishableResources(nodeName string) resourceslice.DriverResources {
	return BuildDriverResources(p.devices, p.profile, nodeName)
}
