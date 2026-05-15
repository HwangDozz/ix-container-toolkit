# 异构加速卡集群调度研究综述

> 日期：2026-05-13
> 范围：DL 训练任务的 GPU/加速卡资源过配问题，聚焦异构调度、共享分区、right-sizing 三个方向

---

## 1. 问题背景

### 1.1 核心痛点

在深度学习作业管理平台中，用户倾向于申请高性能、大显存的加速卡（如 A100/H100），即使任务在低端卡上也能满足需求。这导致：

- **资源利用率低**：高端卡被低需求任务占用，低端卡闲置
- **排队时间长**：高端卡成为瓶颈，其他用户等待
- **成本高**：高端卡单价高，过配直接增加 TCO

### 1.2 问题本质

GPU 资源过配（over-provisioning）的核心矛盾：用户缺乏动机主动降低资源请求，调度系统缺乏足够的信息做 right-sizing。

### 1.3 研究角度概览

| 角度 | 核心思路 |
|------|---------|
| Job Profiling 与资源需求预测 | 在提交阶段预测 job 的实际 GPU 需求 |
| 异构 GPU 集群调度 | 根据 job 特征分配到最合适的 GPU 类型 |
| GPU 共享与分区 | 多个 job 共享同一张卡，或用 MIG 切分 |
| Right-Sizing 与自动调整 | 运行时持续监控，自动调整 GPU 分配 |
| 激励机制与定价 | 经济手段引导用户选择匹配的 GPU |

---

## 2. 异构 GPU 集群调度

### 2.1 技术路线分类

| 路线 | 代表工作 | 核心方法 |
|------|---------|---------|
| 吞吐最优轮调度 | Gavel (OSDI 2020) | 基于 per-(job, GPU) 吞吐矩阵的 LP 求解 |
| 协同自适应 goodput | Pollux (NSDI 2021) | 在线 profiling + 联合调优 GPU 分配/bs/lr |
| 弹性伸缩 | Lyra (EuroSys 2023) | checkpoint-based 并行度动态调整 |
| 成本公平 | Themis (SIGCOMM 2020) | 用 GPU 货币成本定义公平性 |
| RL 动态分配 | HeterPS (2021) | RL 学习异构集群调度策略 |
| RL+MILP 混合 | RLTune (2025) | RL 缩小搜索空间，MILP 精确求解 |
| 异构感知 profile | Hadar/HadarE (2025) | 扩展 Gavel/Pollux，显式建模 per-GPU 性能 profile |

### 2.2 各工作如何处理异构性

**Gavel (OSDI 2020)**：对每个 job 在每种 GPU 上做 profiling，构建吞吐矩阵 M[i][g]，调度器每轮求解 LP 最大化加权吞吐。局限：profiling 成本高，假设 profile 在调度周期内稳定。

**Pollux (NSDI 2021)**：将吞吐扩展为 goodput（= 吞吐 × 统计效率），通过在线 profiling 适应性能变化。局限：goodput 模型复杂度高，对 profiling 精度敏感。

**Themis (SIGCOMM 2020)**：用货币成本做代理——不同 GPU 类型有不同成本，公平性用成本单位衡量。局限：成本是粗粒度代理，不预测具体 job 在具体 GPU 上的实际表现。

**HeterPS (2021)**：RL agent 从经验中学习异构集群的调度策略，状态空间包含不同 GPU 类型的可用性。局限：RL 训练成本高，对新集群配置的泛化能力不确定。

**RLTune (2025)**：把异构 GPU 类型建模为 MILP 中的不同资源类，RL 负责粗筛。局限：MILP 求解时间随规模增长，依赖准确的性能模型作为输入。

**Hadar/HadarE (2025)**：显式建模 per-GPU-type 性能 profile，HadarE 增加弹性伸缩。局限：仍需离线或在线 profiling。

### 2.3 关键发现：性能等价模型是核心缺口

