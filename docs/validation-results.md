# 验证结果

> 更新日期：2026-04-23

## Iluvatar 早期验证

早期验证确认了项目基本假设：

- Device Plugin 只能解决资源分配和部分设备节点问题。
- 纯净镜像缺少厂商用户态库和 linker 配置时，不能直接使用加速卡。
- runtime/hook 链路可以在容器启动前注入设备、驱动库和工具目录。
- `ld.so.conf.d` 与 `ldconfig` 对动态库发现是必要补充。

这些结论已经沉淀到当前 profile/artifact/linker 设计中，原始过程文档已删除。

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
