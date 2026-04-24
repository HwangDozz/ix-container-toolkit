# 验证结果

> 更新日期：2026-04-24

## Iluvatar 早期验证

早期验证确认了项目基本假设：

- Device Plugin 只能解决资源分配和部分设备节点问题。
- 纯净镜像缺少厂商用户态库和 linker 配置时，不能直接使用加速卡。
- runtime/hook 链路可以在容器启动前注入设备、驱动库和工具目录。
- `ld.so.conf.d` 与 `ldconfig` 对动态库发现是必要补充。

这些结论已经沉淀到当前 profile/artifact/linker 设计中，原始过程文档已删除。

## NVIDIA A100 xpu-runtime Delegate 验证

验证时间：2026-04-24

镜像：

```text
crater-harbor.act.buaa.edu.cn/nvcr.io/nvidia/cuda:12.6.2-base-ubi9
```

installer 镜像：

```text
crater-harbor.act.buaa.edu.cn/xpu-huangsy/accelerator-toolkit-installer:nvidia-a100-delegate-wrapper-20260424
```

installer digest：

```text
sha256:1a80ffd41ab11e23d023a846bd117fb60a1b2ec3fdfa0daa7fba6d9286eaf0e4
```

Job：

```text
crater-workspace/nvidia-a100-runtime-query
crater-workspace/nvidia-a100-xpu-runtime-query
```

节点：

```text
inspur-gpu-04
```

资源规格：

- `nvidia.com/a100: 1`
- `cpu: 2`
- `memory: 4Gi`

已确认：

- `RuntimeClass nvidia` 基线 Job 通过。
- `RuntimeClass xpu-runtime` delegate Job 通过。
- active profile：`profiles/nvidia-a100.yaml`
- `runtime.injectMode: delegate-only`
- `underlyingRuntime: /usr/local/nvidia/toolkit/nvidia-container-runtime`
- 容器内 `NVIDIA_VISIBLE_DEVICES=/var/run/nvidia-container-devices`
- 容器内只看到 1 张 `NVIDIA A100-PCIE-40GB`
- `nvidia-smi` 正常。
- NVIDIA driver libraries 由 NVIDIA runtime 挂载到容器内。

备注：

- 使用 `/usr/bin/nvidia-container-runtime` 作为 underlying runtime 会失败，因为它不能正确处理当前 device plugin 的 `volume-mounts` selector。
- 首次向 `inspur-gpu-04` 写入 `xpu-runtime` handler 后重启过 containerd；后续更新 active profile/config 不需要重启。

## Metax C500 MACA PyTorch Backend 验证

验证时间：2026-04-24

镜像：

```text
cr.metax-tech.com/public-library/maca-pytorch:3.5.3.6-torch2.4-py310-kylinv10-arm64
```

Job：

```text
crater-workspace/metax-c500-backend-smoke-v2
crater-workspace/metax-c500-portable-train-single
crater-workspace/metax-c500-portable-train-transformer
crater-workspace/metax-c500-portable-train-ddp-2card
crater-workspace/metax-c500-xpu-runtime-smoke
crater-workspace/metax-c500-xpu-runtime-train-ddp-2card
```

节点：

```text
greatwall-02
```

资源规格：

- `metax-tech.com/gpu: 2`
- `cpu: 16`
- `memory: 128Gi`

已确认：

- 所有验证 Job 均 `Completed`
- `RuntimeClass metax` 可用
- `/dev/mxcd` 可见
- `mx-smi` 可见并识别 2 张 `MetaX C500`
- `torch 2.4.0+metax3.5.3.9`
- `torch.version.cuda = 11.6`
- `torch.cuda.is_available() = true`
- `torch.cuda.device_count() = 2`

L3 smoke 结果：

- 设备：`cuda:0`
- tensor matmul/backward 完成
- 10 step MLP 训练完成
- first loss：`2.3358445167541504`
- final loss：`2.336742877960205`
- 结果：通过

L4 单卡 portable CNN 结果：

- 脚本：`experiments/ascend-910b/pytorch-backend/training_tests/portable_resnet_train.py`
- 设备：`cuda:0`
- steps：20
- first loss：`2.415478229522705`
- final loss：`2.242485284805298`
- 结果：通过

L6 Tiny Transformer 结果：

- 脚本：`experiments/ascend-910b/pytorch-backend/training_tests/tiny_transformer_train.py`
- 设备：`cuda:0`
- steps：20
- first loss：`7.771679878234863`
- final loss：`7.821425914764404`
- 结果：通过

L5 两卡 DDP 结果：

- `torch.distributed.run --nproc_per_node=2`
- backend：`nccl`
- world size：2
- device：`cuda:0` / `cuda:1`
- steps：20
- rank 0 final loss：`2.2541959285736084`
- avg final loss：`2.2903876304626465`
- 结果：通过

xpu-runtime L3 smoke 结果：

- installer 镜像：`crater-harbor.act.buaa.edu.cn/xpu-huangsy/accelerator-toolkit-installer:metax-c500-envall-20260424`
- installer digest：`sha256:0863d3f12114b08863c3c6dfe3a8e5e1a26b16a14926dd5032789b95cd4e1493`
- active profile：`profiles/metax-c500.yaml`
- `runtimeClassName: xpu-runtime`
- Job 未显式设置 `METAX_VISIBLE_DEVICES`
- runtime shim 默认注入 selector env：`METAX_VISIBLE_DEVICES=all`
- runtime shim 注入 OCI devices：`count=5`
- prestart hook 注入成功
- `mx-smi` 可见并识别 2 张 `MetaX C500`
- tensor matmul/backward 完成
- 10 step MLP 训练完成
- first loss：`2.3281688690185547`
- final loss：`2.325732707977295`
- 结果：通过