所有现有系统要么 (a) 对每个 job 在每种 GPU 上做 profiling（Gavel、Pollux），(b) 用成本做代理（Themis），(c) 让 RL 隐式学习（HeterPS）。**没有任何工作提出一个通用的、job-agnostic 的跨 GPU 性能等价模型**——即不需要 per-job profiling 就能预测任意 DL workload 在不同 GPU 类型上的性能。

### 2.4 研究缺口

1. **无 job-agnostic 的跨 GPU 性能预测**：每个系统都需要 per-job profiling 或 RL 训练，缺乏可迁移的轻量性能等价模型
2. **多维异构被低估**：大多数工作将 GPU 当离散类型处理，忽略时钟降频、显存压力、温度限制等连续变化
3. **成本-吞吐-公平-截止时间联合优化缺失**：Gavel 优化吞吐，Themis 优化成本，没有统一框架
4. **弹性伸缩 × 跨 GPU 异构**：Lyra 做弹性伸缩但在同构集群上，两者结合几乎未被探索
5. **非 NVIDIA GPU 异构**：所有工作假设 NVIDIA GPU，跨架构（AMD、Intel、国产加速卡）调度几乎空白
6. **profiling 开销**：通过迁移学习或硬件基准测试降低 profiling 成本是可行方向

### 2.5 相关论文列表

| 论文 | 年份 | 场景 | 核心贡献 |
|------|------|------|---------|
| Gavel | OSDI 2020 | 异构 DL 集群调度 | 吞吐矩阵 + LP 轮调度 |
| Pollux | NSDI 2021 | 协同自适应调度 | goodput 模型 + 在线 profiling |
| Themis | SIGCOMM 2020 | 公平调度 | 成本公平性定义 |
| Lyra | EuroSys 2023 | 弹性调度 | checkpoint-based 并行度调整 |
| HeterPS | 2021 | 异构 PS 训练 | RL-based 调度策略 |
| RLTune | 2025 | 异构集群 | RL + MILP 混合方法 |
| Hadar/HadarE | 2025 | 异构感知调度 | per-GPU 性能 profile + 弹性伸缩 |
| Tiresias | NSDI 2019 | GPU 集群调度 | GPU 感知的调度器（同构） |
| Optimus | EuroSys 2018 | DL 训练调度 | 吞吐最优的资源分配 |

---

## 3. GPU 共享与分区（Sharing / Partitioning）

### 3.1 共享机制分类

| 类别 | 机制 | 粒度 | 隔离性 |
|------|------|------|--------|
| 硬件分区 | NVIDIA MIG | 固定 SM+显存切片 | 强（硬件隔离） |
| 软件时间共享 | CUDA 调度 | 整卡交替 | 弱 |
| 软件空间共享 | MPS | SM 子集 | 中 |
| 混合 | MIG+MPS | MIG 实例内再分 | 中强 |
| 学术系统 | Salus, PipeSwitch | 自定义 | 不等 |

### 3.2 各工作分析

**Miger (Zhang et al., 2024)**：整合 MIG 和 MPS。MIG 提供硬件隔离但固定分区；MPS 允许灵活 SM 共享但弱隔离。组合后可在 MIG 实例内激活 MPS，实现更细粒度的多租户。

**Dynamic MIG Reconfiguration (Wang et al., 2024)**：运行时动态调整 MIG 分区而无需 GPU reset。传统 MIG 重新分区需要驱逐所有 workload，动态重配置使得按需自适应分区成为可能。

**LithOS (Coppock et al., SOSP 2025, arXiv:2504.15465)**：GPU OS，四机制：TPC Scheduler（TPC 粒度空间调度）、kernel atomization（长 kernel 切成原子单元）、硬件 right-sizing（动态确定每个 atom 的最小 TPC 需求）、透明功耗管理。核心能力：TPC stealing——空闲 TPC 从一个 workload 透明转移到另一个。效果：节省 ~25% GPU 容量，性能损失 ~4%。限制：仅限单 GPU 多路复用。

**KRISP (Chow et al., HPCA 2023)**：kernel-wise right-sizing。单次推理中不同 kernel 的资源需求差异巨大，逐 kernel 做空间分区调整。效果：吞吐量是隔离推理的 2 倍。限制：针对推理服务器。

