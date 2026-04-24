# NVIDIA xpu-toolkit 接入调研

> 更新日期：2026-04-24

## 结论

NVIDIA 可以接入 `xpu-runtime`，但不建议直接复制 Ascend/Metax 的注入方式。当前集群已经有成熟的 NVIDIA GPU Operator 链路，最小风险路径是先支持一个 NVIDIA profile 验证 `xpu-runtime` 能消费 `NVIDIA_VISIBLE_DEVICES`，再决定是否让 xpu-runtime 统一替代 `RuntimeClass nvidia`。

当前集群的主要阻塞点是：NVIDIA device plugin 实际使用 `DEVICE_LIST_STRATEGY=volume-mounts`，workload 中 `NVIDIA_VISIBLE_DEVICES=/var/run/nvidia-container-devices`，不是 UUID 或 index 列表。现有 `pkg/device` 只能解析 `all`、`none`、index-list、UUID-list，不能直接解析这个 volume-mount selector。

## 官方事实

NVIDIA Container Toolkit 使用 OCI env 控制设备枚举和驱动能力：

- `NVIDIA_VISIBLE_DEVICES` 支持 index list、GPU UUID list、`all`、`none`、empty/void。
- `NVIDIA_DRIVER_CAPABILITIES` 支持 `compute`、`utility`、`graphics`、`video`、`display`、`compat32` 等能力，unset 时默认 `utility,compute`。

NVIDIA Kubernetes device plugin 默认资源类型是 `nvidia.com/gpu`，但支持自定义资源名和多种设备列表传递策略：

- `envvar`：通过 `NVIDIA_VISIBLE_DEVICES` 传递设备列表。
- `volume-mounts`：通过 volume mounts 传递设备列表，仍由 NVIDIA runtime 解释。
- `cdi-annotations` / `cdi-cri`：通过 CDI 传递设备。

GPU Operator 新版本正在向 CDI/NRI 演进。官方文档说明，GPU Operator v25.10.0 起 CDI 默认用于 Kubernetes workload 的 GPU 注入；当前集群是 v25.3.1，ClusterPolicy 中 `cdi.enabled=false`。

参考：

- NVIDIA Container Toolkit specialized configurations: https://docs.nvidia.com/datacenter/cloud-native/container-toolkit/1.17.8/docker-specialized.html
- NVIDIA k8s-device-plugin README: https://github.com/NVIDIA/k8s-device-plugin
- NVIDIA GPU Operator CDI/NRI support: https://docs.nvidia.com/datacenter/cloud-native/gpu-operator/latest/cdi.html

## 当前集群事实

已看到的 NVIDIA 节点：

- `dell-61`: Tesla P100
- `dell-62`: RTX 2080 Ti
- `dell-63`, `dell-70`: Quadro RTX 6000
- `dell-66`, `dell-67`, `dell-68`, `inspur-gpu-10`: V100
- `inspur-gpu-04`: A100 PCIe 40GB

`RuntimeClass`：

- 已存在 `nvidia`
- 已存在 `xpu-runtime`

GPU Operator：

- namespace: `nvidia-gpu-operator`
- `nvidia-container-toolkit-daemonset` 正常运行
- `nvidia-device-plugin-daemonset` 正常运行
- `gpu-feature-discovery`、`dcgm-exporter` 正常运行
- ClusterPolicy: `cdi.enabled=false`

device plugin 当前实际 env：

```text
DEVICE_LIST_STRATEGY=volume-mounts
DEVICE_ID_STRATEGY=uuid
PASS_DEVICE_SPECS=true
MIG_STRATEGY=single
```

ConfigMap 中定义了按型号改名的资源：

```text
nvidia.com/a100
nvidia.com/v100
nvidia.com/p100
nvidia.com/rtx6000
nvidia.com/rtx2080
nvidia.com/t4
```

以 `inspur-gpu-04` 为例：

```text
Capacity:
  nvidia.com/a100: 4
  nvidia.com/gpu: 0
```

节点标签样例：

```text
nvidia.com/gpu.present=true
nvidia.com/gpu.product=NVIDIA-A100-PCIE-40GB-SHARED
nvidia.com/gpu.count=4
nvidia.com/gpu.family=ampere
nvidia.com/gpu.compute.major=8
nvidia.com/gpu.compute.minor=0
nvidia.com/gpu.sharing-strategy=time-slicing
nvidia.com/gpu.replicas=2
nvidia.com/mig.capable=true
nvidia.com/mig.strategy=single
```

## Host 事实

在 `inspur-gpu-04` 上通过 `nvidia-container-toolkit-daemonset` 观察：

设备节点：

```text
/dev/nvidia0
/dev/nvidia1
/dev/nvidia2
/dev/nvidia3
/dev/nvidiactl
/dev/nvidia-uvm
/dev/nvidia-uvm-tools
/dev/nvidia-modeset
/dev/nvidia-caps/nvidia-cap1
/dev/nvidia-caps/nvidia-cap2
```

