# DRA Driver Implementation Plan

## Decisions
- K8s 1.35+, DRA structured parameters (GA in resource/v1)
- DaemonSet deployment: separate from installer
- No topology awareness, FCFS allocation (API server built-in allocator)
- containerd 2.x with CDI support
- Multi-vendor: Iluvatar, Ascend, Metax (NVIDIA delegates to its own driver)

## Architecture

```
accelerator-dra-driver (DaemonSet, one pod per node)
├── kubeletplugin.DRAPlugin implementation:
│   ├── PrepareResourceClaims: generate CDI spec → write to /etc/cdi/ → return CDI IDs
│   ├── UnprepareResourceClaims: cleanup
│   └── HandleError: logging
│
└── kubeletplugin.Helper (from k8s.io/dynamic-resource-allocation):
    ├── PublishResources: create/update ResourceSlice objects
    ├── gRPC server: NodePrepareResources / NodeUnprepareResources
    └── Registration: register with kubelet plugin registry
```

## Data Flow

```
User Pod requests iluvatar.com/gpu: 1 (via ResourceClaim)
         ↓
API Server built-in DRA allocator matches ResourceClaim to ResourceSlice device
         ↓
ResourceClaim.status.allocation.devices.results = [{device: "GPU-aaaa", driver: "gpu.accelerator-toolkit.io"}]
         ↓
kubelet reads allocated claim, calls NodePrepareResources gRPC
         ↓
DRA driver: pkg/cdi.Generator generates CDI spec → writes to /etc/cdi/iluvatar.json
         ↓
kubelet tells containerd: CDI devices = ["iluvatar.com/gpu=GPU-aaaa"]
         ↓
containerd reads CDI spec, injects device nodes + driver mounts + ldconfig hook
```

## Implementation Status

### Step 1: Dependencies & Types ✅
- Added k8s.io/client-go, k8s.io/api, k8s.io/apimachinery, k8s.io/dynamic-resource-allocation (v0.36.1)
- Kept homegrown CDI types in pkg/cdi/ (working, no need to switch)

### Step 2: ResourceSlice Builder ✅
- `pkg/dra/resourceslice.go`: BuildDriverResources(), toK8sDevice(), deviceName(), CDIDeviceID()
- Converts pkg/device.Device → resourceapi.Device with vendor/model/uuid/path attributes
- Handles 128-device slice limit

### Step 3: DRA Plugin (NodePrepareResource) ✅
- `pkg/dra/plugin.go`: Plugin struct implements kubeletplugin.DRAPlugin interface
- PrepareResourceClaims: finds allocated devices, generates CDI spec, writes to disk, returns CDI IDs
- UnprepareResourceClaims: cleanup tracking
- Uses existing pkg/cdi.Generator (full reuse)

### Step 4: Main Binary ✅
- `cmd/accelerator-dra-driver/main.go`: wires profile → device discovery → kubeletplugin.Start → PublishResources
- Flags: --profile, --kubeconfig, --cdi-dir, --node-name, --resync-period
- In-cluster and kubeconfig modes

### Step 5: K8s Manifests ✅
- `deployments/dra-driver/rbac.yaml`: ServiceAccount + ClusterRole (ResourceSlice CRUD, ResourceClaim get/list/watch, Node get)
- `deployments/dra-driver/daemonset.yaml`: DaemonSet with host mounts for /dev, /etc/cdi, kubelet plugin dirs
- `Dockerfile.dra-driver`: multi-stage build
- Makefile targets: build, docker-build-dra

### Step 6: Tests ✅
- `pkg/dra/resourceslice_test.go`: 9 tests covering empty, single, multi, no-UUID, slice overflow, device naming, CDI ID format
- All existing tests still pass

### Step 7: Documentation — TODO
- Update docs/ with DRA driver usage
- Update CLAUDE.md project status

## Files Created/Modified

### New files
- `pkg/dra/resourceslice.go` — ResourceSlice builder
- `pkg/dra/plugin.go` — DRA plugin (kubeletplugin.DRAPlugin)
- `pkg/dra/resourceslice_test.go` — unit tests
- `cmd/accelerator-dra-driver/main.go` — DRA driver binary
- `deployments/dra-driver/rbac.yaml` — RBAC manifests
- `deployments/dra-driver/daemonset.yaml` — DaemonSet manifest
- `Dockerfile.dra-driver` — container image

### Modified files
- `go.mod` / `go.sum` — added K8s + DRA dependencies
- `Makefile` — added DRA driver build/docker targets
