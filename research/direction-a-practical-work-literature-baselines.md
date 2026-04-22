# 方向 A 调研：实际工作成果、参考文献与 Baseline

> 日期：2026-04-22
> 范围：结合 `research/heterogeneous-gpu-cluster-directions.md` 中“胖镜像 vs toolkit 注入 vs lazy image 的系统比较”，以及 `research/thesis-image-build-distribution-and-toolkit-brainstorm.md` 中“层次化镜像家族构建与跨镜像复用机制”。

## 1. 建议把方向 A 收敛成的问题

可表述为：

**面向异构 GPU/NPU 集群，如何在不牺牲可复现性和兼容性的前提下，将硬件相关运行环境从用户镜像中解耦，并通过镜像分层、节点注入和按需加载降低镜像构建、分发与冷启动成本？**

这比单纯比较“胖镜像 vs lazy pull”更贴合当前仓库，因为 `ix-container-toolkit` 已经有：

- Iluvatar runtime/hook 注入链路。
- `metadata/runtime/kubernetes/device/inject` 五段式 profile schema。
- Iluvatar BI-V150 与 Ascend 910B profile 样例。
- 设备节点、驱动库、工具目录、linker 配置和 extra env 注入模型。

因此这个方向的可量化贡献不应只写成“镜像变小”，而应写成：

- 用户镜像与厂商驱动/runtime 解耦。
- 多厂商 profile 降低重复适配成本。
- 冷启动和镜像分发成本下降。
- 驱动升级时减少镜像重建与回归验证范围。
- lazy image 与 toolkit 注入可以组合，进一步减少首启数据读取。

## 2. 当前可参考的实际工作成果

### 2.1 设备与运行时注入

**NVIDIA Container Toolkit / GPU Operator / CDI**

NVIDIA 是最重要的工程参照。它从 legacy `nvidia` runtime 逐渐走向 CDI/NRI。NVIDIA GPU Operator 文档显示，GPU Operator v25.10.0 起 CDI 默认用于 Kubernetes 中的 GPU 注入，containerd / CRI-O 通过 CDI 把 GPU 注入 workload container。

参考价值：

- 证明“驱动和设备由节点侧注入，而不是全部塞进业务镜像”是主流工程路线。
- 可作为 vendor-specific toolkit baseline。
- CDI 可作为你后续 profile 生成目标之一。

参考：

- https://docs.nvidia.com/datacenter/cloud-native/gpu-operator/latest/cdi.html
- https://docs.docker.com/build/building/cdi/
- https://github.com/NVIDIA/k8s-device-plugin

**Kubernetes Device Plugin / DRA**

Kubernetes Device Plugin 已是 stable，用于让厂商以 DaemonSet 形式上报 GPU/NIC/FPGA 等资源。DRA 在 Kubernetes v1.35 为 stable，进一步把设备选择、共享和属性过滤结构化。

参考价值：

- Device Plugin 是资源发现与调度层 baseline。
- DRA 是后续“环境 profile + 设备属性 + 调度约束”更强版本的参照。
- 你的工作可以定位为 device allocation 之后的 runtime environment materialization。

参考：

- https://kubernetes.io/docs/concepts/extend-kubernetes/compute-storage-net/device-plugins/
- https://kubernetes.io/docs/concepts/scheduling-eviction/dynamic-resource-allocation/

**AMD ROCm k8s-device-plugin**

AMD 的 Kubernetes device plugin 可作为第二厂商工程样例，用于证明多厂商 GPU 生态普遍采用“节点安装驱动 + Kubernetes device plugin 暴露资源”的模式。

参考：

- https://github.com/ROCm/k8s-device-plugin

### 2.2 Lazy image / on-demand loading

**Slacker, FAST 2016**

这是该方向的经典论文。它提出 HelloBench，并发现镜像拉取占容器启动时间大头，而启动只读取少量数据；由此提出 lazy fetching 的存储驱动。

