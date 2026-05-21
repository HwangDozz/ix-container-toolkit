# DRA Driver 集成测试报告

> 测试日期：2026-05-19
> 测试节点：kunlun-02（openEuler 22.03, aarch64）
> 测试分支：cdi-dra-implement

## 1. 测试环境

### 1.1 硬件环境

| 项目 | 值 |
|------|-----|
| GPU 型号 | Ascend 910B × 8 |
| 设备节点 | `/dev/davinci0` - `/dev/davinci7` |
| 控制设备 | `/dev/davinci_manager`, `/dev/devmm_svm`, `/dev/hisi_hdc` |
| 驱动路径 | `/usr/local/Ascend/driver/` |
| 工具链路径 | `/usr/local/Ascend/ascend-toolkit/` |

### 1.2 软件环境

| 项目 | 值 |
|------|-----|
| 操作系统 | openEuler 22.03 (LTS-SP4), aarch64 |
| Kubernetes | v1.35.2 |
| containerd | 2.2.1 |
| Go | 1.26.1 |
| DRA API | resource.k8s.io/v1（GA） |
| CDI 支持 | `enable_cdi = true`，spec 目录 `/etc/cdi`, `/var/run/cdi` |

### 1.3 集群节点状态

kunlun-02 节点存在磁盘压力（root 分区 94%），导致 `node.kubernetes.io/disk-pressure:NoSchedule` taint。测试 Pod 需要添加相应 toleration。

## 2. 测试流程

### 2.1 阶段一：单元测试

**目的：** 验证代码编译和核心逻辑正确性。

```bash
go test ./...
```

**结果：**

| 包 | 状态 |
|----|------|
| `pkg/cdi` | 通过 |
| `pkg/device` | 通过 |
| `pkg/dra` | 通过 |
| `pkg/profile` | 通过 |
| `pkg/strutil` | 通过 |
| `internal/hook` | 失败（缺少 `pkg/runtimeview` 模块，与 DRA 无关） |

### 2.2 阶段二：本地 CDI Spec 生成测试

**目的：** 验证 CDI spec 生成器能否正确发现设备并生成符合 CDI 0.8.0 标准的 spec 文件。

```bash
go build -o /tmp/cdi-generate ./cmd/cdi-generate
sudo /tmp/cdi-generate --profile ./profiles/ascend-910b.yaml
```

**结果：**
- 设备发现：8 个设备
- CDI spec 写入：`/etc/cdi/huawei.json`
- 设备条目：8 个（name: "0" - "7"）
- Mount 条目：1456 个（驱动库 so 文件）
- 每设备包含：4 个 deviceNode + 12 个环境变量 + 178 个 mount

### 2.3 阶段三：构建 DRA Driver 二进制

```bash
GOOS=linux GOARCH=arm64 go build -o bin/linux/arm64/accelerator-dra-driver ./cmd/accelerator-dra-driver
```

构建产物：65MB ELF 二进制（linux/arm64）。

### 2.4 阶段四：部署 DRA Driver

**方式选择：** 由于节点磁盘压力导致 DaemonSet Pod 被驱逐，改为直接在节点上运行二进制。

```bash
sudo cp bin/linux/arm64/accelerator-dra-driver /usr/local/bin/
sudo mkdir -p /etc/accelerator-toolkit/profiles
sudo cp profiles/ascend-910b.yaml /etc/accelerator-toolkit/profiles/active.yaml
sudo /usr/local/bin/accelerator-dra-driver \
  --profile=/etc/accelerator-toolkit/profiles/active.yaml \
  --cdi-dir=/etc/cdi \
  --node-name=kunlun-02 \
  --kubeconfig=/home/huangsy/.kube/config
```

**验证项：**

