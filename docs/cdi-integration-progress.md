# CDI Integration Progress

> Last updated: 2026-05-13
> Branch: `abstract` (uncommitted changes)

## Goal

为 accelerator-toolkit 引入 CDI (Container Device Interface) 支持，作为当前 runtime shim + OCI hook 注入路径的替代方案。CDI 是容器运行时原生支持的声明式设备描述标准，containerd 1.6+ 和 CRI-O 可以直接消费 CDI spec，无需自定义 runtime shim 或 OCI hook。

## 代码架构上下文

### 两层 CDI 实现

项目中存在**两套独立的 CDI 实现**，职责不同：

| 层 | 包 | 用途 | 是否检查宿主机文件系统 | 输出 |
|---|---|---|---|---|
| 静态渲染器 | `pkg/profile/cdi.go` | 从 profile YAML 生成模板 CDI spec，用于预览/调试 | 否 | 单个 device entry (name="all") |
| 动态生成器 | `pkg/cdi/` | 安装时在节点上生成真实 CDI spec | 是 | 每个物理 GPU 一个 device entry |

静态渲染器通过 `accelerator-profile-render cdi` 子命令调用（`cmd/accelerator-profile-render/main.go:116-147`），用于在非 GPU 节点上预览 CDI spec 结构。

动态生成器由 installer 的 `generateCDISpec()` 步骤调用，是生产路径。

### Profile 模型如何驱动 CDI 生成

`pkg/profile/profile.go` 定义的 `Profile` 结构是 CDI 生成的唯一输入：

```
Profile.Kubernetes.ResourceNames[0]  →  CDI kind (如 "iluvatar.ai/gpu")
Profile.Device.SelectorEnvVars[0]    →  注入容器的设备选择环境变量名
Profile.Device.DeviceGlobs           →  设备节点发现的 glob 模式
Profile.Device.ControlDeviceGlobs    →  控制设备节点的 glob 模式
Profile.Device.Mapping               →  UUID 到 index 的映射命令配置
Profile.Inject.ContainerRoot         →  容器内挂载根路径
Profile.Inject.Artifacts             →  需要注入的 artifact 列表 (库、目录、设备节点)
Profile.Inject.ExtraEnv              →  额外环境变量
Profile.Inject.Linker.RunLdconfig    →  是否生成 ldconfig hook
Profile.Inject.Linker.ConfigPath     →  ld.so.conf.d 片段路径
```

### 设备发现流程

`pkg/device/device.go` 提供设备发现能力，CDI 和 OCI hook 共用同一套发现逻辑：

```
device.DiscoverWithProfile(visibleDevices, profile, log)
  → enumerateAll(profile.Device.DeviceGlobs)  // filepath.Glob 扫描 /dev/* 设备
  → filterDevices()                            // 根据 visibleDevices 参数过滤
      → "all": 返回全部
      → "none": 返回空
      → UUID 列表: queryMapping() 调用厂商 CLI 做 UUID→index 映射
      → 数字列表: 直接按 index 过滤
```

`queryMapping()` 使用 profile 中 `Device.Mapping` 配置的命令（如 `ixsmi --query-gpu=index,uuid --format=csv`）来建立 UUID 到设备路径的映射。

### 代码复用与重复

以下逻辑在多个包中存在重复实现：

| 函数 | 位置 1 | 位置 2 | 差异 |
|---|---|---|---|
| `isSharedLibrary()` | `pkg/cdi/generator.go:324-339` | `internal/hook/hook.go:198-208` | CDI 版更严格：要求 `.so.` 后第一个字符必须是数字；hook 版更宽松：匹配任意 `.so.` |
| `artifactContainerPath()` | `pkg/cdi/generator.go:342-348` | `pkg/profile/cdi.go:166-172` | 逻辑完全相同 |
| CDI 类型定义 | `pkg/cdi/types.go` (Spec, Device, Mount 等) | `pkg/profile/cdi.go` (CDISpec, CDIDevice, CDIMount 等) | profile 版缺少 `Hooks` 支持 |

**已完成的重构** (2026-05-13):

1. **统一 `isSharedLibrary()`** — 提取到 `pkg/strutil/library.go`，CDI 和 hook 包均改为调用 `strutil.IsSharedLibrary()`，消除了行为差异
2. **合并 CDI 类型定义** — 将 `pkg/profile/cdi.go` 的静态渲染器移入 `pkg/cdi/preview.go`，使用 `pkg/cdi/types.go` 的类型定义，删除了 `pkg/profile/cdi.go` 中的重复类型。`cmd/accelerator-profile-render` CLI 工具改为调用 `cdi.RenderPreviewSpec()`

## Current Architecture vs CDI