**gShare (Yang et al., 2026)**：FaaS 平台上的激进 GPU 共享调度。

**BOER (Zhang et al., 2025)**：混合空间 GPU 共享，结合 SM 分区和显存带宽分区。

**InSS (Han et al., 2024)**：时空共享的多 GPU 推理调度，同时在 SM 子集（空间）和时间片（时间）两个维度调度。

**Salus (USENIX ATC 2019)**：通过显存管理和 SM 调度实现训练任务的 GPU 共享，支持抢占。少数聚焦训练的共享系统之一。

**PipeSwitch (OSDI 2020)**：快速上下文切换，实现低开销时间共享。

### 3.3 训练 vs 推理

**绝大多数 GPU 共享研究针对推理，而非训练。** 原因是根本性的：

- **训练是同步迭代的**：data-parallel 训练中 all-reduce 要求所有 worker 同步完成每轮迭代，一个慢 worker 拖慢所有人
- **训练显存占用高且波动大**：activation、gradient、optimizer state 随 batch size 变化，显存溢出直接 OOM
- **训练收敛可能受影响**：batch size 与 learning rate 耦合，共享导致的吞吐变化会改变有效 batch size

### 3.4 NVIDIA MPS/MIG 机制对比

| 特性 | MIG | MPS |
|------|-----|-----|
| 隔离粒度 | 固定切片（如 A100 的 7 个切片） | SM 子集，灵活分配 |
| 隔离强度 | 强（硬件级，零性能干扰） | 中（软件级，可被饿死） |
| 显存隔离 | 独立显存分区 | 共享显存池 |
| 灵活性 | 低（重新分区需驱逐 workload） | 高（运行时动态调整） |
| 适用场景 | 推理、小训练任务 | 共置训练、尽力而为共享 |

### 3.5 研究缺口

1. **没有训练专用的隔离原语**：MIG 太刚性，MPS 太弱，没有 GPU 版的 cgroup
2. **收敛感知共享**：没有系统联合优化共享效率和训练收敛性，资源竞争与 SGD 动力学的交互缺乏建模
3. **显存中心的共享**：训练显存由 activation（依赖 batch size）和 optimizer state（依赖模型大小）主导，跨 job 的显存 packing 问题未解
4. **梯度同步开销**：all-reduce 每轮发生，共置 job 争抢 NVLink/PCIe 带宽会导致同步延迟飙升
5. **生产环境现状**：大多数集群仍然是一张 GPU 跑一个训练任务，共享技术停留在学术验证阶段

### 3.6 相关论文列表

| 论文 | 年份 | 聚焦 | 核心贡献 |
|------|------|------|---------|
| Salus | USENIX ATC 2019 | 训练共享 | 显存管理 + SM 调度 + 抢占 |
| PipeSwitch | OSDI 2020 | 上下文切换 | 低开销时间共享 |
| LithOS | SOSP 2025 | TPC 级 right-sizing | TPC stealing + kernel atomization |
| KRISP | HPCA 2023 | 推理 right-sizing | kernel-wise 空间分区 |
| Miger | 2024 | MIG+MPS 整合 | MIG 实例内 MPS 共享 |
| Dynamic MIG | 2024 | 动态 MIG | 运行时 MIG 重配置 |
| gShare | 2026 | FaaS 共享 | 激进共享调度 |
| BOER | 2025 | 空间共享 | 混合 SM + 显存带宽分区 |
| InSS | 2024 | 时空共享 | 多 GPU 推理的时空调度 |

---

## 4. Right-Sizing 与自动调整

### 4.1 运行时 right-sizing 的三种方法

**空间分区（sub-GPU 粒度）**：LithOS 和 KRISP 在整卡级别以下操作。LithOS 在 TPC 粒度调度，实现 TPC stealing（空闲 TPC 透明转移）。KRISP 做 kernel-wise right-sizing，识别单次推理中不同 kernel 的资源需求差异。

**阶段感知动态调整**：FLEXI 检测执行阶段（CPU-heavy 预处理 vs GPU-heavy 模型执行），在阶段间动态重分配资源，解决"GPU-heavy 阶段 CPU 闲置、CPU-heavy 阶段 GPU 闲置"的资源搁置问题。

