# 异构加速卡集群研究方向探索

> 基于 accelerator-toolkit 项目的科研方向调研
> 日期：2026-05-08

## 一、相关论文调研

### 1.1 异构 GPU 集群训练

| 论文 | 年份 | 核心贡献 |
|------|------|----------|
| **Zorse** (arXiv:2507.10392) | 2025 | 首个统一流水线并行+数据并行的异构 LLM 训练系统，支持非对称 PP stage 和异构 DP group |
| **HARP** (arXiv:2509.24859) | 2025 | 自动并行训练框架，细粒度 planner 搜索 inter-operator 并行策略，异构感知 1F1B 调度器 |
| **Cephalo** (arXiv:2411.01075) | 2024 | 解耦计算分配与训练状态分配，优化异构集群上 Transformer 的计算和内存利用率 |
| **HAP** (arXiv:2401.05965) | 2024 | 基于程序合成的 SPMD 并行训练，自动优化张量分片策略和通信方法，2.41x 加速 |
| **Frenzy** (arXiv:2412.14479) | 2024 | 无服务器 LLM 训练，内存感知调度，92% 内存预测精度，12-18% JCT 降低 |
| **GOGH** (arXiv:2510.15652) | 2025 | 基于学习的异构集群 GPU 编排，在线资源分配，最小化能耗同时满足性能需求 |
| **RLTune** (arXiv:2512.10271) | 2025 | RL 驱动的异构集群调度，GPU 利用率提升 20%，JCT 降低 70% |
| **HetPipe** (arXiv:2005.14038) | 2020 | 流水线模型并行+数据并行，支持异构 GPU 集群上的大 DNN 训练 |

### 1.2 集群通信优化

| 论文 | 年份 | 核心贡献 |
|------|------|----------|
| **Efficient Collective Communication Library** (arXiv:2510.00991) | 2025 | 大规模 GPU 训练集群中的高效可靠集合通信库设计 |
| **Cronus** (arXiv:2509.17357) | 2025 | 异构 GPU 集群上的 LLM 推理，部分 disaggregated prefill |

### 1.3 已知经典系统（训练调度方向）

| 系统 | 会议 | 核心思想 |
|------|------|----------|
| **Gavel** | OSDI 2020 | 异构 GPU 调度，基于优化的公平性保证 |
| **Pollux** | OSDI 2021 | 协同自适应集群调度，goodput 优化 |
| **Tiresias** | ATC 2019 | GPU 集群管理，JCT 优化 |
| **Themis** | OSDI 2020 | 公平调度，finished duration fairness |
| **Galaxy** | SOSP 2023 | 大规模 GPU 集群调度，效率+公平性 |

## 二、研究空白分析

基于论文调研和本项目现状，识别出以下研究空白：

### 空白 1：容器化层面对异构训练的透明支持

**现状**：现有异构训练研究（Zorse、HARP、Cephalo）聚焦在训练框架层面（并行策略、调度算法），假设底层容器/运行时环境已经就绪。但实际上，不同架构 GPU 的容器化部署存在显著差异（驱动注入、NCCL 兼容性、设备发现）。

**机会**：在容器 runtime 层面实现透明的异构 GPU 支持，让训练框架无需感知底层硬件差异。

### 空白 2：跨架构集合通信的自动适配

**现状**：NCCL 是 NVIDIA 生态的通信标准，但国产 GPU（Iluvatar、Ascend、Metax）各自有兼容层，且兼容程度不一（如 Iluvatar 的 NCCL P2P/SHM 传输 bug）。现有研究假设通信库正常工作。

**机会**：构建自适应通信层，自动检测并选择最优传输通道（P2P/SHM/Socket），处理跨架构的通信兼容性问题。

### 空白 3：异构集群的统一设备抽象