```
当前路径:
  Pod → Device Plugin → env var → runtime shim 拦截 create → 注入 OCI hook → hook 做 bind-mount + ldconfig

CDI 路径:
  Pod → Device Plugin → env var → installer 生成 CDI spec → containerd 原生消费 CDI spec
```

CDI 路径下可以删除的组件:
- `cmd/accelerator-container-runtime` (runtime shim)
- installer 中的 `patchContainerd()` 步骤
- RuntimeClass 资源 (`xpu-runtime` handler)

## What Was Done

### 1. New package: `pkg/cdi/` (4 files)

**`pkg/cdi/types.go`** (61 行) — CDI 1.1.0 spec 类型定义，包含 `Hooks` 支持 (用于 ldconfig 生命周期钩子)。类型层次: `Spec → Device → ContainerEdits → {Env, DeviceNodes, Mounts, Hooks}`。

**`pkg/cdi/generator.go`** (374 行) — 节点本地 CDI spec 生成器:
- `Generator` 结构持有 profile + 已发现设备列表 + logger
- `Generate() (*Spec, error)` 是主入口，返回完整 CDI spec
- 为每个物理 GPU 生成一个 CDI device entry (以 UUID 命名，无 UUID 时 fallback 到 index)
- `buildMounts()` 遍历 profile 的 `Inject.Artifacts`，跳过 `device-nodes` kind
- `buildSoOnlyMounts()` 扫描宿主机目录，逐个挂载 `.so` 文件 (通过 `isSharedLibrary()` 过滤)，排除 `ExcludeDirs` 中的子目录
- `buildArtifactMounts()` 处理三种模式: `bind`(目录挂载)、`so-only`(逐文件挂载)、`copy`(降级为 bind)
- `mergeDeviceEdits()` 将公共 edits 深拷贝后合并每个设备专属的 device nodes，包括 `ControlDeviceGlobs` 展开
- `resolveHostPaths()` 去重 + 检查路径存在性，optional 路径缺失时静默跳过

**`pkg/cdi/writer.go`** (50 行) — 序列化 spec 为 YAML，写入 `/etc/cdi/<vendor>.json`。`specFilename()` 从 kind 提取 vendor 域名并小写化 (如 `"huawei.com/Ascend910"` → `"huawei.json"`)。

**`pkg/cdi/generator_test.go`** (481 行) — 14 个测试，覆盖:
- Kind 生成、per-device entries、UUID fallback、环境变量
- Device nodes、so-only 过滤、hooks、边界条件
- `setupTestProfile(t)` 辅助函数创建带真实临时目录的完整 profile

### 2. Modified: `cmd/accelerator-installer/main.go`

在 installer 流水线中新增 `generateCDISpec()` 步骤 (在 `writeConfig` 之后、`patchContainerd` 之前):

```
1. copy binaries
2. write config
3. generate CDI spec  ← 新增
4. patch containerd
5. label node
6. restart containerd
```

`generateCDISpec()` 函数 (第 167-217 行) 的具体逻辑:
- 检查 `CDI_ENABLED` 环境变量，`"false"` 时跳过
- 从 `profile.Device.SelectorFormats` 中检测是否支持 `"all"` 格式
- 调用 `device.DiscoverWithProfile("all", ...)` 发现节点上所有加速器设备
- 设备数为 0 时返回 warning (非 error)，不阻塞安装流程
- 使用 `cdi.NewGenerator` + `gen.Generate()` 生成 CDI spec
- 写入 `<HOST_MOUNT>/etc/cdi/` (默认 `/host/etc/cdi/`，通过 hostPath 映射到宿主机 `/etc/cdi/`)
- 日志输出写入路径、设备数和 CDI kind

## Uncommitted Files

```
M  cmd/accelerator-installer/main.go      (+66 lines, generateCDISpec 函数 + 流水线集成)
M  cmd/accelerator-profile-render/main.go (改用 cdi.RenderPreviewSpec)
M  internal/hook/hook.go                  (改用 strutil.IsSharedLibrary)
M  pkg/profile/profile_test.go            (移除 CDI 测试，已迁移到 pkg/cdi)
D  pkg/profile/cdi.go                     (已删除，功能合并到 pkg/cdi/preview.go)
A  pkg/cdi/generator.go                   (new, 374 行)
A  pkg/cdi/generator_test.go              (new, 481 行, 14 个测试)
A  pkg/cdi/preview.go                     (new, 从 pkg/profile/cdi.go 迁移的静态渲染器)
A  pkg/cdi/preview_test.go                (new, preview 渲染器测试)
A  pkg/cdi/types.go                       (new, 61 行)
A  pkg/cdi/writer.go                      (new, 50 行)
A  pkg/strutil/library.go                 (new, 共享的 IsSharedLibrary 函数)
A  pkg/strutil/library_test.go            (new, IsSharedLibrary 测试)
```

