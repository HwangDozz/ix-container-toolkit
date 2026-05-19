package dra

import (
	"testing"

	resourceapi "k8s.io/api/resource/v1"
	"k8s.io/apimachinery/pkg/types"

	"github.com/accelerator-toolkit/accelerator-toolkit/pkg/device"
	"github.com/accelerator-toolkit/accelerator-toolkit/pkg/profile"
)

func testProfile() *profile.Profile {
	return &profile.Profile{
		Metadata: profile.Metadata{
			Vendor:      "iluvatar",
			ModelFamily: "BI-V150",
		},
		Kubernetes: profile.Kubernetes{
			ResourceNames: []string{"iluvatar.com/gpu"},
		},
	}
}

func TestBuildDriverResources_EmptyDevices(t *testing.T) {
	p := testProfile()
	res := BuildDriverResources(nil, p, "node1")

	pool, ok := res.Pools["node1"]
	if !ok {
		t.Fatal("expected pool 'node1'")
	}
	if len(pool.Slices) != 1 {
		t.Fatalf("expected 1 slice, got %d", len(pool.Slices))
	}
	// Empty pool should have one slice with no devices.
	if len(pool.Slices[0].Devices) != 0 {
		t.Fatalf("expected 0 devices, got %d", len(pool.Slices[0].Devices))
	}
}

func TestBuildDriverResources_SingleDevice(t *testing.T) {
	p := testProfile()
	devs := []device.Device{
		{Path: "/dev/iluvatar0", Index: 0, UUID: "GPU-aaaa"},
	}
	res := BuildDriverResources(devs, p, "node1")

	pool := res.Pools["node1"]
	if len(pool.Slices) != 1 {
		t.Fatalf("expected 1 slice, got %d", len(pool.Slices))
	}
	if len(pool.Slices[0].Devices) != 1 {
		t.Fatalf("expected 1 device, got %d", len(pool.Slices[0].Devices))
	}

	dev := pool.Slices[0].Devices[0]
	if dev.Name != "GPU-aaaa" {
		t.Errorf("expected name 'GPU-aaaa', got '%s'", dev.Name)
	}
	if dev.Attributes["vendor"].StringValue == nil || *dev.Attributes["vendor"].StringValue != "iluvatar" {
		t.Errorf("expected vendor 'iluvatar'")
	}
	if dev.Attributes["uuid"].StringValue == nil || *dev.Attributes["uuid"].StringValue != "GPU-aaaa" {
		t.Errorf("expected uuid 'GPU-aaaa'")
	}
}

func TestBuildDriverResources_MultipleDevices(t *testing.T) {
	p := testProfile()
	devs := []device.Device{
		{Path: "/dev/iluvatar0", Index: 0, UUID: "GPU-aaaa"},
		{Path: "/dev/iluvatar1", Index: 1, UUID: "GPU-bbbb"},
		{Path: "/dev/iluvatar2", Index: 2, UUID: "GPU-cccc"},
	}
	res := BuildDriverResources(devs, p, "node1")

	pool := res.Pools["node1"]
	if len(pool.Slices) != 1 {
		t.Fatalf("expected 1 slice, got %d", len(pool.Slices))
	}
	if len(pool.Slices[0].Devices) != 3 {
		t.Fatalf("expected 3 devices, got %d", len(pool.Slices[0].Devices))
	}

	// Verify order is preserved.
	if pool.Slices[0].Devices[0].Name != "GPU-aaaa" {
		t.Errorf("first device should be GPU-aaaa")
	}
	if pool.Slices[0].Devices[2].Name != "GPU-cccc" {
		t.Errorf("last device should be GPU-cccc")
	}
}

func TestBuildDriverResources_NoUUID(t *testing.T) {
	p := testProfile()
	devs := []device.Device{
		{Path: "/dev/iluvatar0", Index: 0},
	}
	res := BuildDriverResources(devs, p, "node1")

	dev := res.Pools["node1"].Slices[0].Devices[0]
	if dev.Name != "index-0" {
		t.Errorf("expected name 'index-0', got '%s'", dev.Name)
	}
	// UUID attribute should not be set.
	if _, ok := dev.Attributes["uuid"]; ok {
		t.Errorf("expected no uuid attribute when UUID is empty")
	}
}

func TestBuildDriverResources_SliceOverflow(t *testing.T) {
	p := testProfile()
	// Create more than ResourceSliceMaxDevices (128) devices.
	devs := make([]device.Device, 130)
	for i := range devs {
		devs[i] = device.Device{Path: "/dev/iluvatar0", Index: i, UUID: "GPU-" + string(rune('a'+i%26))}
	}
	res := BuildDriverResources(devs, p, "node1")

	pool := res.Pools["node1"]
	if len(pool.Slices) != 2 {
		t.Fatalf("expected 2 slices for 130 devices, got %d", len(pool.Slices))
	}
	if len(pool.Slices[0].Devices) != resourceapi.ResourceSliceMaxDevices {
		t.Errorf("first slice should have %d devices", resourceapi.ResourceSliceMaxDevices)
	}
	if len(pool.Slices[1].Devices) != 2 {
		t.Errorf("second slice should have 2 devices")
	}
}

func TestDeviceName_UUID(t *testing.T) {
	dev := device.Device{UUID: "GPU-aaaa", Index: 0}
	if name := deviceName(dev); name != "GPU-aaaa" {
		t.Errorf("expected 'GPU-aaaa', got '%s'", name)
	}
}

func TestDeviceName_IndexFallback(t *testing.T) {
	dev := device.Device{Index: 5}
	if name := deviceName(dev); name != "index-5" {
		t.Errorf("expected 'index-5', got '%s'", name)
	}
}

func TestCDIDeviceID(t *testing.T) {
	p := testProfile()
	dev := device.Device{UUID: "GPU-aaaa", Index: 0}
	uid := types.UID("test-uid")
	id := CDIDeviceID(dev, p, uid)
	if id != "iluvatar.com/gpu=GPU-aaaa-test-uid" {
		t.Errorf("expected 'iluvatar.com/gpu=GPU-aaaa-test-uid', got '%s'", id)
	}
}

func TestCDIDeviceID_NoUUID(t *testing.T) {
	p := testProfile()
	dev := device.Device{Index: 3}
	uid := types.UID("uid-123")
	id := CDIDeviceID(dev, p, uid)
	if id != "iluvatar.com/gpu=index-3-uid-123" {
		t.Errorf("expected 'iluvatar.com/gpu=index-3-uid-123', got '%s'", id)
	}
}
