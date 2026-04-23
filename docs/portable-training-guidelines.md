# 可迁移训练代码规范

> 更新日期：2026-04-23

## 目标

同一份训练代码应能在不同 PyTorch backend 上运行。代码可以按硬件选择设备和分布式 backend，但不应把 CUDA/NCCL 或某个厂商路径写死在模型实现中。

## 必须满足

- device 从配置、环境或 runtime 能力推导，不写死 `cuda`。
- 分布式 backend 可配置，不写死 `nccl`。
- Ascend NPU 使用 `hccl`，CPU/debug 使用 `gloo`。
- checkpoint 加载先落到 CPU：`torch.load(path, map_location="cpu")`。
- checkpoint 保存优先使用 `model.state_dict()`。
- dtype/AMP 策略可配置，默认先使用 `fp32` 验证可用性。
- 训练入口能在 synthetic data 上运行，便于隔离数据集和网络问题。
- 日志输出应包含 device、backend、world size、step、loss。

## 应避免

- `tensor.cuda()`、`model.cuda()`、`torch.device("cuda")`。
- `dist.init_process_group("nccl")`。
- CUDA-only 依赖：
  - `flash-attn`
  - Triton CUDA kernel
  - 自定义 CUDA extension
  - `bitsandbytes`
  - 仅支持 CUDA 的 fused optimizer
- checkpoint 中保存 CUDA tensor、CUDA graph 或设备绑定 optimizer 状态。

## 推荐写法

Device 选择：

```python
def resolve_device(torch, requested="auto", local_rank=0):
    if requested != "auto":
        return torch.device(requested)
    if hasattr(torch, "npu") and torch.npu.is_available():
        torch.npu.set_device(local_rank)
        return torch.device(f"npu:{local_rank}")
    if torch.cuda.is_available():
        torch.cuda.set_device(local_rank)
        return torch.device(f"cuda:{local_rank}")
    return torch.device("cpu")
```

分布式 backend 选择：

```python
def resolve_backend(device, requested="auto"):
    if requested != "auto":
        return requested
    if device.type == "npu":
        return "hccl"
    if device.type == "cuda":
        return "nccl"
    return "gloo"
```

Checkpoint：

```python
torch.save(model.state_dict(), path)

state = torch.load(path, map_location="cpu")
model.load_state_dict(state)
model.to(device)
```

## 验证分级

| Level | 内容 |
|---|---|
| L3 | backend smoke：import、tensor ops、backward、短训练 |
| L4 | 单卡 CNN/MLP 真实训练路径 |
| L5 | 单机多卡 DDP/HCCL |
| L6 | Transformer 类模型训练路径 |
| L7 | 更长 step、更多卡或真实数据集 |

## 当前适配样本

- `experiments/ascend-910b/pytorch-backend/training_tests/portable_resnet_train.py`
- `experiments/ascend-910b/pytorch-backend/training_tests/tiny_transformer_train.py`