| 检查项 | 结果 |
|--------|------|
| 设备发现 | 8 个设备 |
| ResourceSlice 创建 | `00000-gpu.accelerator-toolkit.io-kunlun-02-*` |
| DRA Socket | `/var/lib/kubelet/plugins/gpu.accelerator-toolkit.io/dra.sock` |
| 设备属性 | vendor=Ascend, model=910B, path=/dev/davinciN |

### 2.5 阶段五：DRA 端到端测试

**创建测试资源：**

1. **DeviceClass** — 选择 `gpu.accelerator-toolkit.io` driver 的设备
2. **ResourceClaimTemplate** — 请求 1 个 GPU
3. **Pod** — 使用 `runc-cdi` RuntimeClass，挂载 ResourceClaim

```bash
kubectl apply -f /tmp/dra-test/deviceclass.yaml
kubectl apply -f /tmp/dra-test/test-pod.yaml
```

**验证结果：**

| 检查项 | 结果 |
|--------|------|
| ResourceClaim 状态 | `allocated,reserved` |
| CDI Spec 设备名 | `index-0-<claimUID>`（带 claim UID 后缀） |
| 容器内 `/dev/davinci0` | 存在（仅已分配的设备） |
| 容器内 `/dev/davinci1-7` | 不存在（未分配的设备不注入） |
| 容器内 `/dev/davinci_manager` | 存在（控制设备） |
| 环境变量 `ASCEND_VISIBLE_DEVICES` | `all` |
| 环境变量 `LD_LIBRARY_PATH` | 包含驱动库路径 |
| 环境变量 `ASCEND_TOOLKIT_HOME` | `/usr/local/Ascend/ascend-toolkit/latest` |

## 3. 工作原理

### 3.1 DRA 工作流程

```
用户 Pod 提交 ResourceClaimTemplate
        ↓
API Server 创建 ResourceClaim
        ↓
DRA Allocator 匹配 DeviceClass 中的 CEL 表达式
        ↓
从 ResourceSlice 中选择符合条件的设备
        ↓
ResourceClaim 状态变为 allocated,reserved
        ↓
kubelet 调用 DRA driver 的 NodePrepareResource gRPC
        ↓
DRA driver 生成 CDI spec 写入 /etc/cdi/huawei.json
        ↓
返回 CDI device ID 给 kubelet
        ↓
kubelet 创建容器时传递 CDI device ID 给 containerd
        ↓
containerd 读取 CDI spec，注入设备节点、环境变量、mount
        ↓
容器启动，可访问 GPU
```

### 3.2 CDI Spec 结构

```yaml
cdiVersion: "0.8.0"
kind: "huawei.com/Ascend910"
devices:
  - name: "index-0-<claimUID>"    # 设备名 = 索引 + claim UID
    containerEdits:
      env:                         # 注入的环境变量
        - "ASCEND_VISIBLE_DEVICES=all"
        - "LD_LIBRARY_PATH=..."
      deviceNodes:                 # 注入的设备节点
        - hostPath: "/dev/davinci0"
          path: "/dev/davinci0"
          permissions: "rwm"
        - hostPath: "/dev/davinci_manager"
          path: "/dev/davinci_manager"
          permissions: "rwm"
      mounts:                      # 注入的驱动库
        - hostPath: "/usr/local/Ascend/driver/lib64/driver/libascend_hal.so"
          containerPath: "/usr/local/Ascend/driver/lib64/driver/libascend_hal.so"
          options: ["rbind", "ro"]
      hooks:                       # OCI 生命周期钩子
        - hookName: "prestart"
          path: "/sbin/ldconfig"
          args: ["ldconfig"]
```

### 3.3 设备隔离机制

DRA 的核心优势是**设备级隔离**：

- 旧方案（Device Plugin + OCI hook）：注入所有设备，依赖环境变量选择
- 新方案（DRA + CDI）：只注入已分配的设备，容器内看不到其他设备

当 Pod 请求 1 个 GPU 时：
- CDI spec 只包含 `/dev/davinci0`（被分配的设备）
- 容器内 `ls /dev/davinci*` 只能看到 `/dev/davinci0`
- 其他 7 个设备对容器不可见