参考价值：

- 可作为论文动机和指标设计来源。
- 可借鉴 HelloBench 的“从部署开始到容器可用”的度量方法。

参考：

- https://www.usenix.org/conference/fast16/technical-sessions/presentation/harter

**stargz-snapshotter / eStargz**

containerd 的 stargz-snapshotter 是最直接的 lazy pull 工程 baseline。它允许容器不等待完整镜像拉取完成，而是按需获取必要 chunk。BuildKit 支持构建 eStargz 镜像。

参考价值：

- 作为 lazy image baseline。
- 可以和“胖镜像 + overlayfs”比较冷启动。
- 可以和“toolkit 注入 + eStargz”组合，验证两者是否互补。

参考：

- https://github.com/containerd/stargz-snapshotter

**Nydus / Dragonfly image service**

Nydus 使用 RAFS/content-addressable filesystem，目标是提升启动速度、空间效率、网络效率和数据完整性。它也强调 chunk 级复用、按需下载、镜像完整性校验。

参考价值：

- 更适合大规模生产场景的 lazy image baseline。
- 和 Dragonfly 组合时，可以覆盖“lazy + P2P 分发”。
- 对你的“跨镜像复用”论证很有帮助，因为 Nydus 明确支持 chunk 级去重。

参考：

- https://github.com/dragonflyoss/nydus
- https://www.cncf.io/blog/2020/10/20/introducing-nydus-dragonfly-container-image-service/

**AWS SOCI / soci-snapshotter**

SOCI 的特点是不改原镜像 digest，而是生成独立 SOCI index 作为 OCI artifact，适合强调供应链签名、镜像不可变和生产兼容性。

参考价值：

- 作为“无需转换原镜像”的 lazy baseline。
- 可用于讨论 eStargz/Nydus 改格式带来的兼容性和签名问题。

参考：

- https://aws.amazon.com/about-aws/whats-new/2022/09/introducing-seekable-oci-lazy-loading-container-images/

**AWS Lambda on-demand container loading, USENIX ATC 2023**

这是工业级 on-demand loading 论文，场景是 Lambda 支持最大 10GiB 容器镜像，并在大规模、高并发、低延迟条件下做按需加载、缓存、去重、加密和纠删码。

参考价值：

- 可作为“按需加载不是玩具优化，而是云厂商生产系统”的强证据。
- 适合写相关工作中 serverless/container image cold start 部分。

参考：

- https://www.usenix.org/conference/atc23/presentation/brooker
- https://www.amazon.science/publications/on-demand-container-loading-in-aws-lambda

### 2.3 镜像分发、预热和层复用

**Dragonfly**

Dragonfly 是 CNCF Graduated 项目，用 P2P 做镜像、OCI artifact、AI model 等大对象分发。它适合作为大规模分发 baseline，而不是单机 lazy pull baseline。

参考价值：

- 用于“registry/source bandwidth pressure”指标。
- 可作为全量镜像分发加速 baseline。
- 可和 Nydus 组合成为“P2P + lazy”强 baseline。

参考：

- https://www.cncf.io/projects/dragonfly/
- https://www.cncf.io/blog/2022/11/21/dragonfly-integrates-nydus-for-image-acceleration-practice/

**Kraken**

Uber 的 P2P Docker registry，生产场景中面向大规模 Docker image blob 分发。

参考价值：

- 适合证明“镜像分发本身在大规模集群中是实际瓶颈”。
- 可作为 Dragonfly 之外的工业系统相关工作。

参考：

- https://www.uber.com/blog/introducing-kraken/
- https://github.com/uber/kraken

**OpenKruise ImagePullJob**

OpenKruise 提供 ImagePullJob / ImageListPullJob，用 CRD 声明哪些镜像要预下载到哪些节点。

参考价值：