**现状**：每个 GPU 厂商有自己的设备发现机制（`nvidia-smi`、`ixsmi`、`mx-smi`、`npu-smi`），环境变量命名不一致（`CUDA_VISIBLE_DEVICES`、`ILUVATAR_COREX_VISIBLE_DEVICES`、`METAX_VISIBLE_DEVICES`）。

**机会**：构建统一的设备抽象层，屏蔽厂商差异，为上层调度和训练框架提供一致的设备视图。

### 空白 4：容器化环境下的 GPU 故障检测与恢复

**现状**：异构 GPU 集群的故障模式多样（NCCL hang、SIGBUS、设备枚举失败），现有容错研究主要针对同构集群。

**机会**：在容器 runtime 层面实现 GPU 健康检测和自动恢复，结合通信超时检测和 fallback 策略。

## 三、基于本项目的科研方向建议

### 方向 1：透明异构 GPU 容器运行时（Transparent Heterogeneous GPU Container Runtime）

**核心 idea**：将 accelerator-toolkit 扩展为一个通用的异构 GPU 容器运行时，通过 profile 驱动的设备注入和通信适配，让同一套训练代码无需修改即可在不同架构 GPU 上运行。

**创新点**：
- 统一的设备发现和抽象层（基于 profile 的设备枚举）
- 自适应通信层：自动检测 NCCL 兼容性，选择 P2P/SHM/Socket/gloo
- 跨架构的环境变量和 linker 配置注入
- 容器级别的 GPU 健康检测和 fallback

**与现有工作的区别**：
- 现有工作（NVIDIA Container Toolkit）只支持单一厂商
- 本方案支持多厂商、多架构的统一容器化
- 增加了通信层的自适应能力

**可能的发表方向**：ATC / EuroSys / HotOS

### 方向 2：异构 GPU 集群的容器感知调度（Container-Aware Heterogeneous GPU Scheduling）

**核心 idea**：结合容器 runtime 层面的设备信息（GPU 类型、通信拓扑、NCCL 兼容性），为异构集群的训练任务自动选择最优的设备组合和并行策略。

**创新点**：
- 利用容器 runtime 暴露的设备元信息（GPU 架构、互联拓扑、通信能力）指导调度
- 调度器感知不同 GPU 的 NCCL 兼容性（如 Iluvatar 需要 Socket 传输）
- 自动为任务选择最优的 GPU 组合（同构 vs 异构，考虑通信开销）

**与现有工作的区别**：
- Gavel/Pollux 等假设同构通信开销
- 本方案在调度时考虑实际的通信拓扑和兼容性

**可能的发表方向**：MLSys / NSDI

### 方向 3：跨架构集合通信容错（Cross-Architecture Collective Communication Fault Tolerance）

**核心 idea**：基于本项目发现的 Iluvatar NCCL P2P/SHM bug 场景，构建一个自动检测和容错的集合通信层。

**创新点**：
- 通信通道健康检测：自动发现 P2P hang、SHM SIGBUS 等问题
- 自动 fallback：P2P → SHM → Socket → gloo，逐级降级
- 通信性能建模：根据 GPU 架构和互联拓扑预测最优通信策略
- 容器级别的通信超时检测和进程重启

**与现有工作的区别**：
- 现有容错研究（如 Gemini）关注节点故障，不关注通信层 bug
- 本方案针对的是实际遇到的 NCCL 兼容性问题

**可能的发表方向**：SC / IPDPS / HPDC

### 方向 4：GPU 容器化基准测试与能效分析（GPU Containerization Benchmarking）

**核心 idea**：构建一个跨架构 GPU 容器化基准测试框架，系统评估不同 GPU 架构在容器化环境下的训练性能、能效和可靠性。

**创新点**：
- 统一的 portable training benchmark（如本项目的 GPT-3 训练脚本）
- 跨架构的性能对比：Iluvatar vs Ascend vs Metax vs NVIDIA
- 容器化开销量化：hook 注入、ldconfig、设备发现的延迟
- 能效分析：不同 GPU 的 FLOPS/Watt 在容器化环境下的实际表现