## 与现有 hook/shim 路径的关系

CDI 路径和现有的 runtime shim + OCI hook 路径**可以共存**：

- CDI 生成步骤在 installer 中是独立的，失败不阻塞后续的 `patchContainerd` 步骤
- 现有路径: Pod 指定 `runtimeClassName: xpu-runtime` → containerd 调用 `accelerator-container-runtime` → 注入 OCI hook → hook 做 bind-mount
- CDI 路径: Pod 通过 `cdi.k8s.io/devices` annotation 请求设备 → containerd 原生读取 `/etc/cdi/<vendor>.json` → 直接注入
- 两条路径互不干扰：CDI spec 文件存在时，containerd 的 CDI 逻辑生效；没有 CDI spec 时，走原有 shim 路径

### 现有路径中可删除的组件 (阶段 2)

一旦 CDI 路径验证稳定，以下组件可以安全删除：
- `cmd/accelerator-container-runtime/` — runtime shim，仅用于透明注入 OCI hook
- installer 中的 `patchContainerd()` 步骤 — 注册 accelerator runtime handler
- `deployments/runtimeclass/` — `xpu-runtime` RuntimeClass 资源
- `cmd/accelerator-container-hook/` — OCI prestart hook 二进制

### 静态 CDI 渲染器 (`pkg/profile/cdi.go`)

这个文件是 CDI 的早期原型实现，保留用于：
- `accelerator-profile-render cdi` CLI 工具：在非 GPU 节点上预览 CDI spec 结构
- 不检查宿主机文件系统，不展开 glob，不做 `.so` 过滤
- 输出单个 device entry (name="all")，不像动态生成器那样为每个物理设备生成独立 entry
- 类型定义与 `pkg/cdi/types.go` 重复但缺少 `Hooks` 支持

## Key Design Decisions

### so-only 展开策略

`pkg/profile/cdi.go` 中的静态 CDI renderer 无法表达 `so-only` 语义。`pkg/cdi/generator.go` 在 spec 生成时扫描宿主机目录，将 `.so` 文件逐个列出为 mount，而非 bind-mount 整个目录。这与 NVIDIA `nvidia-ctk cdi generate` 的做法一致。

### CDI Hooks 用于 ldconfig

CDI 1.1 支持 `containerEdits.hooks.prestart`。当 profile 的 `linker.runLdconfig: true` 时，生成一个调用 `/sbin/ldconfig` 的 prestart hook。

注意: CDI hooks 的 `path` 必须是容器内的绝对路径。当前实现假设容器镜像中有 `/sbin/ldconfig`。如果目标镜像缺少此命令，需要改用其他方式 (如在 CDI mounts 中直接提供已配置的 `ld.so.cache`)。

### Device 命名

每个 GPU 设备以 UUID 命名 (如 `GPU-aaaa`)，无 UUID 时 fallback 到 index (如 `0`)。这允许用户通过 `cdi.k8s.io/devices` annotation 选择特定 GPU。

## 本地验证结果 (2026-05-13, kunlun-02)

### CDI Spec 生成验证 — 通过

在 kunlun-02 (8x Ascend 910B, driver v25.5.1, CANN 8.5.1) 上成功生成 CDI spec：

- 设备发现: 8 个 davinci 设备 (davinci0-7)，正确跳过 davinci_manager 等控制设备
- 环境变量: 12 个 (1 selector + 11 extraEnv)
- 设备节点: 每设备 4 个 (1 compute + 3 control)
- 挂载点: 每设备 178 个 (so-only 展开的 .so 文件 + bind 目录)
- Hooks: ldconfig prestart hook 正确生成
- Optional 路径: `/usr/local/Ascend/nnal/atb/latest/atb/cxx_abi_0/lib` 正确跳过 (不存在)
- CDI spec 写入 `/etc/cdi/huawei.json` (371KB, 7403 行)

### containerd CDI 消费验证 — 未通过

containerd v1.7.28 配置 `enable_cdi = true` 后，`cdi.k8s.io/devices` annotation **未被处理**：

- containerd 日志始终显示 `CDI devices from CRI Config.CDIDevices: []`
- 无论 annotation 放在 pod 级别还是 container 级别，CDIDevices 始终为空
- `ctr` CLI 不支持 `--cdi-devices` flag
- crictl 直接创建 container 时 `cdi_devices` 字段也未生效

**原因分析**: containerd 1.7.28 的 CRI 插件可能不支持从 Kubernetes annotation 自动解析 CDI 设备。CDI annotation 处理 (`cdi.k8s.io/devices`) 可能需要更新版本的 containerd 或特定的 CRI API 支持。

### 下一步排查方向

