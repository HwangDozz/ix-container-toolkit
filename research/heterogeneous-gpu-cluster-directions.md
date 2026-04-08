# 异构 GPU 集群研究方向调研

> 更新日期：2026-04-08
> 目标：结合当前仓库中的 toolkit 工作，梳理异构 GPU 集群在镜像交付、设备抽象、运行时注入与调度方面的相关生态，并形成可执行的研究方向。

## 1. 背景

当前异构 GPU 集群面临的核心矛盾，不只是“如何把设备分给容器”，而是下面几类问题同时出现：

- 不同厂商设备的驱动、工具链、环境变量、控制设备节点差异很大
- 用户既希望镜像易用，又不希望每个业务镜像都做成几十 GB 的大镜像
- 运行时层已经不再满足于简单的 device plugin，需要支持共享、切片、拓扑、设备状态与动态配置
- 调度层不能只看“几张卡”，而需要逐步理解设备能力、拓扑、共享模式与环境准备成本

从这个角度看，异构 GPU 集群里的“镜像使用便捷性”其实不是孤立问题，而是下面三层系统问题的交叉点：

1. 环境交付：胖镜像、宿主机 toolkit 注入、lazy image
2. 设备抽象：device plugin、OCI hook、CDI、NRI、DRA
3. 调度决策：异构感知、拓扑感知、共享与隔离

这也是当前仓库最有潜力延展成研究工作的地方。

## 2. 与当前仓库工作的关系

当前仓库已经具备一个可工作的最小链路：

- `accelerator-installer` 在宿主机落盘二进制、profile、containerd 配置
- `accelerator-container-runtime` 在 OCI `create` 阶段改写 bundle，把 hook 注入为 `prestart`
- `accelerator-container-hook` 在容器启动前注入设备节点、驱动库、工具目录和 linker 配置
- profile 已经能表达 Iluvatar 和 Ascend 910B 的差异

这说明当前项目已经不只是一个厂商适配脚本，而是一个“声明式 accelerator runtime”的雏形。它和学术/开源生态里以下问题天然相关：

- 多厂商环境事实如何抽象
- 环境注入是否应从 runtimeClass/hook 迁移到 CDI/NRI/DRA
- 镜像与驱动环境应如何分层
- 调度器是否应显式考虑环境准备成本

换句话说，当前仓库不是研究工作的配角，而是非常适合作为研究原型系统。

## 3. 相关开源生态与技术趋势

### 3.1 Kubernetes 原生设备模型正在从 Device Plugin 走向 DRA

Kubernetes 的 device plugin 仍然是主流基础接口，但它更适合“暴露设备”，不擅长表达复杂设备选择、共享和动态配置。

更值得关注的是 Dynamic Resource Allocation（DRA）：

- Kubernetes 官方文档显示，DRA 已进入稳定阶段
- DRA 引入了 `DeviceClass`、`ResourceClaim`、`ResourceClaimTemplate`、`ResourceSlice`
- DRA 支持基于属性筛选设备
- DRA 正在继续扩展 consumable capacity、binding conditions、优先级列表等能力

这意味着未来 GPU/加速卡资源管理会逐步从“按扩展资源名申请整卡”转向“按设备属性、容量、配置和状态进行声明式申请”。

对你的研究来说，这提供了一个很强的切入点：

- 今天做的 hook/profile 抽象，将来可以直接对接到 DRA 的 `DeviceClass` 和 claim 语义
- 可以研究“环境交付层”和“资源申请层”是否应该统一建模

### 3.2 CDI 正在把设备注入标准化

Container Device Interface（CDI）非常值得重视，因为它已经把设备注入这件事从厂商私有 runtime hack，推向了统一的容器编辑模型。

CDI 的价值在于，它能统一描述：

- 设备节点
- 挂载
- 环境变量
- OCI hooks

这和当前仓库中的 profile 注入模型高度同构。也就是说，当前项目的 profile schema 有机会演进成：

- 一份中间表示
- 既可生成当前自定义 hook/runtime 配置
- 也可生成 CDI spec

这会把项目从“一个 toolkit 实现”提升到“一个多后端运行时抽象层”。

### 3.3 NRI 提供了比 OCI hook 更规范的 runtime 扩展点

containerd 的 NRI（Node Resource Interface）提供了 runtime 扩展接口，允许插件统一调整容器 spec、设备注入、hook 注入等行为。

和纯 OCI prestart hook 相比，NRI 的优势更偏工程化：

- 语义更稳定
- 更容易和 containerd 生态协同
- 更适合作为统一设备与资源策略的扩展点

对研究而言，这里可以形成一个明确的问题：

