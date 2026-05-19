package dra

import (
	"fmt"

	resourceapi "k8s.io/api/resource/v1"
	"k8s.io/apimachinery/pkg/types"

	"k8s.io/dynamic-resource-allocation/resourceslice"

	"github.com/accelerator-toolkit/accelerator-toolkit/pkg/device"
	"github.com/accelerator-toolkit/accelerator-toolkit/pkg/profile"
)

// DriverName is the DRA driver name used in ResourceSlice specs.
// It should be a DNS subdomain. Each vendor's resource name
// (e.g. iluvatar.com/gpu) is used as the CDI kind, while this
// driver name identifies the DRA driver itself.
const DriverName = "gpu.accelerator-toolkit.io"

// BuildDriverResources converts discovered devices into a DriverResources
// that can be published via the DRA ResourceSlice controller.
//
// poolName is typically the node name. The profile provides the resource
// name (used as CDI kind) and device metadata.
func BuildDriverResources(
	devs []device.Device,
	p *profile.Profile,
	poolName string,
) resourceslice.DriverResources {
	if len(devs) == 0 {
		// Publish an empty pool so the allocator knows the driver is
		// running but has no devices.
		return resourceslice.DriverResources{
			Pools: map[string]resourceslice.Pool{
				poolName: {Slices: []resourceslice.Slice{{}}},
			},
		}
	}

	k8sDevices := make([]resourceapi.Device, 0, len(devs))
	for _, dev := range devs {
		k8sDevices = append(k8sDevices, toK8sDevice(dev, p))
	}

	// Respect the 128-device-per-slice limit.
	var slices []resourceslice.Slice
	for i := 0; i < len(k8sDevices); i += resourceapi.ResourceSliceMaxDevices {
		end := i + resourceapi.ResourceSliceMaxDevices
		if end > len(k8sDevices) {
			end = len(k8sDevices)
		}
		slices = append(slices, resourceslice.Slice{
			Devices: k8sDevices[i:end],
		})
	}

	return resourceslice.DriverResources{
		Pools: map[string]resourceslice.Pool{
			poolName: {Slices: slices},
		},
	}
}

// toK8sDevice converts a device.Device to a K8s resourceapi.Device.
// The device name is the UUID (or index-based fallback), which also
// serves as the CDI device unique name.
func toK8sDevice(dev device.Device, p *profile.Profile) resourceapi.Device {
	name := deviceName(dev)

	attrs := map[resourceapi.QualifiedName]resourceapi.DeviceAttribute{
		"vendor": {StringValue: &p.Metadata.Vendor},
		"model":  {StringValue: &p.Metadata.ModelFamily},
	}
	if dev.UUID != "" {
		uuid := dev.UUID
		attrs["uuid"] = resourceapi.DeviceAttribute{StringValue: &uuid}
	}
	if dev.Path != "" {
		path := dev.Path
		attrs["path"] = resourceapi.DeviceAttribute{StringValue: &path}
	}

	return resourceapi.Device{
		Name:       name,
		Attributes: attrs,
	}
}

// deviceName returns a DNS-label-safe name for the device.
// Prefers UUID, falls back to "index-<N>".
func deviceName(dev device.Device) string {
	if dev.UUID != "" {
		return dev.UUID
	}
	return fmt.Sprintf("index-%d", dev.Index)
}

// CDIDeviceID returns the full CDI device ID for a DRA device,
// in the format "<kind>=<device-name>-<claimUID>".
// The claim UID is included to avoid CDI cache conflicts when
// the same physical device is unprepared and re-prepared by a
// different claim.
func CDIDeviceID(dev device.Device, p *profile.Profile, claimUID types.UID) string {
	kind := p.Kubernetes.ResourceNames[0]
	return fmt.Sprintf("%s=%s-%s", kind, deviceName(dev), string(claimUID))
}