**弹性并行度调整**：Aryl、EasyScale、Tenplex 通过改变 data/model parallelism 度来调整 job 的 GPU 分配。需要 checkpoint-reconfigure-restart 周期或实时并行度重配置。

### 4.2 过配检测信号

| 信号 | 来源 | 含义 |
|------|------|------|
| GPU SM/TPC 利用率低 | LithOS | TPC 可被 steal |
| 阶段性资源交替 | FLEXI | 阶段间可动态调整 |
| 吞吐随 GPU 数增加而饱和 | EasyScale, Aryl | 弹性伸缩的收益边界 |
| 显存带宽利用率低 | 多个工作 | 算力分配冗余 |
| kernel 并发度低 | KRISP | 计算容量未充分利用 |
| 集群 GPU 利用率 <50% | Jeon et al. (Microsoft trace) | 生产环境普遍过配 |

### 4.3 安全回收资源的四种机制

1. **透明 TPC stealing**（LithOS）：GPU OS 级别重分配，sub-ms 延迟，workload 无感知
2. **Checkpoint-reconfigure-restart**（Tenplex）：保存训练状态 → 释放 GPU → 重配并行度 → 恢复。安全但有秒级到分钟级停机
3. **阶段边界调整**（FLEXI）：仅在阶段切换时回收，避免 kernel 中断
4. **弹性 batch size 缩放**（EasyScale）：GPU 被回收时增大 per-GPU batch size，配合 lr scaling 保持收敛

### 4.4 相关论文列表

| 论文 | 年份 | 场景 | 核心贡献 |
|------|------|------|---------|
| LithOS | SOSP 2025 | 单 GPU 多路复用 | TPC stealing + kernel atomization |
| KRISP | HPCA 2023 | 推理 right-sizing | kernel-wise 空间分区 |
| FLEXI | BigData Congress 2025 | Serverless GPU | 阶段感知函数弹性调整 |
| Aryl | 2022 | 弹性集群调度 | 统一训练+推理集群 |
| EasyScale | 2022 | 弹性训练 | 精度一致的并行度调整 |
| Tenplex | 2023 | 动态并行度 | checkpoint-based 并行度重配 |
| Kale | SoCC 2024 | 在线训练调度 | 弹性 GPU 调度 |
| EaCO | 2024 | 训练能效 | GPU 利用率与能效关系 |
| Lazarus | 2024 | MoE 弹性训练 | 分布式弹性 + 容错 |
| Jeon et al. | 2019 | 生产 trace 分析 | Microsoft 多租户 GPU 集群利用率分析 |

### 4.5 研究缺口

1. **分布式训练的统一 right-sizing**：LithOS 只做单 GPU，没有系统将 TPC 级 right-sizing 与多节点分布式训练结合
2. **自动过配检测**：大多数系统依赖手动 profiling 或静态策略，没有生产级系统能自动检测分布式训练 job 的过配并安全回收
3. **信号粒度不足**：GPU 利用率本身不够——一个 job 可以显示高利用率但计算效率低（memory-bound kernel 只用少量 SM）
4. **分布式 checkpoint 开销**：大模型（数百 GB）的 checkpoint 需要分钟级时间，sub-minute 资源重分配仍是难题
5. **异构 GPU 集群 right-sizing**：大多数工作假设同构 GPU，跨架构 right-sizing 未被探索
6. **生产部署空白**：学术系统在受控环境有结果，没有论文报告千卡级、百 job 并发的生产部署

---

## 5. 国产异构加速卡的特殊维度

### 5.1 国产加速卡生态概览