### 3.4 Claim UID 后缀的作用

CDI 设备名格式为 `index-0-<claimUID>`，后缀的作用：

- 当同一个物理设备被 unprepare 再 prepare 时（例如 Pod 重建），生成不同的 CDI 设备名
- 避免 CDI cache 冲突
- 确保每次分配都是独立的 CDI 条目

## 4. 问题分析与修复

### 4.1 Bug 1：CDI 设备名不一致

**现象：**

```
CDI device injection failed: unresolvable CDI devices huawei.com/Ascend910=index-0-<claimUID>
```

containerd 无法解析 CDI 设备 ID。

**根因分析：**

项目中存在两个 `deviceName` 函数，命名规则不一致：

| 文件 | 函数 | 返回值 |
|------|------|--------|
| `pkg/cdi/generator.go:68` | `Generator.deviceName()` | `"0"`（纯索引） |
| `pkg/dra/resourceslice.go:92` | `deviceName()` | `"index-0"`（带前缀） |

DRA driver 的工作流程：
1. `PrepareResourceClaims` 调用 `CDIDeviceID()` 生成 CDI device ID：`huawei.com/Ascend910=index-0-<claimUID>`
2. 调用 `cdi.NewGenerator` 生成 CDI spec，设备名为 `0-<claimUID>`（无 `index-` 前缀）
3. containerd 收到 CDI device ID `index-0-<claimUID>`，在 CDI spec 中查找 `index-0-<claimUID>`，找不到

**修复：**

修改 `pkg/cdi/generator.go` 的 `deviceName` 函数，统一使用 `index-N` 格式：

```go
// 修改前
func (g *Generator) deviceName(dev device.Device) string {
    if dev.UUID != "" {
        return dev.UUID
    }
    return fmt.Sprintf("%d", dev.Index)
}

// 修改后
func (g *Generator) deviceName(dev device.Device) string {
    if dev.UUID != "" {
        return dev.UUID
    }
    return fmt.Sprintf("index-%d", dev.Index)
}
```

同步更新 `pkg/cdi/generator_test.go` 中的测试断言。

### 4.2 Bug 2：CDI Hooks 格式错误

**现象：**

```
CDI registry refresh failed: failed to parse CDI Spec "/etc/cdi/huawei.json":
failed to unmarshal CDI Spec: error unmarshaling JSON: while decoding JSON:
json: cannot unmarshal object into Go struct field ContainerEdits.devices.containerEdits.hooks
of type []*specs.Hook
```

containerd 无法解析 CDI spec 中的 hooks 字段。

**根因分析：**

自定义的 CDI types 使用了嵌套对象格式：

```yaml
# 错误格式（自定义）
hooks:
  prestart:
    - hookName: "ldconfig"
      path: "/sbin/ldconfig"
  poststart: []
```

CDI 0.8.0 标准和 containerd 期望的是扁平数组格式：

```yaml
# 正确格式（CDI 标准）
hooks:
  - hookName: "prestart"
    path: "/sbin/ldconfig"
```

**修复：**

修改 `pkg/cdi/types.go`：

```go
// 修改前
type ContainerEdits struct {
    Env         []string     `yaml:"env,omitempty"`
    DeviceNodes []DeviceNode `yaml:"deviceNodes,omitempty"`
    Mounts      []Mount      `yaml:"mounts,omitempty"`
    Hooks       *Hooks       `yaml:"hooks,omitempty"`        // 嵌套对象
}

type Hooks struct {
    Prestart  []Hook `yaml:"prestart,omitempty"`
    Poststart []Hook `yaml:"poststart,omitempty"`
}

// 修改后
type ContainerEdits struct {
    Env         []string     `yaml:"env,omitempty"`
    DeviceNodes []DeviceNode `yaml:"deviceNodes,omitempty"`
    Mounts      []Mount      `yaml:"mounts,omitempty"`
    Hooks       []Hook       `yaml:"hooks,omitempty"`        // 扁平数组
}
```