1. 确认 containerd 1.7.28 是否支持 `cdi.k8s.io/devices` annotation 解析
2. 如果不支持，考虑升级 containerd 到支持 CDI annotation 的版本
3. 或者通过 device plugin 的 Allocate 响应来传递 CDI 设备信息（需要修改 device plugin）

## Verification on Ascend Node

### Step 1: 构建 installer 镜像

```bash
make docker-build-installer  # 或对应的镜像构建命令
```

### Step 2: 部署到昇腾节点

```bash
make deploy  # 或 kubectl apply -f 部署清单
```

### Step 3: 验证 CDI spec 生成

在 GPU 节点上检查:

```bash
cat /etc/cdi/huawei.json
```

预期内容结构:
```yaml
cdiVersion: 1.1.0
kind: huawei.com/Ascend910
devices:
  - name: "0"  # 或 UUID
    containerEdits:
      env:
        - ASCEND_VISIBLE_DEVICES=all
        - LD_LIBRARY_PATH=...
        - PATH=...
      deviceNodes:
        - hostPath: /dev/davinci0
          path: /dev/davinci0
          permissions: rwm
        - hostPath: /dev/davinci_manager
          ...
      mounts:
        - hostPath: /usr/local/Ascend/driver/lib64/common/libascend_driver.so
          containerPath: /usr/local/Ascend/driver/lib64/common/libascend_driver.so
          options: [rbind, ro]
        ...
      hooks:
        prestart:
          - hookName: accelerator-toolkit-ldconfig
            path: /sbin/ldconfig
```

### Step 4: 验证 containerd CDI 消费

如果节点 containerd 版本 >= 1.6 且启用了 CDI 支持，可以测试不指定 `runtimeClassName` 的 Pod:

```yaml
apiVersion: v1
kind: Pod
metadata:
  name: cdi-test
  annotations:
    cdi.k8s.io/devices: "huawei.com/Ascend910=0"
spec:
  containers:
  - name: test
    image: ascend-test:latest
    command: ["npu-smi", "info"]
```

如果 containerd 未启用 CDI，现有的 runtime shim + hook 路径仍然可用 (不受本次修改影响)。

## Known Limitations

1. **CDI hooks 的 ldconfig 路径假设** — 当前写死 `/sbin/ldconfig`，部分精简镜像可能没有此命令
2. **Control device 的 CDI 表达** — CDI spec 中 control devices (如 `/dev/davinci_manager`) 作为 deviceNodes 注入，这是正确的
3. **copy 模式降级为 bind** — CDI 不支持 copy 语义，`copy` 模式的 artifact 降级为 `rbind,ro` mount
4. **containerd CDI 启用** — 需要 containerd 配置中 `[plugins."io.containerd.grpc.v1.cri".containerd]` 下设置 `cdi_spec_dirs = ["/etc/cdi"]`
5. **YAML 内容 + `.json` 扩展名** — `writer.go` 使用 `yaml.Marshal` 序列化但文件命名为 `*.json`，CDI 规范允许两种格式，但命名可能造成混淆
6. **~~`isSharedLibrary()` 实现不一致~~** — 已修复：统一使用 `pkg/strutil.IsSharedLibrary()`
7. **无 CDI spec 清理机制** — installer 只写入当前 vendor 的 CDI spec 文件，如果设备被移除或 profile 变更，旧的 CDI spec 不会自动清理（但覆盖写入是幂等的）
8. **~~类型定义重复~~** — 已修复：`pkg/profile/cdi.go` 的类型已合并到 `pkg/cdi/`，静态渲染器移至 `pkg/cdi/preview.go`

## Next Steps

1. **在昇腾节点上验证 CDI spec 生成的正确性** — 构建 installer 镜像并部署，检查 `/etc/cdi/huawei.json` 内容是否包含正确的设备节点、库挂载和 hooks
2. **确认 containerd 版本和 CDI 配置** — 需要 containerd >= 1.6 且配置中 `cdi_spec_dirs` 包含 `/etc/cdi`
3. **测试 CDI 原生注入路径** — 创建不指定 `runtimeClassName` 的 Pod，通过 `cdi.k8s.io/devices` annotation 请求设备
4. **~~统一 `isSharedLibrary()` 实现~~** — 已完成：提取到 `pkg/strutil/`
5. **~~合并 CDI 类型定义~~** — 已完成：静态渲染器移入 `pkg/cdi/preview.go`，删除 `pkg/profile/cdi.go`
6. **如果 CDI 路径稳定，删除 runtime shim 和 hook (阶段 2)** — 可删除 `cmd/accelerator-container-runtime/`、`cmd/accelerator-container-hook/`、installer 的 `patchContainerd()` 步骤和 RuntimeClass 资源
7. **考虑 DRA driver 实现 (阶段 3，需要 K8s 1.35+)** — DRA 是 CDI 的上层抽象，支持动态资源分配