- 在异构加速器场景里，OCI hook、CDI、NRI 的可维护性、隔离性和表达能力差异是什么？

### 3.4 NVIDIA 生态已经开始往 CDI / DRA 演进

NVIDIA GPU Operator 是当前最具代表性的工业界参照物。

值得注意的信号：

- GPU Operator 已支持 CDI
- 文档中提供 `nvidia-cdi` / `nvidia-legacy` runtime class
- 还引入了 NRI 相关支持
- NVIDIA 也已经在推进 DRA Driver for GPUs

这说明产业界方向已经不是继续堆更多私有 runtime hack，而是在迁移到：

- 标准化设备注入
- 更细粒度设备共享
- 动态可配置资源分配

如果你的研究只停在“我们也能做一个 hook”，价值会受限；但如果上升到“统一表示 + 多后端生成 + 与调度联动”，就会明显更强。

### 3.5 CNCF 生态中，异构设备虚拟化和调度已经成为热点

几个非常相关的项目：

- HAMi：已进入 CNCF Sandbox，定位就是 heterogeneous AI computing / device virtualization
- Volcano：明确支持 heterogeneous cluster、GPU virtualization、Dynamic MIG、vGPU 等
- Koordinator：在做 fine-grained device scheduling，覆盖 GPU / RDMA / FPGA
- Kueue：Topology Aware Scheduling 已进入 beta，开始面向 AI/ML 工作负载提供更显式的拓扑调度能力

这些项目给你的启发不是“选一个替代你当前仓库”，而是：

- 它们分别覆盖了共享、虚拟化、批调度、拓扑调度
- 你的工作可以补上“镜像环境交付”和“运行时环境抽象”这一块空白
- 更进一步，可以研究环境交付与调度是否应耦合优化

### 3.6 镜像分发方向：大镜像不是唯一解，lazy image 已非常成熟

镜像侧的一个重要趋势是：

- stargz-snapshotter
- Nydus / nydus-snapshotter
- SOCI

都在解决“大镜像启动慢、拉取慢、网络浪费大”的问题。

其中：

- stargz-snapshotter 走 eStargz / lazy pulling 路线
- Nydus 更强调 chunk-based、按需加载、去重和更好的镜像分发效率
- SOCI 走的是 Seekable OCI index 路线，尽量在不改镜像使用方式的前提下做按需加载

这和你的研究方向直接相关，因为“胖镜像 vs toolkit”不是二选一，还可以出现第三种甚至第四种架构：

- 胖镜像
- 纯 toolkit 注入
- 胖镜像 + lazy loading
- toolkit 注入 + lazy loading
- 混合式分层环境镜像

## 4. 结合当前方向，可以形成的核心研究问题

### 4.1 镜像交付与环境注入到底应该如何分工

这是最自然的主问题。

你可以把环境拆成三层：

- 宿主机绑定层：驱动、设备节点、控制设备、与内核匹配的 runtime libs
- 可共享运行时层：常用基础库、厂商工具、少量框架适配层
- 用户业务层：训练框架、推理框架、应用代码、模型、依赖

核心研究点：

- 哪些内容必须来自宿主机
- 哪些内容适合由 toolkit 注入
- 哪些内容必须放进镜像
- 哪些内容适合做 lazy image

这个问题很贴近实际，也非常容易设计实验。

### 4.2 是否可以用统一中间表示描述多厂商运行时环境

当前仓库的 profile 已经在做这件事，但还可以进一步学术化。

你可以尝试定义一个 vendor-neutral 的 accelerator environment IR，覆盖：

- 设备选择语义
- 设备节点与控制设备
- 映射命令与解析器
- library / binary artifact
- linker 策略
- extra env
- 可选初始化动作

然后研究：

- 它能否覆盖 NVIDIA / Ascend / Iluvatar / AMD / Cambricon
- 哪些字段是公共抽象，哪些必须保留厂商扩展
- IR 能否同时生成 hook 配置、CDI spec、NRI 插件配置、未来的 DRA 资源描述

如果做成，这会是一个很强的系统抽象工作。

### 4.3 环境交付成本是否应成为调度输入

这是我最推荐你认真考虑的方向之一。

当前大多数 GPU 调度器主要关心：

- 有没有卡
- 卡是否空闲
- 任务优先级
- 拓扑是否合适

但在真实系统中，任务冷启动成本往往还受这些因素强烈影响：

- 镜像大小
- 节点上是否已有镜像缓存
- 节点是否已有所需 toolkit/profile
- 驱动文件是否已预热
- lazy image 首次缺页成本

因此可以提出一个很自然的问题：