同步修改 `pkg/cdi/generator.go` 的 `buildHooks` 函数，使用 `hookName: "prestart"` 标识生命周期阶段。

### 4.3 环境问题：ascend-docker-runtime 冲突

**现象：**

```
OCI runtime create failed: /usr/local/Ascend/Ascend-Docker-Runtime/ascend-docker-runtime
did not terminate successfully: exit status 1
```

**根因分析：**

containerd 默认 runc runtime 配置为 `ascend-docker-runtime`：

```toml
[plugins."io.containerd.cri.v1.runtime".containerd.runtimes.runc.options]
    BinaryName = "/usr/local/Ascend/Ascend-Docker-Runtime/ascend-docker-runtime"
```

`ascend-docker-runtime` 是华为提供的 OCI runtime wrapper，用于注入 Ascend 设备。当 CDI 已经注入了设备，两者产生冲突。

**解决方案：**

添加新的 runtime 配置，使用标准 runc：

```toml
[plugins."io.containerd.cri.v1.runtime".containerd.runtimes.runc-cdi]
    runtime_type = "io.containerd.runc.v2"

    [plugins."io.containerd.cri.v1.runtime".containerd.runtimes.runc-cdi.options]
        BinaryName = "/usr/bin/runc"
        SystemdCgroup = true
```

创建对应的 RuntimeClass：

```yaml
apiVersion: node.k8s.io/v1
kind: RuntimeClass
metadata:
  name: runc-cdi
handler: runc-cdi
```

Pod 使用 `runtimeClassName: runc-cdi` 避免冲突。

## 5. 测试产物

| 文件 | 说明 |
|------|------|
| `/tmp/dra-test/deviceclass.yaml` | DeviceClass 定义 |
| `/tmp/dra-test/test-pod.yaml` | ResourceClaimTemplate + Pod 定义 |
| `/tmp/dra-test/runtimeclass.yaml` | runc-cdi RuntimeClass 定义 |
| `/tmp/dra-test/daemonset-test.yaml` | DRA Driver DaemonSet（测试用） |

## 6. 后续工作

1. **DaemonSet 部署验证**：解决磁盘压力问题后，验证 DaemonSet 方式部署
2. **多 GPU 分配测试**：验证 count > 1 时的设备分配和 CDI 注入
3. **设备释放测试**：验证 Pod 删除后 CDI spec 清理和设备释放
4. **并发分配测试**：多个 Pod 同时请求 GPU 的场景
5. **与 ascend-device-plugin 共存**：验证 DRA 和传统 Device Plugin 在同一节点上的共存
6. **containerd 配置标准化**：将 runc-cdi runtime 配置集成到 installer 流程中
---

# DRA + CDI E2E Test Context Handoff

Date: 2026-05-20

## Purpose

Save the current DRA driver + CDI integration context before moving testing to an Iluvatar node. The target path is Kubernetes DRA allocation, kubelet NodePrepareResources, claim-scoped CDI spec generation, runtimeClassName runc-cdi injection, smoke test, short training test, and CDI cleanup on unprepare.

## Fixes Already Made