xpu-runtime 两卡 DDP 结果：

- `runtimeClassName: xpu-runtime`
- Job 未显式设置 `METAX_VISIBLE_DEVICES`
- runtime shim 默认注入 selector env：`METAX_VISIBLE_DEVICES=all`
- runtime shim 注入 OCI devices：`count=5`
- `torch.distributed.run --nproc_per_node=2`
- backend：`nccl`
- world size：2
- device：`cuda:0` / `cuda:1`
- steps：20
- rank 0 final loss：`2.289407253265381`
- avg final loss：`2.3003616333007812`
- 结果：通过

备注：

- `4Gi` 内存规格会在 Python/torch 初始化阶段被 OOM kill。
- `1` 张 GPU 规格下，MACA PyTorch 曾出现设备枚举断言；当前可复现通过规格为 `2` 张 GPU。
- 当前 `env-all` 验证中 containerd handler 已存在，installer 未重启 containerd。

## Iluvatar BI-V150 Backend 初测

验证时间：2026-04-23

镜像：

```text
crater-harbor.act.buaa.edu.cn/tianshu/corex:4.3.0
```

节点：

```text
inspur-01
```

已确认：

- Device Plugin 实际资源名：`iluvatar.com/gpu`
- 调度标签：`iluvatar.ai/gpu=present`
- `ILUVATAR_COREX_VISIBLE_DEVICES` 注入值为 GPU UUID
- CoreX PyTorch 使用 `torch.cuda` 兼容路径
- `torch 2.4.1`
- `torch.version.cuda = 10.2`
- `torch.cuda.is_available() = true`
- `torch.distributed.is_nccl_available() = true`

L3 smoke 结果：

- 设备：`cuda:0`
- 10 step tensor/backward 完成
- first loss：`14399.4033203125`
- final loss：`0.0`
- 结果：通过

L4 单卡 portable CNN 结果：

- 脚本：`experiments/ascend-910b/pytorch-backend/training_tests/portable_resnet_train.py`
- 设备：`cuda:0`
- steps：20
- first loss：`2.4377217292785645`
- final loss：`2.250478744506836`
- 结果：通过

L6 Tiny Transformer 结果：

- 脚本：`experiments/ascend-910b/pytorch-backend/training_tests/tiny_transformer_train.py`
- 设备：`cuda:0`
- steps：20
- first loss：`7.771687984466553`
- final loss：`7.821423530578613`
- 结果：通过

L5 两卡 DDP 当前状态：

- `torch.distributed.run --nproc_per_node=2`
- `nccl` 初始化成功
- rank 0 / rank 1 完成 NCCL communicator init
- P2P/IPC ring 已连通
- 训练未在预期时间内结束
- 当前结论：通信初始化通过，训练阶段或后续 collective 仍需排查

## Ascend 910B Profile 验证

已验证：

- `profiles/ascend-910b.yaml` 可以表达 910B 的资源名、selector env、设备节点和控制设备。
- `RuntimeClass xpu-runtime` 能把 Pod 导向 toolkit runtime。
- runtime 能在 OCI spec 中注入 selected `/dev/davinciN` 与控制设备。
- hook 能注入 profile 声明的 artifact、环境变量和 linker 配置。

## Ascend 910B Backend L3 Smoke

验证时间：2026-04-23

镜像：

```text
crater-harbor.act.buaa.edu.cn/xpu-huangsy/ascend-910b-pytorch-backend:cann851-py311-torch271-npu271post3
```

Job：

```text
crater-workspace/ascend-910b-backend-smoke
```

节点：

```text
kunlun-02
```

结果：

- Job Pod `Completed`
- runtime 注入 extra env
- runtime 注入 OCI linux devices
- prestart hook 注入成功
- `/dev/davinci7` 可见
- `/dev/davinci_manager`、`/dev/devmm_svm`、`/dev/hisi_hdc` 可见
- `torch 2.7.1+cpu`
- `torch_npu` import 成功
- `torch.npu.is_available()` 为 true
- 单卡 MLP 训练 10 step 完成

## Portable Training 验证

验证时间：2026-04-23

训练脚本：

```text
experiments/ascend-910b/pytorch-backend/training_tests/portable_resnet_train.py
```

单卡结果：

- Tiny ResNet-like CNN
- synthetic CIFAR shape input
- device：`npu:0`
- steps：20
- first loss：`2.4209210872650146`
- final loss：`2.352341413497925`
- 结果：通过

2 卡 DDP 结果：

- `torch.distributed.run --nproc_per_node=2`
- backend：`hccl`
- world size：2
- device：`npu:0` / `npu:1`
- steps：20
- rank 0 final loss：`2.380683183670044`
- avg final loss：`2.330897331237793`
- 结果：通过

该验证使用 `post3` backend 镜像直接完成，没有在 Job 启动时临时安装依赖。

## Tiny Transformer 验证

验证时间：2026-04-23

训练脚本：

```text
experiments/ascend-910b/pytorch-backend/training_tests/tiny_transformer_train.py
```

单卡结果：

- Tiny Transformer LM
- synthetic token input
- device：`npu:0`
- steps：20
- batch size：8
- sequence length：64
- vocab size：2048
- d_model：128
- layers：2
- first loss：`7.815044403076172`
- final loss：`7.756039619445801`
- 结果：通过

## 当前未作为通过条件

- `npu-smi` 是否在容器 `PATH` 中可见。
- 性能基准。
- backend 镜像瘦身。
