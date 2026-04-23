# Iluvatar BI-V150 Backend 调研结论

> 更新日期：2026-04-23
> 范围：基于本机仓库、profile、历史节点验证记录和 2026-04-23 集群查询结果。

## 核心判断

天数迁移的主要难点确实是 PyTorch backend，而不是 toolkit 注入链路。

当前 toolkit 已能用 profile 表达 Iluvatar 的设备节点、selector env、`ixsmi` 映射命令、驱动库和工具注入路径。真正需要补齐的是一个能让 PyTorch 在 CoreX 上正常执行 tensor、backward、CNN、DDP 和 Transformer 的 backend 镜像。

## 已知节点事实

历史验证节点：

```text
inspur-01
```

硬件事实：

- GPU：Iluvatar BI-V150
- 数量：8
- 设备节点：`/dev/iluvatar0` 到 `/dev/iluvatar7`
- 设备 major：`508`
- 宿主机驱动根目录：`/usr/local/corex -> /usr/local/corex-4.3.0`
- 管理工具：`/usr/local/corex/bin/ixsmi`
- `ixsmi` 需要 `LD_LIBRARY_PATH=/usr/local/corex/lib64:/usr/local/corex/lib`
- Kubernetes resource：`iluvatar.com/gpu`
- 节点标签：`iluvatar.ai/gpu=present`

历史记录中，宿主机 `/usr/local/corex/lib64` 只包含驱动层关键库：

```text
libcuda.so
libcuda.so.1
libixml.so
libixthunk.so
```

2026-04-23 复核当前 `inspur-01` 时，`/usr/local/corex` 仍是指向 `corex-4.3.0/` 的符号链接，但 `/usr/local/corex-4.3.0` 目录在只读 hostPath 查询中不可见，属于悬空链接状态。

因此当前可用路径更像是：

```text
Device Plugin 注入 /dev/iluvatarN 和 ILUVATAR_COREX_VISIBLE_DEVICES
+ backend 镜像自带 /usr/local/corex-4.3.0 runtime/PyTorch
```

不能假设宿主机一定有可挂载的完整 `/usr/local/corex` 用户态目录。`profiles/iluvatar-bi-v150.yaml` 中 driver library/bin artifact 已设置为 optional，避免统一 runtime 在该节点上因缺失 hostPath 阻断容器启动。

## 当前 profile 要点

`profiles/iluvatar-bi-v150.yaml` 当前表达：

- selector env：`ILUVATAR_COREX_VISIBLE_DEVICES`
- 设备节点：`/dev/iluvatar*`
- 映射命令：`ixsmi --query-gpu=index,uuid --format=csv`
- 驱动库路径：`/usr/local/corex/lib64`、`/usr/local/corex/lib`
- 工具路径：`/usr/local/corex/bin`
- 容器内 driver root：`/usr/local/corex`
- linker：写入 `ld.so.conf.d` 并运行 `ldconfig`

历史 Pod 记录显示 Device Plugin 注入的 selector env 值是 GPU UUID 列表，而不是数字 index：

```text
ILUVATAR_COREX_VISIBLE_DEVICES=GPU-...,GPU-...
```

因此 UUID 到 `/dev/iluvatarN` 的映射能力是必须项。

## 资源名与调度标签

2026-04-23 已通过 `kubectl get node inspur-01 -o yaml` 和 `ix-config` 确认：

| 项 | 当前值 |
|---|---|
| Kubernetes resource | `iluvatar.com/gpu` |
| Capacity / Allocatable | `8` |
| Device Plugin config | `resourceName: iluvatar.com/gpu` |
| 调度标签 | `iluvatar.ai/gpu=present` |

因此 Job 资源请求必须使用 `iluvatar.com/gpu`，但 toolkit DaemonSet / RuntimeClass 调度仍可使用 `iluvatar.ai/gpu=present`。

## 已知官方/种子镜像

历史 Pod 使用过：

```text
tianshu/corex:4.3.0
```

该镜像内置：

- `/usr/local/corex -> /usr/local/corex-4.3.0`
- `/usr/local/openmpi`
- PyTorch：`2.4.1+corex.4.3.0`
- torchvision：`0.19.1a0+corex.4.3.0`
- torchaudio：`2.4.1+corex.4.3.0`
- vLLM、DeepSpeed、Megatron-DeepSpeed、Flash Attention、APEX、Triton、Transformers、Accelerate 等上层框架