| 厂商 | 代表型号 | 软件栈 | 编程模型 | 关键差异 |
|------|---------|--------|---------|---------|
| NVIDIA | A100/H100/B200 | CUDA/cuDNN/NCCL | CUDA | 基准线 |
| 华为昇腾 | 910B/910C | CANN/MindSpore | Ascend C | 达芬奇架构，cube core + vector core |
| 天数智芯 | BI-V150 | CoreX SDK | CUDA-like | 兼容 CUDA 语义，自有驱动层 |
| 寒武纪 | MLU370/590 | BANG/CNToolkit | BANG C | 自有指令集，MLU-Link 互联 |
| 壁仞 | BR100/BR104 | BIRENSUPA | 类 CUDA | 高算力但软件生态较新 |
| 摩尔线程 | MTT S4000 | MUSA | MUSA（CUDA-like） | 兼容 CUDA 语义 |
| 燧原 | i20/i30 | TopsRider | GCU-C | 自有 GCU 架构 |
| 昆仑芯 | R200/R300 | XPU SDK | XPU C | 百度系，R200 支持类 CUDA |

### 5.2 新增研究方向

#### 5.2.1 跨架构性能建模与等价推理

**问题**：同一个 ResNet-50 训练任务，在 A100 上 45 分钟跑完，在昇腾 910B 上要多久？在寒武纪 MLU370 上呢？

**为什么难**：
- 不是简单的"算力比例换算"——不同芯片对不同算子的支持效率差异巨大
- 昇腾的 cube core 对矩阵乘法高度优化，但对不规则算子（如 scatter/gather）效率骤降
- 寒武纪的 BANG 编程模型与 CUDA 差异大，某些 kernel 需要完全重写
- 即使"兼容 CUDA"的卡（天数、摩尔线程），底层微架构也不同，性能特征不等价

**研究切入点**：
- 基于**算子级 profiling**的跨架构性能预测：对一组基准算子在不同芯片上做 profiling，构建算子性能指纹，再通过模型计算图组合预测端到端训练时间
- **架构特征向量**：将每种加速卡表示为一个特征向量（SM 数量、显存带宽、互联带宽、算子覆盖率、精度支持），与 job 的计算图特征做交叉预测
- 可以借鉴 MLPerf 的 benchmark 思路，但目标不是排名而是构建可迁移的性能模型

#### 5.2.2 算子覆盖差异下的任务拆分与调度

**问题**：不同加速卡对算子的支持覆盖率不同。有些算子在某张卡上完全不支持（或效率极低），但在另一张卡上很高效。

**为什么这是新问题**：
- NVIDIA 生态内，A100 和 V100 的差异只是速度，算子都能跑
- 但昇腾不支持某些 CUDA 算子，寒武纪不支持某些自定义 op，壁仞的软件栈可能缺少某个 kernel 实现
- 一个完整的 DL 训练 job 可能包含 50+ 种不同算子，其中 48 种在国产卡上能跑，2 种不行

**研究切入点**：
- **算子级异构调度**：将 job 拆分为算子组，根据每种芯片的算子覆盖和效率，将不同算子组分配到不同芯片上执行
- **异构 pipeline parallelism**：模型的不同层/阶段放在不同类型的加速卡上——矩阵密集型层放在 cube core 优化的昇腾上，不规则操作层放在 CUDA 兼容的天数上
- **Fallback-aware scheduling**：当某张卡不支持某个算子时，自动 fallback 到 CPU 或其他卡，调度器需要建模 fallback 的性能代价

**与现有工作的区别**：
- 现有的 pipeline parallelism（PipeDream、GPipe）假设所有 GPU 同构
- 在异构卡上做 pipeline parallelism，每 stage 的处理速度不同，micro-batch 调度策略需要重新设计

#### 5.2.3 互联拓扑感知的异构多卡调度

**问题**：不同加速卡的互联方式不同，跨卡通信带宽差异巨大。

**拓扑差异**：
- NVIDIA：NVLink（600+ GB/s）> PCIe Gen5（64 GB/s）
- 昇腾：HCCS（华为片间互联，~100 GB/s 级别）
- 寒武纪：MLU-Link
- 天数/壁仞/摩尔线程：主要依赖 PCIe
- 混合部署时，卡间通信可能退化到最慢的链路