驱动库：

```text
/usr/lib/x86_64-linux-gnu/libcuda.so*
/usr/lib/x86_64-linux-gnu/libnvidia-ml.so*
/usr/lib/x86_64-linux-gnu/libnvidia-ptxjitcompiler.so*
```

工具：

```text
/usr/bin/nvidia-smi
```

UUID 映射命令可用：

```bash
nvidia-smi --query-gpu=index,uuid,name --format=csv
```

样例输出：

```text
index, uuid, name
0, GPU-09ab6992-d722-c828-7bca-934ccb46aa54, NVIDIA A100-PCIE-40GB
1, GPU-c9db5c06-61c6-01b7-b9aa-20307600482c, NVIDIA A100-PCIE-40GB
2, GPU-4ce9cbee-8444-67c3-2ead-dfda2618ccdf, NVIDIA A100-PCIE-40GB
3, GPU-08efa9c6-c2b6-09d7-95ad-8b214dc49ab2, NVIDIA A100-PCIE-40GB
```

## Workload 查询结果

实验清单：

```text
experiments/nvidia-a100/k8s/query-nvidia-runtime-job.yaml
```

运行配置：

- node: `inspur-gpu-04`
- `runtimeClassName: nvidia`
- resource: `nvidia.com/a100: 1`
- image: `crater-harbor.act.buaa.edu.cn/nvcr.io/nvidia/cuda:12.6.2-base-ubi9`

结果：

- Job `crater-workspace/nvidia-a100-runtime-query` Completed
- 容器 env:

```text
NVIDIA_VISIBLE_DEVICES=/var/run/nvidia-container-devices
NVIDIA_DRIVER_CAPABILITIES=compute,utility
LD_LIBRARY_PATH=/usr/local/nvidia/lib:/usr/local/nvidia/lib64
```

- 容器只看到分配到的 GPU 设备节点，例如：

```text
/dev/nvidia1
/dev/nvidiactl
/dev/nvidia-uvm
/dev/nvidia-uvm-tools
/dev/nvidia-modeset
```

- `nvidia-smi` 只看到 1 张 A100，并能输出该卡 UUID。
- mount 中出现 `/run/nvidia-container-devices/GPU-<uuid>`，说明当前 device plugin 的 `volume-mounts` 策略正在生效。

## 接入方案

### 方案 A：要求 NVIDIA device plugin 使用 `envvar`

这是最贴近当前 xpu-toolkit 设计的方案。

做法：

- 新增 `profiles/nvidia-gpu.yaml`
- `selectorEnvVars: [NVIDIA_VISIBLE_DEVICES]`
- `selectorFormats: [all, none, index-list, uuid-list]`
- `mapping.strategy.primary: command-csv-index-uuid`
- mapping command 使用 `nvidia-smi --query-gpu=index,uuid --format=csv`
- device globs 覆盖 `/dev/nvidia*`
- control device globs 覆盖 `/dev/nvidiactl`、`/dev/nvidia-uvm*`、`/dev/nvidia-modeset`、`/dev/nvidia-caps/*`
- artifacts 覆盖 NVIDIA 驱动库、`nvidia-smi` 等工具

优点：

- 代码改动小，基本复用 Iluvatar 的 UUID mapping 路径。
- 适合验证 xpu-runtime 对 NVIDIA 的基本兼容性。

问题：

- 当前集群实际强制 `DEVICE_LIST_STRATEGY=volume-mounts`，需要调整 GPU Operator/device plugin 配置或选独立测试节点。
- NVIDIA runtime 已经很成熟，xpu-toolkit 自己挂载 NVIDIA 库可能重复实现官方 toolkit 行为。

### 方案 B：支持 NVIDIA 当前的 `volume-mounts` selector

这是适配当前集群现状的方案。

需要新增能力：

- 支持 `mapping.strategy.primary: nvidia-volume-mounts`
- 当 `NVIDIA_VISIBLE_DEVICES=/var/run/nvidia-container-devices` 时，从 OCI mounts 或 rootfs 路径解析 `GPU-<uuid>` 条目。
- 用 `nvidia-smi` 把 UUID 映射回 index，再选择 `/dev/nvidiaN`。

优点：

- 不需要改 GPU Operator 现有配置。
- 能和当前集群 `PASS_DEVICE_SPECS=true` / `volume-mounts` 形态对齐。

问题：

- 这不是通用 selector env，必须让 runtime/hook 读 OCI mounts 或容器 rootfs。
- 容易和 NVIDIA runtime 的 mount 行为重叠。
- 对 MIG、time-slicing、共享资源的语义需要单独处理。

### 方案 C：xpu-runtime 包装 NVIDIA runtime，不自己注入 NVIDIA artifacts