这说明它适合作为首轮 L3-L6 验证的 seed image，但不是理想的最小 PyTorch backend。

2026-04-23 查询确认：

- 镜像在 `inspur-01` 已缓存：`crater-harbor.act.buaa.edu.cn/tianshu/corex:4.3.0`
- image digest：`sha256:b9881c1f7568cf2d372b983697d3cc5e0d30c57ecdfb3231956721e334010877`
- 容器内 `python` 不存在，应使用 `python3`
- `COREX_VERSION=4.3.0`
- `PYTHONPATH=/usr/local/corex-4.3.0/lib64/python3/dist-packages`
- `LD_LIBRARY_PATH=/usr/local/corex-4.3.0/lib64:/usr/local/openmpi/lib:/usr/local/lib:`
- `ILUVATAR_COREX_VISIBLE_DEVICES` 由 Device Plugin 注入，值为 GPU UUID
- 容器只可见被分配的 `/dev/iluvatarN`
- `ixsmi --query-gpu=index,uuid --format=csv` 在容器内可用，且只返回容器可见设备

PyTorch 查询结果：

```text
torch_version 2.4.1
torch_cuda_version 10.2
cuda_is_available True
cuda_device_count 1
distributed_is_available True
nccl_is_available True
gloo_is_available True
device_name Iluvatar BI-V150
backward_ok True
```

CoreX PyTorch 使用 `torch.cuda` 兼容路径；DDP 第一候选 backend 应按 CUDA 兼容栈使用 `nccl`。

## 官方资料调研

2026-04-23 检索天数智芯官网和官方社区资料，结论如下：

- 天数智芯官网有“文档中心”和“资源中心”，入口位于 `support.iluvatar.com`。公开页面本身是登录型支持平台，搜索引擎无法直接索引具体安装文档。
- 官网“天数智算软件栈”页面说明软件栈包含深度学习编程框架、函数库加速层、编译器、调试调优工具、运行库和驱动，并支持主流 Linux 发行版。
- 官网链接的 DeepSpark 开源社区提供了 Iluvatar 容器镜像构建说明。该说明要求：
  - x86_64 Linux
  - Docker
  - 已安装驱动
  - 从 NVIDIA 获取 CUDA Toolkit 10.2 Linux 离线安装包
  - 从天数智芯官网资源中心获取 Linux 版 CoreX 软件栈离线安装包
  - 离线包中包含 Python 版本目录和 whl 包
  - 使用 `corex-installer-***.run` 与 `cuda_***_linux.run` 作为 Docker build 输入
- DeepSpark 的 Kubernetes device-plugin 示例使用镜像 `corex:4.3.0`，资源请求为 `iluvatar.com/gpu: 1`。
- Paddle FastDeploy 的 Iluvatar CoreX 文档也采用“厂商镜像 + `/dev` + `/lib/modules` + privileged”的容器启动方式，并在构建时显式使用 `/usr/local/corex/lib64/libcuda.so.1`。

目前没有在公开官网资料中找到：

- 仅安装 PyTorch backend 的最小 Dockerfile。
- 可公开下载的 CoreX PyTorch wheel 索引。
- CoreX 4.3.0 对应的 PyTorch、torchvision、torchaudio wheel 文件名矩阵。

因此，最可行路线不是从公网 pip 安装，而是基于天数智芯资源中心的离线软件栈包拆分。

参考入口：

- `https://www.iluvatar.com/software?fullCode=cpjs-rj-rjz`
- `https://support.iluvatar.com/`
- `https://www.deepspark.org.cn/`
- `https://gitee.com/deep-spark/ix-device-plugin/blob/master/corex-example.yaml`
- `https://gitee.com/gongyafei/deepsparkhub/blob/master/docker/Iluvatar/README.md`

## Backend 镜像建议

第一阶段先用可工作的 CoreX PyTorch 环境打通验证：

```text
tianshu/corex:4.3.0
  + smoke/training scripts
  + 必要的 Python 依赖固化
  -> iluvatar-bi-v150-pytorch-backend:corex430-py<py>-torch241-v1
```

原因：