- deployments/dra-driver/rbac.yaml: resourceslices now allow get, list, watch, create, update, patch, delete.
- deployments/dra-driver/daemonset.yaml: driver Pod now mounts host /usr/local/Ascend at /usr/local/Ascend. For Iluvatar, the same rule applies to /usr/local/corex.
- pkg/cdi/writer.go: CDI spec writes are atomic.
- pkg/cdi/generator.go: so-only generation skips zero-byte shared libraries after symlink resolution.
- pkg/dra/plugin.go: Unprepare cleans claim-scoped CDI entries by claim UID even after driver restart.
- profiles/ascend-910b.yaml: added Python 3.11.14 paths, aarch64-linux/lib64, and optional driver-binaries.
- Ascend experiment jobs now resolve python3, python, and /usr/local/python*/bin/*, with apt-get python3 fallback.

## Ascend 910B Results

Environment: Kubernetes v1.35.2, node kunlun-02, containerd 2.2.1, RuntimeClass runc-cdi, DRA driver gpu.accelerator-toolkit.io.

Validated:

- DeviceClass and ResourceClaimTemplate apply successfully.
- ResourceSlice exists for gpu.accelerator-toolkit.io on kunlun-02 with devices index-0..index-7.
- ResourceClaim reaches allocated,reserved.
- DRA driver logs Prepared resource claim and returns claim-scoped CDI ID, for example huawei.com/Ascend910=index-0-<claimUID>.
- CDI cleanup works: after failed smoke Job, /etc/cdi/huawei.json was removed by UnprepareResourceClaims.
- python3 not found was fixed by interpreter resolution.
- libhccl.so: file too short was root-caused to zero-byte host libhccl.so files. After skipping zero-byte .so files, smoke saw a valid 642K ELF libhccl.so and torch_npu imported successfully.

Remaining Ascend blockers:

- smoke reached application runtime but failed with torch.npu.is_available() false and device_count 0.
- Before fixing driver Pod same-path mounts, CDI generation skipped host driver lib paths because the driver container could not see /usr/local/Ascend/... directly.
- kunlun-02 showed kubelet/containerd context deadline exceeded while starting ordinary containers and the temporary DRA driver Pod, preventing final corrected smoke/train rerun.

## Iluvatar BI-V150 Profile Facts

From profiles/iluvatar-bi-v150.yaml:

- Kubernetes resource: iluvatar.com/gpu.
- Node label: iluvatar.ai/gpu=present.
- Device glob: /dev/iluvatar*.
- Selector env var: ILUVATAR_COREX_VISIBLE_DEVICES.
- Driver libraries: /usr/local/corex/lib64 and /usr/local/corex/lib.
- Driver binaries: /usr/local/corex/bin.
- Driver library and binary artifacts are optional.

## Iluvatar Test Plan

1. Check node and runtime prerequisites.

```bash
kubectl get nodes -l iluvatar.ai/gpu=present -o wide
kubectl get runtimeclass runc-cdi -o yaml
kubectl get resourceslices -o wide | grep gpu.accelerator-toolkit.io || true
```

2. Check the Iluvatar node facts.

```bash
ls -la /dev/iluvatar* 2>/dev/null
ls -la /usr/local/corex /usr/local/corex/lib64 /usr/local/corex/bin 2>/dev/null
/usr/local/corex/bin/ixsmi --query-gpu=index,uuid --format=csv 2>/dev/null || true
```

3. Deploy the DRA driver as a Pod/DaemonSet, not as a manually started host process.

Required mounts for Iluvatar driver Pod:

- host /etc/cdi to the CDI output path.
- host /usr/local/corex to container /usr/local/corex.
- host /dev read-only for discovery.
- /var/lib/kubelet/plugins and /var/lib/kubelet/plugins_registry.

4. Confirm driver startup and ResourceSlices.

```bash
kubectl apply -f deployments/dra-driver/rbac.yaml
kubectl apply -f deployments/dra-driver/daemonset.yaml
kubectl -n kube-system get pod -l app=accelerator-dra-driver -o wide
kubectl -n kube-system logs -l app=accelerator-dra-driver --tail=200
kubectl get resourceslices -o wide
```

5. Create Iluvatar DRA manifests mirroring Ascend experiments.

- DeviceClass selecting device.driver == "gpu.accelerator-toolkit.io".
- ResourceClaimTemplate requesting one device.
- Debug Pod with runtimeClassName runc-cdi, resources.claims, and node selector iluvatar.ai/gpu=present.
- Smoke Job that prints runtime facts before framework code.
- Short training Job only after smoke confirms the device is visible.

6. Debug Pod checks.

```bash
env | sort | grep -E "ILUVATAR|COREX|LD_LIBRARY_PATH|PATH"
ls -la /dev/iluvatar* 2>/dev/null || true
ls -la /usr/local/corex /usr/local/corex/lib64 /usr/local/corex/bin 2>/dev/null || true
command -v ixsmi || true
ixsmi || true
```

7. CDI checks while the debug Pod is running.

```bash
kubectl get resourceclaims -A
kubectl describe resourceclaim -n <namespace> <claim-name>
ls -la /etc/cdi
cat /etc/cdi/*.json
```

## Iluvatar Pass Criteria

- ResourceClaim becomes allocated,reserved.
- DRA driver logs Prepared resource claim.
- A claim-scoped CDI entry appears in /etc/cdi.
- Workload starts with runtimeClassName runc-cdi.
- Container sees allocated /dev/iluvatar* and ILUVATAR_COREX_VISIBLE_DEVICES.
- CoreX libraries/binaries are visible if present on host.
- ixsmi or framework backend can query the device.
- Deleting the Pod/Job removes the claim-specific CDI entry.

## Failure Signatures

- dial tcp <apiserver>: connect: network is unreachable: driver is using a bad node-local kubeconfig; use in-cluster ServiceAccount config.
- required path ... not found: profile path is not optional, or driver Pod cannot see host path at the same absolute path.
- framework imports but device count is zero: compare generated CDI mounts/env with host runtime requirements, especially /dev nodes and driver library paths.
- context deadline exceeded during container start: kubelet/containerd runtime issue on node; fix node health before treating it as a DRA/CDI bug.

## Local Verification Already Passing

```bash
go test ./pkg/cdi ./pkg/dra ./pkg/device ./pkg/profile ./pkg/strutil ./cmd/accelerator-dra-driver
git diff --check
kubectl apply --dry-run=client -f deployments/dra-driver/daemonset.yaml
```

---

# Iluvatar BI-V150 on inspur-01 E2E Results

Date: 2026-05-21

## Environment

| Item | Value |
|------|-------|
| Node | inspur-01 |
| OS / Arch | Ubuntu 22.04, amd64 |
| Kubernetes | v1.35.2 |
| containerd | 1.7.28 |
| GPU | Iluvatar BI-V150 x 8 |
| Device nodes | /dev/iluvatar0 - /dev/iluvatar7 |
| Final runtime path | RuntimeClass runc-cdi -> standard runc with containerd CDI enabled |
| Diagnostic runtime path | RuntimeClass ix -> ix-container-runtime + ix-container-hook |

containerd was not upgraded to 2.2.1 because CDI injection worked after enabling CDI in the existing 1.7.28 config. The existing storage settings were preserved:

- `root = "/storage/containerd"`
- `state = "/storage/containerd-data"`

The runtime config was extended with `enable_cdi = true` and a dedicated `runc-cdi` handler using `/usr/sbin/runc`.

## RuntimeClass ix Conclusion

`runtimeClassName: ix` selects the containerd runtime handler named `ix`, which is configured to invoke `/usr/local/bin/ix-container-runtime`. That wrapper injects `/usr/local/bin/ix-container-hook` during container create and represents the legacy runtime/hook path.

For DRA + CDI validation, `ix` must only be used as a diagnostic comparison path. The target path is `runtimeClassName: runc-cdi`, where kubelet passes DRA-allocated CDI device IDs to containerd and containerd applies the CDI spec directly.

## Deployment

Manifest directory:

`experiments/iluvatar-bi-v150/pytorch-backend/k8s-dra/`

Main resources:

- `daemonset-local-binary.yaml`: DRA driver DaemonSet pinned to inspur-01, using the local amd64 binary and `profiles/iluvatar-bi-v150.yaml`.
- `debug-pod.yaml`: pure `runc-cdi` debug Pod.
- `smoke-job.yaml`: pure `runc-cdi` framework smoke Job.
- `train-single-job.yaml`: pure `runc-cdi` single-card training Job.
- `*-ix-runtime.yaml`: legacy runtime comparison manifests only.
- `oci-spec-compare-job.yaml` and `oci-spec-full-diff-job.yaml`: host-side OCI spec comparison utilities.

Driver status:

- DaemonSet: `kube-system/accelerator-dra-driver-iluvatar`
- Pod: `1/1 Running` on inspur-01
- Logs:
  - `Loaded profile profile=iluvatar-bi-v150`
  - `Discovered devices count=8`
  - `DRA driver started devices=8 driver=gpu.accelerator-toolkit.io node=inspur-01 resource=iluvatar.com/gpu`

## DRA and CDI Validation

ResourceSlice:

- Driver: `gpu.accelerator-toolkit.io`
- Node: `inspur-01`
- Devices: `index-0` through `index-7`
- Attributes include `vendor=Iluvatar`, `model=BI-V150`, and `/dev/iluvatarN` paths.

Claim-scoped prepare/unprepare was validated from driver logs:

- Smoke Job prepared `iluvatar.com/gpu=index-2-<claimUID>` and later unprepared it.
- Single-card training prepared `iluvatar.com/gpu=index-3-<claimUID>` and later unprepared it.

Pure `runc-cdi` debug Pod validation:

- ResourceClaim reached `allocated,reserved`.
- Container saw only the allocated `/dev/iluvatarN`.
- `ILUVATAR_COREX_VISIBLE_DEVICES=all` was injected.
- `ixsmi` worked.
- `torch.cuda.is_available()` returned `true`.
- `torch.cuda.device_count()` returned `1`.

## Workload Results

Pure `runc-cdi` smoke Job:

```json
{
  "torch": "2.4.1",
  "torch_cuda_version": "10.2",
  "device": "cuda:0",
  "cuda_is_available": true,
  "cuda_device_count": 1,
  "steps": 10,
  "first_loss": 14399.4033203125,
  "final_loss": 0.0
}
```

Pure `runc-cdi` single-card training Job:

```json
{
  "torch": "2.4.1",
  "device": "cuda:0",
  "distributed": false,
  "steps": 20,
  "batch_size": 32,
  "first_loss": 2.4377217292785645,
  "final_loss": 2.2593231201171875
}
```

Diagnostic `ix` runtime comparison:

- DRA ResourceClaim and CDI injection also worked.
- Minimal tensor/matmul/backward diagnostic completed.
- The comparison proved legacy runtime/hook can coexist with DRA allocation, but it is not the desired final architecture.

## OCI Comparison

The full OCI config diff between a pure `runc-cdi` debug container and an `ix` debug container showed no material differences after normalizing pod/container IDs and device index:

- Both had only the CDI prestart `/sbin/ldconfig` hook in the final OCI config.
- Both had the allocated `/dev/iluvatarN` device node and matching cgroup device allow rule.
- Relevant env vars and mounts were equivalent.

This means any old `ix-container-runtime` side effect is either outside the final persisted OCI config or not needed for the passing `runc-cdi` smoke/train jobs.

## Final Status

The inspur-01 Iluvatar BI-V150 DRA + CDI end-to-end path passed:

- DaemonSet deployment: passed
- Device discovery and ResourceSlice publication: passed
- Pod-level DRA allocation and CDI injection: passed
- In-container device and environment validation: passed
- Framework smoke test on pure `runc-cdi`: passed
- Single-card training on pure `runc-cdi`: passed
- Claim cleanup after Job completion: passed

`runtimeClassName: ix` remains useful only for diagnostics. The final target should continue to be DRA + CDI + standard runc through `runtimeClassName: runc-cdi`, removing dependence on the old hook/runtime path.
