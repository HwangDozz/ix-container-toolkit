# PyTorch Backend 与异构训练可移植性总结

> 更新日期：2026-04-22
> 目标：简要说明 PyTorch backend、训练代码无缝迁移、镜像分层和 accelerator-container-toolkit 的职责边界。

## 1. 核心结论

“同一套训练代码 + 同一份训练数据”可以在不同加速卡上运行，但前提不是一个镜像通吃所有硬件，而是：

```text
设备无关训练代码
+ 各厂商 PyTorch backend 镜像
+ 节点侧 toolkit 注入驱动与设备环境
+ 统一调度入口
```

更准确的目标是：

- 用户不感知设备节点、驱动库、`LD_LIBRARY_PATH`、厂商工具和 RuntimeClass 差异
- 训练代码遵守可移植规范
- 每类硬件有经过验证的 PyTorch backend 镜像
- toolkit 根据节点 profile 自动注入底层运行时事实

## 2. 什么是 PyTorch backend 镜像

PyTorch backend 镜像是“能让 PyTorch 在某类加速卡上运行”的基础镜像。

它通常包含：

- Python
- PyTorch
- 厂商 PyTorch 扩展或适配包
- 厂商计算库用户态依赖
- 厂商通信库依赖
- 必要环境变量
- 基础验证脚本

示例：

| 硬件 | backend 镜像内容 |
|---|---|
| NVIDIA | CUDA 版 PyTorch、CUDA runtime、cuDNN、NCCL |
| 昇腾 | PyTorch、`torch_npu`、CANN 用户态依赖、HCCL |
| 天数 | CoreX 适配的 PyTorch 包、CoreX runtime、通信库依赖 |
| 沐曦 | MACA 适配的 PyTorch 包、mcDNN/mcBLAS/MCCL 等依赖 |

## 3. 为什么需要 PyTorch backend

PyTorch API 本身不会自动知道如何使用每一种国产加速卡。

训练代码调用：

```python
loss = model(input).sum()
loss.backward()
optimizer.step()
```

底层需要 backend 回答：

- tensor 放在哪个设备
- matmul、conv、attention 调哪个算子库
- backward 算子由谁实现
- 显存如何分配
- 多卡通信使用哪个 backend
- AMP 支持哪些 dtype

因此：

```text
toolkit 解决“容器看得见硬件”
PyTorch backend 解决“PyTorch 用得上硬件”
训练代码规范解决“用户代码不绑死硬件”
```

## 4. 可移植训练代码还需要满足什么

即使用户默认遵守“可移植训练代码规范”，要做到无缝迁移仍需要满足以下条件。

### 4.1 算子覆盖

目标 backend 必须支持模型所需算子。风险较高的部分包括：

- custom CUDA op
- flash attention / xformers / triton kernel
- fused optimizer
- 稀疏算子
- 特殊 loss
- 量化算子

### 4.2 dtype 与 AMP 兼容

不同硬件对 `fp32`、`fp16`、`bf16`、`tf32`、`fp8` 的支持不同。

代码中不应硬编码 CUDA AMP 细节，应通过配置选择 precision 策略。

### 4.3 分布式训练 backend 可切换

不能写死：

```python
dist.init_process_group("nccl")
```

应由运行环境选择：

| 硬件 | 通信 backend |
|---|---|
| NVIDIA | NCCL |
| 昇腾 | HCCL |
| 其他国产卡 | 厂商通信库或兼容层 |
| CPU/debug | Gloo |

### 4.4 第三方训练库适配

很多科研代码依赖：

- `transformers`
- `accelerate`
- `deepspeed`
- `megatron`
- `lightning`
- `torchvision`
- `flash-attn`
- `bitsandbytes`

这些库可能默认 CUDA，需要确认目标硬件是否已有适配版或替代实现。

### 4.5 checkpoint 可迁移

推荐保存：

```python
torch.save(model.state_dict(), path)
```

加载时先落到 CPU：

```python
state = torch.load(path, map_location="cpu")
model.load_state_dict(state)
model.to(device)
```

避免 checkpoint 绑定 CUDA tensor、CUDA graph 或特定 fused optimizer 状态。

### 4.6 数值一致性有预期

不同 backend 的 kernel 实现不同，通常不应要求 bitwise 一致。

更合理的验收标准是：

```text
无需修改业务代码即可启动训练，并达到可接受的精度和性能
```

## 5. 镜像分层建议

不建议为每张卡维护完整大镜像。建议分层如下：

```text
base-os
  -> python-runtime
    -> common-ml-deps
      -> pytorch-backend
        -> training-framework
          -> user-code
```

分层原则：

| 层 | 内容 | 是否按厂商区分 |
|---|---|---|
| `base-os` | OS、基础工具、用户、证书 | 尽量不区分 |
| `python-runtime` | Python、pip/uv/conda | 尽量不区分 |
| `common-ml-deps` | numpy、tokenizers、datasets 等 | 尽量不区分 |
| `pytorch-backend` | PyTorch 与厂商适配包 | 必须区分 |
| `training-framework` | deepspeed、megatron、accelerate 等 | 多数需要区分 |
| `user-code` | 训练代码、配置、启动脚本 | 尽量不区分 |

训练数据和模型权重不应进入镜像，应使用 PVC、对象存储或节点缓存。

## 6. toolkit 的职责

`accelerator-container-toolkit` 的职责不是替代 PyTorch backend，而是统一异构加速卡的容器运行时注入。

它负责：

- 根据 profile 识别节点厂商事实
- 注入设备节点
- 注入驱动共享库
- 注入厂商工具目录
- 写入 linker 配置
- 注入必要环境变量
- 统一 `RuntimeClass/handler = xpu-runtime`

它不负责：

- 安装 PyTorch
- 安装 `torch_npu`、MACA PyTorch、CoreX PyTorch 等框架包
- 适配 CUDA-only 算子
- 修改用户训练代码
- 保证不同 backend 数值完全一致

## 7. 推荐系统边界

最终系统应拆成四层契约：

```text
代码契约：
  不写死 cuda/nccl/custom CUDA，device、dtype、distributed backend 从配置读取

框架契约：
  每类卡提供一个经过验证的 PyTorch backend 镜像

运行时契约：
  toolkit 根据节点 profile 注入设备、驱动库、工具和环境变量

验证契约：
  每个 backend 镜像通过同一组 smoke test 和代表性模型训练基准
```

整体目标不是消灭 backend，而是让用户不直接感知 backend。