做法：

- `xpu-runtime` shim 仍作为统一入口。
- NVIDIA profile 的 `underlyingRuntime` 指向 `nvidia-container-runtime` 或已配置好的 `nvidia` runtime binary。
- xpu-toolkit 只做统一日志、profile 选择和少量 env normalization。
- 增加 profile 级别开关，允许跳过本项目 hook/device/artifact 注入。

优点：

- 风险最低，复用 NVIDIA 官方 toolkit。
- 对 CDI/NRI 演进更友好。
- 避免重复挂载 NVIDIA 驱动库。

问题：

- 统一入口是 `xpu-runtime`，但真正 NVIDIA 注入仍由 NVIDIA runtime 完成。

## 建议路径

建议分两阶段。

第一阶段只做验证，不改集群 NVIDIA Operator 全局配置：

1. 保留当前 `RuntimeClass nvidia` 作为基线。
2. 保留 `experiments/nvidia-a100/k8s/query-nvidia-runtime-job.yaml` 作为事实查询清单。
3. 新增一个 draft `profiles/nvidia-a100.yaml`，先表达 host facts、资源名和 selector 事实。
4. 在代码上优先实现 `runtime.injectMode: delegate-only` profile 开关，让 `xpu-runtime` 能包装 NVIDIA runtime。
5. 用 `runtimeClassName: xpu-runtime` + `nvidia.com/a100: 1` 验证 `nvidia-smi`。

第二阶段再评估是否让 xpu-toolkit 自己完整注入 NVIDIA：

1. 如果必须完全摆脱 NVIDIA runtime，优先做 `envvar` 模式验证。
2. 若必须兼容当前集群默认配置，再实现 `nvidia-volume-mounts` selector parser。
3. 单独处理 MIG 和 time-slicing，不把它们混进 L3 smoke 的第一版闭环。

## 当前判断

可以接入，但不要从“自己挂所有 NVIDIA 库和设备”开始。NVIDIA 官方链路已经存在，当前最稳妥的接入定义是：

```text
统一 RuntimeClass 入口 + profile 管理 + 委托 NVIDIA runtime 完成设备/驱动注入
```

等 `xpu-runtime` 包装路径验证通过后，再决定是否实现更重的 native NVIDIA artifact 注入。

## 当前落地状态

已新增 draft profile：

```text
profiles/nvidia-a100.yaml
```

该 profile 使用：

```yaml
runtime:
  underlyingRuntime: /usr/local/nvidia/toolkit/nvidia-container-runtime
  injectMode: delegate-only
```

这表示 `xpu-runtime` 只作为统一入口包装 NVIDIA GPU Operator 安装的 container runtime wrapper，不修改 OCI spec，也不运行本项目 hook。

## xpu-runtime delegate 验证

验证时间：2026-04-24

installer 镜像：

```text
crater-harbor.act.buaa.edu.cn/xpu-huangsy/accelerator-toolkit-installer:nvidia-a100-delegate-wrapper-20260424
```

installer digest：

```text
sha256:1a80ffd41ab11e23d023a846bd117fb60a1b2ec3fdfa0daa7fba6d9286eaf0e4
```

安装节点：

```text
inspur-gpu-04
```

验证 Job：

```text
crater-workspace/nvidia-a100-xpu-runtime-query
experiments/nvidia-a100/k8s/query-xpu-runtime-job.yaml
```

结果：

- `runtimeClassName: xpu-runtime`
- resource: `nvidia.com/a100: 1`
- Job `Completed`
- 容器内 `NVIDIA_VISIBLE_DEVICES=/var/run/nvidia-container-devices`
- 容器内 `LD_LIBRARY_PATH=/usr/local/nvidia/lib:/usr/local/nvidia/lib64`
- 容器内只看到 1 个分配到的 GPU 设备节点，例如 `/dev/nvidia1`
- `nvidia-smi` 正常，只显示 1 张 `NVIDIA A100-PCIE-40GB`
- `nvidia-smi --query-gpu=index,uuid,name --format=csv` 输出 1 张卡：

```text
index, uuid, name
0, GPU-c9db5c06-61c6-01b7-b9aa-20307600482c, NVIDIA A100-PCIE-40GB
```

调试记录：

- 首次 profile 使用 `/usr/bin/nvidia-container-runtime` 失败。
- 失败原因：该路径没有复用 GPU Operator 为 `RuntimeClass nvidia` 配置的 wrapper 行为，会把 `NVIDIA_VISIBLE_DEVICES=/var/run/nvidia-container-devices` 当成 UUID/index 解析。
- 改为 `/usr/local/nvidia/toolkit/nvidia-container-runtime` 后通过。
- 首次安装后需要重启 `inspur-gpu-04` 的 containerd 才加载新写入的 `xpu-runtime` handler；后续只改 active profile/config 不需要重启。