- 在异构 GPU 集群中，调度器是否应联合优化“资源适配”和“环境准备成本”？

这比单纯做 GPU scheduler 更有新意，因为它把镜像系统和资源系统接起来了。

### 4.4 异构 GPU 资源是否应按“能力向量”而不是“卡数”申请

这和许多异构调度论文的精神一致，但你可以把它落到 Kubernetes / 云原生实现里。

能力向量可以包括：

- 显存容量
- Tensor / Matrix 核支持
- FP16 / BF16 / INT8 能力
- 互联能力
- 是否支持某厂商工具链或框架 runtime
- 是否具备特定共享模式

研究目标不是替代 device count，而是补充一种更贴近 workload 需求的资源表达方式。

如果结合 DRA 的属性选择与 claim 机制，这个方向会非常自然。

### 4.5 共享/切片机制在异构场景下的统一 QoS 研究

当前生态中已经有多种共享模式：

- MIG
- time-slicing
- vGPU
- 软件层 oversubscription
- DRA 的 consumable capacity

这些机制在不同厂商上实现方式不一样，但对用户来说都表现为“不是整卡独占”。

可以研究的问题包括：

- 不同共享模式的隔离性、公平性、tail latency、吞吐差异
- 训练和推理混部下，哪种共享模式更稳定
- 共享模式是否应与环境交付策略联动
- 共享模式变化是否应触发不同的镜像/环境准备策略

### 4.6 拓扑、通信和环境是否应联合建模

这是更偏系统/集群调度的方向。

多机训练里，任务性能通常同时取决于：

- GPU 型号
- NVLink / PCIe / RDMA 拓扑
- 节点间网络位置
- collective communication backend
- 软件环境是否匹配

如果未来跨厂商训练逐步增多，那么“能否注入驱动”只是第一步，后面还会遇到：

- 跨厂商 collective library
- 框架 backend 兼容
- 节点间环境一致性

因此可研究：

- 调度器是否应保证“拓扑一致性 + 环境一致性”
- 是否需要把运行时环境描述上送给调度器，作为 placement 约束的一部分

## 5. 可落地的研究方向建议

下面给出几条更像论文题目的方向。

### 方向 A：胖镜像 vs toolkit 注入 vs lazy image 的系统比较

这是最容易形成完整实验闭环的方向。

研究问题：

- 在异构 GPU 集群中，不同环境交付模式对冷启动、吞吐、缓存命中、网络开销、镜像复用率、安全面和可复现性有什么影响？

建议 baseline：

- 大镜像
- 纯 toolkit 注入
- 大镜像 + lazy pull
- toolkit 注入 + lazy pull
- 混合镜像分层

建议指标：

- Pod 启动时间
- 首次任务可用时间
- 镜像拉取流量
- 节点磁盘占用
- 失败恢复时间
- 多任务复用率
- 用户镜像制作复杂度

优点：

- 非常贴近你的当前系统
- 容易做出强实验
- 有明显工程与学术价值

### 方向 B：面向异构加速器的声明式环境抽象与多后端生成

研究问题：

- 能否设计一个统一 IR，表达多厂商 accelerator runtime 环境，并自动生成 hook/CDI/NRI/DRA 侧配置？

核心创新点可能包括：

- 统一环境事实抽象
- 多后端生成
- 与 vendor-specific runtime 的兼容策略
- 运行时验证机制

优点：

- 和当前仓库代码演化路径最一致
- 容易做出“原型系统 + 多厂商 case study”

风险：

- 如果只停留在 schema 设计，会偏工程总结
- 需要补强验证、自动转换、覆盖面或性能收益，论文说服力才够

### 方向 C：环境交付感知的异构 GPU 调度

研究问题：

- 调度器能否联合考虑设备匹配、拓扑匹配和环境准备成本，从而降低任务启动时延并提高整体利用率？

可能的调度输入：

- 节点加速器能力向量
- 节点已有环境 profile
- 镜像缓存状态
- lazy image 命中预测
- 作业的环境依赖描述

可以做的结果：

- 更快的冷启动
- 更少的重复拉镜像
- 更低的环境切换抖动
- 更好的跨作业复用率

这条线如果做得好，学术味会很强。

### 方向 D：共享与切片模式下的统一 QoS 控制

研究问题：

- 在 heterogeneous cluster 中，不同厂商的共享/切片机制能否用统一模型进行 QoS 管控与调度？

这条线可以把 HAMi、Volcano、Koordinator、DRA consumable capacity 放到同一比较框架里。

优点：

- 很贴近 CNCF/产业热点
- 适合做系统 benchmark

难点：