- 先验证 toolkit + CoreX PyTorch + 多卡通信是否闭环。
- 避免一开始卡在厂商 wheel、runtime 包来源和版本矩阵上。
- L3-L6 通过后，再基于实际依赖做镜像瘦身。

第二阶段再做最小化 backend：

- 保留 Python、CoreX 适配 PyTorch、torchvision/torchaudio 可选、CoreX runtime、通信库、必要 Python 依赖。
- 由于当前宿主机 CoreX 用户态目录不可依赖，Iluvatar backend 镜像应包含 CoreX runtime，不应只包含 PyTorch wheel。
- 移除 vLLM、DeepSpeed、Megatron、Transformers、Accelerate、Flash Attention、APEX、Triton 等上层框架，除非测试目标明确需要。
- 不把模型权重、数据集、业务代码打入 backend。

如果天数提供更小的 PyTorch/CoreX 官方镜像或 wheel 安装包，应优先使用官方最小基底，而不是从 `tianshu/corex:4.3.0` 删除文件。删除已存在镜像层中的大文件不会真正降低底层镜像体积。

## 最小化构建路线

建议把后续工作拆成两步。

第一步：复现官方离线包构建方式。

```text
ubuntu:22.04
  + CUDA Toolkit 10.2 runtime subset
  + CoreX 4.3.0 software stack runtime subset
  + CoreX PyTorch / torchvision / torchaudio whl
  + smoke and portable training scripts
```

构建输入需要从 `support.iluvatar.com` 获取：

```text
corex-installer-*.run
Python whl package directory
```

同时准备：

```text
cuda_10.2.*_linux.run
```

第二步：从离线包中裁剪 backend 内容。

最小 backend 应保留：

- Python 运行时。
- CoreX runtime：`/usr/local/corex-4.3.0/lib64` 中 PyTorch 执行所需的 CUDA 兼容库、CoreX runtime、NCCL/cuDNN/cuBLAS 兼容库。
- CoreX PyTorch 包：`torch`、`torchvision`、`torchaudio` 的 `+corex` 版本。
- `ixsmi` 和必要诊断工具。
- OpenMPI 只在 L5/DDP 或多节点训练确认为必须时保留。

最小 backend 可先排除：

- vLLM。
- DeepSpeed。
- Megatron-DeepSpeed。
- Transformers / Accelerate。
- Flash Attention / APEX / Triton。
- compiler、header、sample、document。

需要注意：CoreX PyTorch 当前表现为 CUDA 兼容 backend，不能安装普通 PyPI/官方 CUDA PyTorch 替代 `+corex` PyTorch；否则大概率只能看到 CUDA ABI，无法使用 Iluvatar kernel。

## 不建议复制宿主机 CoreX 作为 backend

不建议把 `inspur-01` 的 `/usr/local/corex` 直接复制进 backend 镜像作为主要方案。

原因：

- 历史记录显示宿主机 `/usr/local/corex` 更偏 driver 层，只有少量驱动库和工具，不包含 PyTorch `2.4.1+corex.4.3.0`。
- driver 层应由 toolkit 从宿主机注入，以匹配内核驱动版本。
- PyTorch backend 需要的是 CoreX 适配 PyTorch 和用户态计算/通信依赖，这应来自官方镜像或官方安装包。

## 仍需在 inspur-01 确认

进入 L5/L6 前至少确认：

- 节点 CoreX driver 版本是否仍为 `4.3.0`。
- 多卡 DDP 是否能直接用 `nccl` 完成初始化和 all-reduce。
- 统一 `xpu-runtime` 在 `inspur-01` 上重新部署后是否能替代旧的 `ix` runtime。

## 验证顺序

沿用 Ascend 910B 的验证分层：

| Level | 内容 | Iluvatar 关注点 |
|---|---|---|
| L3 | import、tensor ops、backward、短训练 | `torch.cuda.is_available()` 或厂商 API 是否可用 |
| L4 | 单卡 CNN/MLP | 基础算子和 autograd |
| L5 | 单机 2 卡 DDP | CoreX 通信 backend、UUID 选择、多进程可见设备 |
| L6 | Tiny Transformer | attention/embedding/layernorm 等组合算子 |

L3-L6 通过后，才能认为 Iluvatar PyTorch backend 基线可用。
