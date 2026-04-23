# Backend 镜像制作注意事项

> 更新日期：2026-04-23

## 目标

backend 镜像负责让 PyTorch 能使用某类加速卡；toolkit 负责让容器看见节点上的设备和宿主机驱动事实。两者不要混在一起。

## 镜像应包含

- OS / Python 运行时。
- PyTorch。
- 厂商 PyTorch 扩展，例如 `torch_npu`。
- 与厂商 PyTorch 扩展强绑定的用户态计算库。
- 通信库用户态依赖，例如 HCCL/NCCL/MCCL 等。
- 必要 Python 依赖。
- 最小 smoke / training 验证脚本。

Ascend 910B 当前基线：

```text
CANN 8.5.1
Python 3.11
PyTorch 2.7.1+cpu
torch_npu 2.7.1.post2
```

## 镜像不应包含

- 设备节点，如 `/dev/davinci*`。
- 宿主机内核驱动或内核模块。
- 用户业务代码。
- 模型权重和训练数据。
- vLLM、DeepSpeed、Transformers 等上层训练/推理框架，除非该镜像明确是 framework 层而不是 backend 层。
- CUDA-only 或厂商不支持的扩展。

## toolkit 应负责

- 设备节点和控制设备注入。
- 宿主机 driver 层库注入。
- OCI device/cgroup 配置。
- selector env，例如 `ASCEND_VISIBLE_DEVICES`。
- RuntimeClass 和 hook 链路。

## 版本匹配

backend 镜像必须记录版本矩阵：

| 项 | 示例 |
|---|---|
| 硬件 | Ascend 910B |
| Driver | 节点实际版本 |
| 用户态 SDK | CANN 8.5.1 |
| Python | 3.11 |
| PyTorch | 2.7.1 |
| 厂商扩展 | torch_npu 2.7.1.post2 |
| 通信 backend | HCCL |

不确认版本匹配时，不要直接进入真实训练；先跑 L3 smoke。

## 依赖固化

不要依赖 Job 启动时临时 `pip install`。训练过程中暴露出的依赖必须回写 Dockerfile。

Ascend 910B 真实 CNN 训练曾暴露 CANN 编译依赖缺口，最终固化：

```text
absl-py
attrs
cloudpickle
decorator
ml-dtypes
numpy
psutil
pyyaml
scipy
setuptools
tornado
wheel
```

## 验证矩阵

每个 backend 至少通过：

| Level | 内容 |
|---|---|
| L3 | import、tensor ops、backward、短训练 |
| L4 | 单卡 CNN/MLP 训练 |
| L5 | 单机 2 卡 DDP，验证通信 backend |
| L6 | Transformer 类模型训练 |

通过后记录：

- 镜像 tag 和 digest。
- 运行节点。
- 设备数量。
- backend 通信库。
- 关键 loss 和 step 数。
- 是否有临时依赖安装。

## Tag 与发布

- 开发时可以用可覆盖 tag。
- 阶段通过后必须打不可变 tag。
- 文档中记录 digest，避免“同名 tag 指向旧镜像”。

推荐格式：

```text
<hardware>-pytorch-backend:<sdk>-py<python>-torch<torch>-<extension>-v<N>
```

示例：

```text
ascend-910b-pytorch-backend:cann851-py311-torch271-npu271post3-v1
```

## 迁移到新加速卡时的顺序

1. 找官方最小 SDK/runtime 镜像或安装包。
2. 确认 PyTorch 与厂商扩展版本矩阵。
3. 构建 backend 镜像，只包含 backend 层。
4. 编写 L3 smoke。
5. 跑 L4 单卡训练。
6. 跑 L5 多卡通信。
7. 跑 L6 Transformer。
8. 再考虑框架层和业务镜像。