- 实验环境要求高
- 需要多种硬件或较强的模拟环境

## 6. 我对你当前阶段的优先级建议

如果你希望先做出一个最稳的研究起点，我建议按下面顺序推进。

### 第一优先级

先做“胖镜像 vs toolkit vs lazy image”的系统比较。

原因：

- 与当前仓库最贴近
- 可以先不引入太多额外复杂度
- 很容易形成一套可信的 measurement
- 实验数据能反过来支撑后续更大的系统设计

### 第二优先级

把当前 profile 体系提升为“统一环境 IR”，并开始尝试对接 CDI。

原因：

- 这一步能把项目从当前的 hook/runtime 工具，升级成更通用的运行时抽象
- 也能自然衔接 CNCF / container runtime 生态趋势

### 第三优先级

再往上做“环境交付感知调度”。

原因：

- 它最有研究潜力
- 但前提是你先把环境交付模型和 measurement 体系做扎实

## 7. 一个建议的论文主线

如果要把当前工作组织成一个更完整的科研主线，我建议考虑下面这个中心问题：

**在异构 GPU 集群中，软件环境交付和设备分配是否应该被统一建模，并由同一个声明式控制面同时驱动运行时注入与调度决策？**

这个问题的好处是：

- 能覆盖镜像与 toolkit 的矛盾
- 能覆盖多厂商环境差异
- 能衔接 CDI / NRI / DRA
- 能自然引出调度问题
- 也能直接把当前仓库作为原型系统

## 8. 对当前仓库最直接的后续演化建议

如果从研究原型角度看，当前仓库下一步最值得做的是：

1. 把 profile 明确定位成 vendor-neutral environment IR
2. 给 IR 增加更清晰的 capability / constraint 表达
3. 增加“生成 CDI spec”的后端
4. 保留当前 hook/runtime 路径作为 legacy backend
5. 为不同交付模式建立统一 benchmark
6. 逐步把环境描述上送给调度层，验证 environment-aware scheduling

## 9. 参考项目与资料

### Kubernetes / Runtime / Device

- Kubernetes Device Plugins  
  https://kubernetes.io/docs/concepts/extend-kubernetes/compute-storage-net/device-plugins/
- Kubernetes Dynamic Resource Allocation  
  https://kubernetes.io/docs/concepts/scheduling-eviction/dynamic-resource-allocation/
- Container Device Interface (CDI)  
  https://github.com/cncf-tags/container-device-interface
- containerd NRI  
  https://github.com/containerd/nri

### Vendor / CNCF / Scheduler

- NVIDIA GPU Operator  
  https://docs.nvidia.com/datacenter/cloud-native/gpu-operator/latest/
- NVIDIA DRA Driver for GPUs  
  https://docs.nvidia.com/datacenter/cloud-native/gpu-operator/25.3.1/dra-intro-install.html
- HAMi  
  https://github.com/Project-HAMi/HAMi
- CNCF Project: HAMi  
  https://www.cncf.io/projects/hami/
- Volcano  
  https://volcano.sh/en/docs/
- Koordinator Device Scheduling  
  https://koordinator.sh/zh-Hans/docs/user-manuals/fine-grained-device-scheduling/
- Kueue Topology Aware Scheduling  
  https://kueue.sigs.k8s.io/docs/concepts/topology_aware_scheduling/

### Image Distribution / Lazy Loading

- stargz-snapshotter  
  https://github.com/containerd/stargz-snapshotter
- Nydus Snapshotter  
  https://github.com/containerd/nydus-snapshotter
- SOCI  
  https://aws.amazon.com/about-aws/whats-new/2022/09/introducing-seekable-oci-lazy-loading-container-images/

### Related Research

- Gavel: Resource Efficient GPU Scheduling for Deep Learning  
  https://www.usenix.org/conference/osdi20/presentation/narayanan-deepak
- Synergy: Toward Better Resource Allocation for Machine Learning Jobs in Shared Clusters  
  https://www.usenix.org/conference/osdi22/presentation/mohan
- HetCCL  
  https://arxiv.org/abs/2601.22585

## 10. 总结

如果用一句话概括这份调研：

异构 GPU 集群里的“镜像便捷使用”不应只被当成镜像工程问题，而更应被看作“环境交付、设备抽象与调度决策的统一系统问题”。

而当前仓库最有潜力的方向，不是继续做一个只面向单厂商的 toolkit，而是把它升级成：

- 一个统一环境抽象
- 一个多后端运行时生成器
- 一个可与调度联动的研究原型系统

这条路线既能接住当前工程积累，也能比较自然地延展成系统研究工作。
