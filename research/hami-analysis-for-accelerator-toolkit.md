# HAMi 与 accelerator-toolkit 对比分析

> 分析日期：2026-05-18
>
> 分析对象：[HAMi](https://github.com/Project-HAMi/HAMi) / [HAMi-DRA](https://github.com/Project-HAMi/HAMi-DRA)
>
> 分析目的：评估 HAMi 对 accelerator-toolkit DRA 实现的帮助、可借鉴的架构设计、以及改进方向

---

## 一、项目定位对比

| 维度 | HAMi | accelerator-toolkit |
|------|------|---------------------|
| **核心目标** | GPU 虚拟化 + 共享 + 异构调度 | 驱动层注入（不涉及虚拟化） |
| **解决的问题** | 一块 GPU 拆成多份给多个 Pod 用 | 容器里缺少宿主机驱动库和设备节点 |
| **技术路径** | LD_PRELOAD 拦截 CUDA/NVML 调用 | CDI spec + containerd 原生注入 |
| **DRA 定位** | 传统 Device Plugin 到 DRA 的兼容桥（HAMi-DRA 子项目） | 原生 DRA driver |
| **厂商覆盖** | 13 家（NVIDIA、Iluvatar、MetaX、Ascend、Cambricon 等） | 4 家（Iluvatar、Ascend、MetaX、NVIDIA） |
| **K8s 组件** | Mutating Webhook + Scheduler Extender + Device Plugin + libvgpu | 单一 DRA Driver DaemonSet |
| **CNCF 状态** | CNCF Sandbox 项目 | 独立项目 |

两者解决的是不同层面的问题，但有互补性：HAMi 做 GPU 虚拟化和共享，accelerator-toolkit 做驱动层注入。

---

## 二、HAMi 项目架构概览

### 2.1 四大组件

```
Pod 提交
  → HAMi Mutating Webhook（注入 GPU 注解，设置调度器）
  → HAMi Scheduler（Filter / Score / Bind 设备感知调度）
  → Device Plugin Allocate()（CDI spec、卷挂载、环境变量）
  → 容器内 libvgpu.so 预加载（LD_PRELOAD 拦截 CUDA 调用）
  → Monitor 与 Metrics
```

- **Mutating Webhook**（`pkg/scheduler/webhook.go`）：拦截 Pod 提交，遍历注册的 device plugin，调用每个设备的 `MutateAdmission()` 注入资源请求/注解
- **Scheduler Extender**（`pkg/scheduler/scheduler.go`）：实现 Filter/Score/Bind 管线，支持 binpack/spread 策略和 NVLink 拓扑感知调度
- **Device Plugin**（`cmd/device-plugin/nvidia`）：实现 K8s device plugin gRPC 接口（`ListAndWatch`、`Allocate`、`PreStartContainer`）
- **容器内虚拟化组件**：`libvgpu.so`（HAMi-core 构建），通过 LD_PRELOAD 注入容器

### 2.2 异构设备接口

所有厂商实现统一的 `Devices` 接口（`pkg/device/devices.go`）：

```go
type Devices interface {
    CommonWord() string
    MutateAdmission(*corev1.Pod) (*corev1.Pod, error)
    CheckHealth(*corev1.Pod, devicecorev1.Device) bool
    NodeCleanUp(string) error
    GetResourceNames() []string
    GetNodeDevices(*corev1.Node) ([]*devicecorev1.DeviceInfo, error)
    LockNode(string) bool
    ReleaseNodeLock(string)
    GenerateResourceRequests(*corev1.Container) map[string]string
    PatchAnnotations(*corev1.Pod, string, map[string]string) (*corev1.Pod, error)
    ScoreNode(*corev1.Node, *corev1.Pod) float32
    AddResourceUsage(*corev1.Pod, map[string]string)
    Fit(*corev1.Pod, *devicecorev1.Device) bool
}
```

每个厂商在 `pkg/device/<vendor>/` 下独立实现该接口，新厂商只需新建子目录。

### 2.3 支持的厂商列表

| 厂商 | 目录 | 设备类型 | 显存隔离 | 核心隔离 | 多卡互联 |
|------|------|----------|---------|---------|---------|
| NVIDIA | `nvidia/` | GPU | 是 | 是 | **是（NVLink）** |
| Iluvatar | `iluvatar/` | GPU | 是 | 是 | 否 |
| MetaX | `metax/` | GPU | 是 | 是 | 否 |
| Mthreads | `mthreads/` | GPU | 是 | 是 | 否 |
| Vastai | `vastai/` | GPU | 是 | 是 | 否 |
| Cambricon | `cambricon/` | MLU | 是 | 是 | 否 |
| Hygon | `hygon/` | DCU | 是 | 是 | 否 |
| Huawei Ascend | `ascend/` | NPU | 是 | 是 | 否 |
| Enflame | `enflame/` | GCU | 是 | 是 | 否 |
| Kunlunxin | `kunlun/` | XPU | 是 | 是 | 否 |
| AWS Neuron | `awsneuron/` | N/A | N/A | N/A | 否 |

---

## 三、HAMi-DRA 子项目分析

HAMi 的 DRA 支持通过独立子项目 [HAMi-DRA](https://github.com/Project-HAMi/HAMi-DRA) 实现，采用**兼容桥模式**。

### 3.1 工作原理

HAMi-DRA 是一个 Mutating Webhook，自动将传统 Device Plugin 风格的资源请求转换为 DRA `ResourceClaim`：

```
用户 Pod 声明 nvidia.com/gpu: 1, nvidia.com/gpumem: 3000
  → HAMi-DRA Webhook 拦截
  → 自动创建 ResourceClaim（含 CEL 选择器）
  → 从 container limits 中移除原始 GPU 资源
  → 将 ResourceClaim 绑定到容器
  → DRA allocator 分配设备
  → kubelet 调 DRA driver 准备设备
```

### 3.2 CEL 选择器生成逻辑

`pkg/webhook/dra/mutating.go` 中的关键步骤：

1. 检测容器是否有 `nvidia.com/gpu` 资源限制
2. 创建 `ResourceClaim`，使用 `DeviceAllocationModeExactCount`
3. 添加 CEL 选择器：`device.attributes[driver].type == "nvidia"`
4. 若请求了 GPU 核心，添加 `capacity["cores"]` 要求
5. 若请求了显存，转换为字节并添加 `capacity["memory"]`
6. 检查 Pod 注解中的 UUID/productName 选择器，添加对应 CEL 表达式
7. 通过 K8s API 创建 ResourceClaim
8. 从 container limits 移除原始 GPU 资源，绑定 ResourceClaim

### 3.3 与主 HAMi 的集成

在 HAMi Helm chart 中（`charts/hami/values.yaml`），DRA 默认关闭（`dra.enabled: false`）。启用 DRA 时，Scheduler Extender **不安装** —— DRA 替代了传统调度器扩展路径。

### 3.4 前置条件

- Kubernetes >= 1.34，需开启 DRA Consumable Capacity feature gate
- containerd 或 CRI-O 启用 CDI 支持
- cert-manager（Webhook TLS 证书）
- NVIDIA GPU Driver >= 440

---

## 四、HAMi-DRA 与 accelerator-toolkit DRA 对比

| 方面 | HAMi-DRA | accelerator-toolkit |
|------|----------|---------------------|
| **架构** | 兼容桥（Webhook 自动转换） | 原生 DRA driver |
| **用户入口** | 传统 `nvidia.com/gpu` 资源声明 | 直接写 ResourceClaimTemplate |
| **组件数** | Webhook + DRA driver | 单一 DRA driver |
| **K8s 版本** | >= 1.34（Consumable Capacity） | >= 1.31（DRA beta） |
| **迁移成本** | 零改动（对已有用户） | 需要改 Pod spec |
| **架构复杂度** | 高（Webhook + 转换逻辑 + 验证） | 低 |
| **标准性** | 依赖 Webhook 转换层 | 纯 K8s 标准路径 |

**结论**：对于新建集群，accelerator-toolkit 的原生 DRA 路径更简洁。如果需要兼容已有 Device Plugin 用户，可参考 HAMi-DRA 的 Webhook 转换模式。

---

## 五、HAMi 对 accelerator-toolkit 的具体帮助

### 5.1 Iluvatar 设备适配经验

HAMi 已实现 `pkg/device/iluvatar/device.go`，覆盖四个产品线：

| 产品线 | 资源名 |
|--------|--------|
| BI-V100 | `iluvatar.ai/BI-V100-vgpu`、`iluvatar.ai/BI-V100.vCore`、`iluvatar.ai/BI-V100.vMem` |
| BI-V150 | `iluvatar.ai/BI-V150-vgpu`、`iluvatar.ai/BI-V150.vCore`、`iluvatar.ai/BI-V150.vMem` |
| MR-V100 | `iluvatar.ai/MR-V100-vgpu`、`iluvatar.ai/MR-V100.vCore`、`iluvatar.ai/MR-V100.vMem` |
| MR-V50 | `iluvatar.ai/MR-V50-vgpu`、`iluvatar.ai/MR-V50.vCore`、`iluvatar.ai/MR-V50.vMem` |

本项目当前只覆盖 BI-V150。HAMi 的多型号适配逻辑可直接参考。

### 5.2 MetaX 深度实现

HAMi 的 `pkg/device/metax/` 有 10 个文件，是非 NVIDIA 厂商中实现最完整的：
- 配置管理、协议处理、QoS 控制
- 评分逻辑、共享设备支持
- 拓扑感知调度（`metaxsGPUTopologyAware`）
- 资源名：`metax-tech.com/sgpu`、`metax-tech.com/vcore`、`metax-tech.com/vmemory`

本项目有 MetaX profile 但只做了基本设备发现，深度支持可参考 HAMi 的实现。

### 5.3 多厂商设备接口设计

HAMi 的 `Devices` 接口覆盖完整生命周期。本项目的 `device.Device` 只是纯数据结构（`Path`、`Index`、`UUID`），没有行为方法。如果未来要支持更多差异化逻辑，HAMi 的接口设计值得参考。

---

## 六、本项目相比 HAMi 的优势

| 方面 | 本项目优势 |
|------|-----------|
| **架构简洁** | 单一二进制，没有 Webhook、Scheduler Extender、多组件协调的复杂度 |
| **原生 DRA** | 直接走 K8s 标准 DRA 路径，不依赖 Webhook 转换层 |
| **CDI 标准化** | 生成标准 CDI 1.1.0 spec，containerd 原生消费 |
| **Profile 驱动** | 一个 YAML 定义厂商全部差异，核心代码无需改动 |
| **无侵入** | 不需要 LD_PRELOAD、不需要修改容器镜像、不需要注入虚拟化库 |
| **K8s 版本要求低** | DRA beta 从 1.31 开始，HAMi-DRA 要求 1.34+ |

---

## 七、HAMi DRA 中值得学习的技术点

### 7.1 容量属性上报

HAMi-DRA 在 ResourceClaim 中使用 `capacity` 字段表达显存和核心数：

```yaml
# HAMi-DRA 生成的容量要求
device.capacity["cores"].value >= 50
device.capacity["memory"].value >= 3000000000
```

本项目的 ResourceSlice 只上报 `vendor`、`model`、`uuid`、`path`，没有 `capacity`。若要支持"需要至少 16GB 显存的 Iluvatar GPU"这类匹配，需要扩展 capacity 字段。

### 7.2 DeviceClass 自动生成

HAMi-DRA 的 Helm chart 自动创建 DeviceClass。本项目的 `accelerator-profile-render` 能渲染 `RuntimeClass`、`DaemonSet`、`RBAC`，但**不渲染 DeviceClass 和 ResourceClaimTemplate**，用户需手动创建。

### 7.3 Volcano 集成

HAMi-DRA 有 `pkg/webhook/volcano/` 支持 Volcano 批调度器的 DRA 集成。对于 AI 训练场景，Volcano 是常见选择，本项目未考虑此集成点。

### 7.4 设备健康检查

HAMi 的 `Devices` 接口有 `CheckHealth()` 方法，设备插件定期上报健康状态。本项目的 DRA driver 没有实现设备健康上报，ResourceSlice 中无 health 信息。

### 7.5 CDI 的 LD_PRELOAD 模式

HAMi 使用 `ld.so.preload` 注入 `libvgpu.so`，本项目使用 `ldconfig` 刷新动态链接器缓存。两种方式目标不同（HAMi 需要拦截 API 调用，本项目只需让链接器找到驱动库），但 HAMi 的 `ld.so.preload` 写入方式在某些场景下比 `ldconfig` 更可靠，可作为备选方案。

---

## 八、可落地的改进建议

基于 HAMi 的经验，按优先级排列：

### P0：补齐 DRA 资源清单

- 在 `accelerator-profile-render` 中增加 `deviceclass` 和 `resourceclaimtemplate` 子命令
- 基于 profile 的 `kubernetes.resourceNames` 自动生成 DeviceClass YAML
- 提供 ResourceClaimTemplate 示例，降低用户使用门槛

### P1：扩展 ResourceSlice 属性

- 在 `BuildDriverResources` 中增加 `capacity` 字段（如显存大小）
- 从 `ixsmi` 或类似工具获取设备容量信息
- 支持 CEL 选择器按容量匹配

### P1：UnprepareResourceClaims 清理 CDI spec

- 当前 `UnprepareResourceClaims` 只清理内存 map，不删 `/etc/cdi/` 下的文件
- 应删除对应的 CDI spec 文件，避免磁盘泄漏

### P2：实现设备健康检查

- 周期性调用 `ixsmi` 检查设备状态
- 在 ResourceSlice 中上报 health 信息
- kubelet 自动跳过不健康设备

### P2：ResourceSlice 动态刷新

- 当前只在启动时 publish 一次
- 应监听设备变化（热插拔、驱动重启）并更新 ResourceSlice
- 可用 `--resync-period` 定期刷新，或监听 udev 事件

### P3：Volcano 集成评估

- 评估是否需要支持 Volcano 批调度器
- 若需要，参考 HAMi-DRA 的 `pkg/webhook/volcano/` 实现

---

## 九、总结

HAMi 和 accelerator-toolkit 是**互补关系**：

- **HAMi**：GPU 虚拟化 + 共享 + 异构调度，适合需要 GPU 切分、多租户共享的场景
- **accelerator-toolkit**：驱动层注入，适合不需要虚拟化、只需要让容器能访问 GPU 的场景

HAMi 对本项目最大的价值：

1. **设备适配经验**：Iluvatar/Ascend/MetaX 的设备发现、UUID 映射、健康检查逻辑可直接参考
2. **DRA 完整度**：DeviceClass 自动生成、容量属性上报、健康检查是本项目明确缺失的能力
3. **接口设计**：多厂商统一接口 `Devices` 的设计模式可借鉴到未来的扩展需求
4. **兼容路径**：HAMi-DRA 的 Webhook 转换模式可作为未来兼容 Device Plugin 用户的可选路径

本项目的原生 DRA + CDI 架构比 HAMi-DRA 的兼容桥模式更简洁，适合作为新集群的标准方案。核心差距在于 DRA 资源清单的完整度和运行时健壮性。

---

## 参考链接

- HAMi 主仓库：https://github.com/Project-HAMi/HAMi
- HAMi-DRA 子项目：https://github.com/Project-HAMi/HAMi-DRA
- HAMi-core 虚拟化库：https://github.com/Project-HAMi/HAMi-core
- HAMi 官网：https://project-hami.io
- HAMi 支持设备列表：https://project-hami.io/docs/userguide/device-supported
- HAMi v2.9 Roadmap（Issue #1615）：https://github.com/Project-HAMi/HAMi/issues/1615
- HAMi-DRA v0.2.0 发布（PR #1845）：https://github.com/Project-HAMi/HAMi/pull/1845
