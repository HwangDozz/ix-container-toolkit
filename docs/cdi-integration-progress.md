# CDI Integration Progress

> Last updated: 2026-05-13
> Branch: `abstract` (uncommitted changes)

## Goal

为 accelerator-toolkit 引入 CDI (Container Device Interface) 支持，作为当前 runtime shim + OCI hook 注入路径的替代方案。CDI 是容器运行时原生支持的声明式设备描述标准，containerd 1.6+ 和 CRI-O 可以直接消费 CDI spec，无需自定义 runtime shim 或 OCI hook。

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

**`pkg/cdi/types.go`** — CDI 1.1.0 spec 类型定义，包含 `Hooks` 支持 (用于 ldconfig 生命周期钩子)。

**`pkg/cdi/generator.go`** — 节点本地 CDI spec 生成器:
- 输入: profile + 已发现的设备列表
- 为每个物理 GPU 生成一个 CDI device entry (以 UUID 命名)
- `so-only` 模式的 artifact 展开为逐个 `.so` 文件 mount (过滤掉非库文件)
- `bind` 模式的 artifact 作为目录 mount
- 注入环境变量 (`SELECTOR_ENV=all` + profile 的 `extraEnv`)
- 当 `linker.runLdconfig: true` 时生成 CDI prestart hook

**`pkg/cdi/writer.go`** — 序列化 spec 为 YAML，写入 `/etc/cdi/<vendor>.json`。

**`pkg/cdi/generator_test.go`** — 14 个测试，覆盖:
- Kind 生成、per-device entries、UUID fallback、环境变量
- Device nodes、so-only 过滤、hooks、边界条件

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

功能:
- 调用 `device.DiscoverWithProfile("all", ...)` 发现节点上所有加速器设备
- 使用 `cdi.NewGenerator` 生成 CDI spec
- 写入 `/etc/cdi/` (通过 hostPath 映射到宿主机)
- 通过 `CDI_ENABLED=false` 环境变量可跳过

## Uncommitted Files

```
M  cmd/accelerator-installer/main.go   (+66 lines)
A  pkg/cdi/generator.go                (new, ~280 lines)
A  pkg/cdi/generator_test.go           (new, ~340 lines)
A  pkg/cdi/types.go                    (new, ~65 lines)
A  pkg/cdi/writer.go                   (new, ~50 lines)
```

## Key Design Decisions

### so-only 展开策略

`pkg/profile/cdi.go` 中的静态 CDI renderer 无法表达 `so-only` 语义。`pkg/cdi/generator.go` 在 spec 生成时扫描宿主机目录，将 `.so` 文件逐个列出为 mount，而非 bind-mount 整个目录。这与 NVIDIA `nvidia-ctk cdi generate` 的做法一致。

### CDI Hooks 用于 ldconfig

CDI 1.1 支持 `containerEdits.hooks.prestart`。当 profile 的 `linker.runLdconfig: true` 时，生成一个调用 `/sbin/ldconfig` 的 prestart hook。

注意: CDI hooks 的 `path` 必须是容器内的绝对路径。当前实现假设容器镜像中有 `/sbin/ldconfig`。如果目标镜像缺少此命令，需要改用其他方式 (如在 CDI mounts 中直接提供已配置的 `ld.so.cache`)。

### Device 命名

每个 GPU 设备以 UUID 命名 (如 `GPU-aaaa`)，无 UUID 时 fallback 到 index (如 `0`)。这允许用户通过 `cdi.k8s.io/devices` annotation 选择特定 GPU。

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

## Next Steps

1. 在昇腾节点上验证 CDI spec 生成的正确性
2. 确认 containerd 版本和 CDI 配置
3. 测试 CDI 原生注入路径 (无需 runtimeClassName)
4. 如果 CDI 路径稳定，考虑删除 runtime shim 和 hook (阶段 2)
5. 考虑 DRA driver 实现 (阶段 3，需要 K8s 1.35+)