**研究切入点**：
- **拓扑感知的 worker 放置**：all-reduce 密集的 data-parallel worker 应放在同互联域内；计算密集的 model-parallel stage 应放在高算力卡上
- **通信-计算联合优化**：给定模型的计算图和通信模式（all-reduce、all-gather、P2P），以及集群的异构拓扑，求解最优的 worker-to-device 映射
- 可形式化为带约束的组合优化问题，用 ILP 或 RL 求解

#### 5.2.4 精度异构调度

**问题**：不同加速卡对数值精度的支持不同。

**精度差异**：
- NVIDIA：FP64/FP32/TF32/BF16/FP16/FP8/INT8
- 昇腾 910B：FP16/BF16/INT8（不原生支持 FP64 训练）
- 寒武纪 MLU370：FP32/FP16/BF16/INT8
- 某些国产卡对 BF16 的支持效率远低于 FP16，或反之

**研究切入点**：
- **精度感知调度**：根据 job 的精度需求匹配到最适合的芯片
- **精度降级策略**：当高端卡不可用时，是否可以将 job 的部分计算降到低端卡支持的精度上执行？需要分析精度降级对收敛的影响
- 与 mixed-precision training（Micikevicius et al., 2018）交叉，但扩展到跨芯片的精度异构场景

#### 5.2.5 软件栈成熟度建模与调度

**问题**：国产卡的软件栈成熟度差异大，同一个 job 在不同卡上的"可用性"和"稳定性"不同。

**现实情况**：
- PyTorch 在 NVIDIA 上经过充分优化，在国产卡上可能有未覆盖的算子或性能 regression
- 同一张卡的不同 SDK 版本对同一算子的支持可能不同
- 某些 job 在国产卡上能跑通但偶发精度异常（软件 bug 或数值差异累积）

**研究切入点**：
- **可靠性感知调度**：将"job 在某张卡上成功完成的概率"作为调度维度
- **软件栈兼容性矩阵**：构建 job feature × accelerator software version 的兼容性数据库
- **渐进式迁移**：先在国产卡上跑小规模验证，确认无精度问题后再大规模调度

#### 5.2.6 成本-合规-性能三维权衡

**问题**：中国的 AI 算力环境中存在"信创合规"约束——某些场景必须使用国产芯片。同时国产卡通常比 NVIDIA 便宜或更容易获取，但性能可能更低。

**研究切入点**：
- **约束优化调度**：在"至少 X% 的计算必须在国产卡上完成"的合规约束下，最小化 job 完成时间或最大化集群吞吐
- **混合集群弹性调度**：将 NVIDIA 卡作为"加速池"，国产卡作为"基础池"，根据负载动态调整比例
- **成本建模**：国产卡的 TCO 不仅是采购价，还包括软件适配成本、调试时间、性能损失

#### 5.2.7 故障弹性与跨架构 Failover

**问题**：国产卡的软件栈成熟度较低，运行中出现异常（OOM、精度异常、驱动 crash）的概率更高。

**研究切入点**：
- **跨架构 checkpoint 兼容**：能否在一个架构上保存 checkpoint，在另一个架构上恢复？需处理不同芯片上 optimizer state 的格式差异
- **弹性 failover**：当国产卡集群出现大面积 job 失败时，自动将 pending job 调度到 NVIDIA 集群
- **渐进式可靠性提升**：随着国产卡软件栈成熟，调度器自动调整对不同芯片的信任权重

---

## 6. 综合研究机会

### 6.1 三个方向的交叉

```
                    +-----------------------------+
                    |   用户提交 DL 训练任务       |
                    +-------------+---------------+
                                  |
                    +-------------v---------------+
                    |  Job Profiling / 特征提取    |
                    |  (轻量，无需跑完整 job)      |
                    +-------------+---------------+
                                  |
              +-------------------+-------------------+
              v                   v                   v
    +-----------------+  +-----------------+  +-----------------+
    | 异构 GPU         |  | GPU 共享/      |  | 运行时           |
    | 等价调度         |  | 分区           |  | Right-Sizing     |
    | (方向 2)         |  | (方向 3)       |  | (方向 4)         |
    +-----------------+  +-----------------+  +-----------------+
```

### 6.2 最有价值的交叉点