**与现有工作的区别**：
- MLPerf 等基准测试不关注容器化层面
- 本方案聚焦容器化环境下的实际表现

**可能的发表方向**：Benchmarking Workshop / CCGrid

## 四、推荐的下一步行动

### 短期（1-2 个月）

1. **整理实验数据**：将 Iluvatar NCCL P2P/SHM 问题的详细排查过程整理为技术报告
2. **扩展 benchmark**：在三种国产 GPU 上运行更完整的 benchmark（不同模型规模、不同 batch size）
3. **量化通信开销**：测量 Socket 传输 vs P2P 传输的性能差异

### 中期（3-6 个月）

4. **实现自适应通信层**：在 accelerator-toolkit 中增加 NCCL 兼容性检测和自动 fallback
5. **构建调度原型**：基于容器 runtime 暴露的设备信息，实现一个简单的异构感知调度器
6. **撰写 workshop paper**：将容器化层面的异构 GPU 支持经验整理为短论文

### 长期（6-12 个月）

7. **系统论文**：基于方向 1 或方向 2，投稿 ATC/EuroSys/MLSys
8. **开源发布**：将 accelerator-toolkit 扩展为通用异构 GPU 容器运行时

## 五、关键参考文献

```bibtex
@article{zorse2025,
  title={Zorse: Optimizing LLM Training Efficiency on Heterogeneous GPU Clusters},
  author={Guo, Runsheng Benson and Anand, Utkarsh and Daudjee, Khuzaima and Sen, Rathijit},
  journal={arXiv preprint arXiv:2507.10392},
  year={2025}
}

@article{harp2025,
  title={HARP: Orchestrating Automated Parallel Training on Heterogeneous GPU Clusters},
  author={Liang, Antian and Zhao, Zhigang and Zhang, Kai and Shi, Xuri},
  journal={arXiv preprint arXiv:2509.24859},
  year={2025}
}

@article{cephalo2024,
  title={Cephalo: Harnessing Heterogeneous GPU Clusters for Training Transformer Models},
  author={Guo, Runsheng Benson and Anand, Utkarsh and Chen, Arthur and Daudjee, Khuzaima},
  journal={arXiv preprint arXiv:2411.01075},
  year={2024}
}

@article{hap2024,
  title={HAP: SPMD DNN Training on Heterogeneous GPU Clusters with Automated Program Synthesis},
  author={Zhang, Shiwei and Diao, Lansong and Wu, Chuan and Cao, Zongyan},
  journal={arXiv preprint arXiv:2401.05965},
  year={2024}
}

@article{frenzy2024,
  title={Frenzy: A Memory-Aware Serverless LLM Training System for Heterogeneous GPU Clusters},
  author={Chang, Zihan and Xiao, Sheng and He, Shuibing and Yang, Siling},
  journal={arXiv preprint arXiv:2412.14479},
  year={2024}
}

@article{gogh2025,
  title={GOGH: Correlation-Guided Orchestration of GPUs in Heterogeneous Clusters},
  author={Raeisi, Ahmad and Dolati, Mahdi and Darabi, Sina and Talebi, Sadegh},
  journal={arXiv preprint arXiv:2510.15652},
  year={2025}
}

@article{rltune2025,
  title={Hybrid Learning and Optimization-Based Dynamic Scheduling for DL Workloads on Heterogeneous GPU Clusters},
  author={Dongare, Shrubi and Khan, Redwan Ibne Seraj and Albahar, Hadeel and Zhao, Nannan},
  journal={arXiv preprint arXiv:2512.10271},
  year={2025}
}

@article{collcomm2025,
  title={An Efficient, Reliable and Observable Collective Communication Library in Large-scale GPU Training Clusters},
  author={Chen, Ziteng and Hu, Xiaohe and Zhang, Menghao},
  journal={arXiv preprint arXiv:2510.00991},
  year={2025}
}
```