- 作为“静态预热” baseline。
- 可和 lazy pull 比较：预热降低首启，但增加无效下载和磁盘占用；lazy 降低首启下载，但可能带来运行期 page/chunk miss。

参考：

- https://openkruise.io/docs/user-manuals/imagepulljob

**BuildKit registry cache / 多阶段构建 / distroless / SlimToolkit**

这些不是运行时注入方案，但适合作为“镜像构建优化”和“用户手工瘦身”baseline。

参考价值：

- 证明单纯要求用户优化 Dockerfile 不能解决异构 GPU 驱动耦合问题。
- 可量化：build time、cache hit rate、最终镜像大小、CVE 数量、重建频率。

参考：

- https://docs.docker.com/build/cache/backends/
- https://docs.docker.com/get-started/docker-concepts/building-images/multi-stage-builds/
- https://github.com/GoogleContainerTools/distroless
- https://github.com/slimtoolkit/slim

### 2.4 调度与缓存感知

**Dependency Scheduling, HotEdge 2020**

该论文让调度器考虑任务依赖在节点上的本地缓存情况，在 Kubernetes 中实现并评估，报告典型场景下启动时延提升 1.4-2.3x。

参考价值：

- 可作为“镜像层/文件/依赖缓存感知调度”的论文 baseline。
- 如果你后续扩展方向 A 到“调度器优先选择已有 toolkit/profile/cache 的节点”，这篇非常贴近。

参考：

- https://www.usenix.org/conference/hotedge20/presentation/fu

## 3. 建议实验 baseline

建议不要只做两个 baseline。至少分成 8 组，能更清楚地区分每个因素的贡献。

| 组别 | Baseline / 方案 | 目的 |
|---|---|---|
| B0 | 原生 Kubernetes + containerd overlayfs + 胖镜像 | 生产中最朴素方案；所有 driver/runtime/framework 都在镜像内 |
| B1 | 胖镜像 + OpenKruise ImagePullJob 预热 | 衡量静态预热能解决多少冷启动 |
| B2 | 胖镜像 + Dragonfly/Kraken P2P 分发 | 衡量大规模并发拉取下 source/registry 压力 |
| B3 | 胖镜像 + stargz/Nydus/SOCI lazy pull | 衡量 lazy image 对大镜像冷启动的收益 |
| B4 | 轻业务镜像 + 当前 toolkit 注入 | 衡量驱动/runtime 外置后镜像大小、构建和冷启动收益 |
| B5 | 轻业务镜像 + toolkit 注入 + lazy pull | 验证注入和 lazy 是否互补 |
| B6 | 手工多阶段构建 / distroless / SlimToolkit | 用户侧镜像瘦身 baseline，说明它不能消除厂商驱动耦合 |
| B7 | NVIDIA Container Toolkit / CDI 或厂商官方 runtime | 工业主流注入方案 baseline |

如果资源有限，最小闭环可以只保留：

- B0 胖镜像。
- B3 胖镜像 + lazy pull。
- B4 toolkit 注入。
- B5 toolkit 注入 + lazy pull。
- B7 vendor toolkit/CDI。

## 4. 建议量化指标

### 4.1 启动性能

- `T_pull`: kubelet/containerd 开始拉取到镜像 ready。
- `T_create`: CRI 创建 container 到 OCI create 完成。
- `T_hook`: hook 执行耗时，拆分设备、库、工具、ldconfig。
- `T_ready`: Pod 创建到 Ready。
- `T_first_useful_work`: Pod 创建到训练/推理程序完成第一步有效计算。
- warm start 与 cold start 分开统计。

### 4.2 分发和存储成本

- 每次启动实际网络下载字节数。
- registry/source 回源流量。
- 节点本地 image/snapshot/cache 占用。
- 跨任务层复用率：`reused_layer_bytes / total_required_layer_bytes`。
- lazy chunk 命中率：`local_chunk_hits / total_chunk_reads`。
- 无效预热率：预热但未被任务使用的 bytes。