1. **跨 GPU 性能等价模型 × 异构调度**：构建基于 GPU 规格 + 模型计算图的轻量性能预测模型，替代 Gavel/Pollux 的 per-job profiling。可作为所有异构调度系统的基础设施。

2. **训练任务共享 × 收敛性保证**：在 MIG/MPS 之上，研究共享对 SGD 收敛的理论影响，给出收敛保证条件下的最优共享策略。

3. **分布式 right-sizing × 异构集群**：在分布式训练运行中，根据 job 的实际资源使用模式，将其部分 worker 从高端 GPU 迁移到低端 GPU（或回收部分 GPU），同时保证训练不中断。

4. **Production trace 驱动的过配检测**：利用 Microsoft/Google/Meta 的生产集群 trace，训练过配检测模型，识别"这个 job 用了 A100 但实际只需要 V100"的模式。

### 6.3 国产加速卡带来的额外交叉

5. **算子覆盖差异 × 异构调度**：在算子支持不完全的约束下做跨架构调度，是 NVIDIA 生态内不存在的新问题维度。

6. **信创合规 × 成本优化**：在合规约束下最大化集群利用率，是具有中国特色的实际需求。

---

## 7. 推荐的可切入研究问题

| 研究问题 | 所属方向 | 新颖性 | 可行性 | 预期产出 |
|---------|---------|--------|--------|---------|
| 构建 job-agnostic 的跨 GPU 性能等价模型 | 异构调度 | 高 | 高 | 模型+系统 |
| 训练任务 GPU 共享的收敛性理论分析 | 共享分区 | 高 | 中 | 理论+系统 |
| 分布式训练的运行时 over-provisioning 检测与自动回收 | Right-Sizing | 中高 | 中 | 系统+trace 分析 |
| 异构 GPU 集群的 cost-performance Pareto 调度 | 异构调度+Right-Sizing | 高 | 中 | 算法+系统 |
| 算子覆盖差异下的跨架构调度 | 国产异构 | 极高 | 中 | 系统+benchmark |
| 国产/非 NVIDIA 加速卡的异构混合调度 | 国产异构 | 高 | 高 | 系统 |
| 精度异构调度与收敛性分析 | 国产异构 | 高 | 中 | 理论+系统 |

---

## 8. 参考文献

### 异构调度
- [Gavel](https://www.usenix.org/conference/osdi20/presentation/mohan) — OSDI 2020
- [Pollux](https://www.usenix.org/conference/nsdi21/presentation/qiu) — NSDI 2021
- [Themis](https://dl.acm.org/doi/10.1145/3387514.3405871) — SIGCOMM 2020
- [Lyra](https://dl.acm.org/doi/10.1145/3552326.3587450) — EuroSys 2023
- HeterPS — 2021
- RLTune — 2025
- Hadar/HadarE — 2025
- Tiresias — NSDI 2019
- Optimus — EuroSys 2018

### 共享与分区
- Salus — USENIX ATC 2019
- PipeSwitch — OSDI 2020
- [LithOS](https://arxiv.org/abs/2504.15465) — SOSP 2025
- KRISP — HPCA 2023
- Miger — 2024
- Dynamic MIG Reconfiguration — 2024
- gShare — 2026
- BOER — 2025
- InSS — 2024

### Right-Sizing 与弹性
- [Aryl](https://arxiv.org/abs/2202.07896) — 2022
- [EasyScale](https://arxiv.org/abs/2208.14228) — 2022
- [Tenplex](https://arxiv.org/abs/2312.05181) — 2023
- [EaCO](https://arxiv.org/abs/2412.08294) — 2024
- Lazarus — 2024
- Kale — SoCC 2024
- FLEXI — BigData Congress 2025
- [Jeon et al.](https://arxiv.org/abs/1901.05758) — 2019 (Microsoft 生产 trace)

### 综述
- Huang et al. — ACM Computing Surveys 2025 (效率、公平、安全的 AI 加速器资源共享综述)
- Thong & Kim — 2025 (GPU 调度技术综述)
- Pashikanti — IJERET 2025 (GPU Fleet FinOps)