### 4.3 镜像构建和维护成本

- 单个 workload 需要维护的镜像数量。
- 驱动升级后需要重建的业务镜像数量。
- 镜像构建耗时和 BuildKit cache hit rate。
- 镜像推送字节数。
- Dockerfile 复杂度：行数、厂商相关命令数、厂商相关路径数。

### 4.4 异构兼容性和可运维性

- 同一业务镜像可运行的 accelerator profile 数量。
- 新增一个厂商 profile 需要改动的代码行数。
- profile 渲染/校验失败率。
- 设备选择语义覆盖：`all/none/index/uuid/MIG-like partition` 等。
- 注入完整性：设备节点、so、工具、env、linker 是否一致。

### 4.5 安全和供应链

- 镜像内特权库/工具数量。
- CVE 数量和扫描噪声。
- 镜像 digest/signature 是否因 lazy 格式转换改变。
- hook/CDI/NRI 注入带来的特权面：host mount 数量、可执行 hook 数量、privileged DaemonSet 数量。

## 5. 推荐 workload

至少准备三类 workload，避免结果只对 hello-world 有意义。

| 类别 | 示例 | 价值 |
|---|---|---|
| 小启动集 | `python -c 'import torch'`、`nvidia-smi/ixsmi/ascend-smi` 类检查 | 测 hook 和基础 runtime 可用时间 |
| 推理集 | ResNet/BERT/LLM tokenizer + 首次推理 | 测首次有用工作时间 |
| 训练集 | 单卡训练 10-100 step，小模型即可 | 测 lazy pull 是否带来运行期抖动 |

异构侧建议至少覆盖：

- Iluvatar BI-V150：已有 profile。
- Ascend 910B：已有 profile。
- NVIDIA 或 AMD：作为业界 baseline，哪怕只做文献/公开工程对照也有价值。

## 6. 论文写作中的定位

可以这样区分你的工作和已有工作：

- Slacker/stargz/Nydus/SOCI 解决“镜像数据如何按需取”。
- Dragonfly/Kraken/OpenKruise 解决“镜像如何预热或大规模分发”。
- NVIDIA Toolkit/CDI/DRA 解决“设备如何暴露给容器”。
- BuildKit/distroless/SlimToolkit 解决“镜像如何构建得更小”。
- 你的系统解决“异构 accelerator 运行环境如何从业务镜像中声明式解耦，并与镜像分层/lazy 分发共同优化冷启动、复用和维护成本”。

更强的贡献表述：

1. 设计 profile 抽象，把设备选择、驱动库、工具目录、linker、extra env 统一建模。
2. 实现 profile-driven runtime/hook，将厂商运行时从用户镜像迁移到节点注入层。
3. 系统比较 fat image、toolkit injection、lazy image 以及组合方案在异构 GPU/NPU 集群中的成本。
4. 给出可复现实验方法，量化镜像大小、启动时间、网络流量、磁盘占用、复用率和维护成本。

## 7. 最建议优先做的最小实验闭环

第一阶段只做 4 组：

| 组别 | 镜像形态 | 运行时形态 | snapshotter |
|---|---|---|---|
| E1 | 胖镜像 | 默认 runc | overlayfs |
| E2 | 胖镜像 | 默认 runc | stargz 或 Nydus |
| E3 | 轻业务镜像 | `ix-container-toolkit` 注入 | overlayfs |
| E4 | 轻业务镜像 | `ix-container-toolkit` 注入 | stargz 或 Nydus |

每组跑：

- 冷节点首次启动。
- 同节点第二次启动。
- 多 Pod 并发启动。
- 驱动/profile 更新后的镜像重建数量和耗时。

这一组实验能直接回答方向 A 的核心问题：**把厂商运行环境从镜像迁到节点注入层，到底减少了多少分发、存储、构建和启动成本；lazy image 在这个基础上是否还能继续带来收益。**
