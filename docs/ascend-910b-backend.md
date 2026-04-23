# Ascend 910B Backend

> 更新日期：2026-04-23

## 目标

Ascend 910B 训练闭环拆成两层：

```text
backend 镜像：CANN 用户态 + Python + PyTorch + torch_npu + smoke test
toolkit：设备节点 + 控制设备 + 宿主机驱动库 + OCI runtime/cgroup 注入
```

## 当前通过镜像

```text
crater-harbor.act.buaa.edu.cn/xpu-huangsy/ascend-910b-pytorch-backend:cann851-py311-torch271-npu271post2
```

基础镜像：

```text
swr.cn-south-1.myhuaweicloud.com/ascendhub/cann:8.5.1-910b-ubuntu22.04-py3.11
```

软件版本：

- CANN：8.5.1
- Python：3.11
- PyTorch：2.7.1+cpu
- torch_npu：2.7.1.post2
- CANN Python 编译依赖：`absl-py`, `attrs`, `cloudpickle`, `decorator`, `ml-dtypes`, `psutil`, `scipy`, `tornado`

Dockerfile：

```text
experiments/ascend-910b/pytorch-backend/Dockerfile.cann
```

Smoke Job：

```text
experiments/ascend-910b/pytorch-backend/k8s/smoke-job-minimal.yaml
```

## 手动构建

在 `kunlun-02` 上构建：

```bash
docker build \
  -f experiments/ascend-910b/pytorch-backend/Dockerfile.cann \
  -t crater-harbor.act.buaa.edu.cn/xpu-huangsy/ascend-910b-pytorch-backend:cann851-py311-torch271-npu271post2 \
  experiments/ascend-910b/pytorch-backend
```

上传：

```bash
docker push \
  crater-harbor.act.buaa.edu.cn/xpu-huangsy/ascend-910b-pytorch-backend:cann851-py311-torch271-npu271post2
```

如果节点使用 `nerdctl`，将 `docker` 替换为 `nerdctl`。

## Smoke 验证

执行：

```bash
kubectl delete job -n crater-workspace ascend-910b-backend-smoke-minimal --ignore-not-found=true
kubectl apply -f experiments/ascend-910b/pytorch-backend/k8s/smoke-job-minimal.yaml
kubectl logs -n crater-workspace job/ascend-910b-backend-smoke-minimal
```

2026-04-23 已完成 L3 smoke：

- Pod 调度到 `kunlun-02`
- `ASCEND_VISIBLE_DEVICES=7`
- `/dev/davinci7` 可见
- 控制设备可见
- `torch_npu_imported: true`
- `torch.npu.is_available(): true`
- `npu_device_count: 1`
- tensor ops 通过
- backward 通过
- 10 step train 通过

2026-04-23 已完成 portable training 验证：

- L4：Tiny ResNet-like CNN + synthetic CIFAR shape，单卡 NPU，20 step 通过
- L5：同一训练脚本，单机 2 卡 DDP，`backend=hccl`，20 step 通过

训练脚本：

```text
experiments/ascend-910b/pytorch-backend/training_tests/portable_resnet_train.py
```

训练 Job：

```text
experiments/ascend-910b/pytorch-backend/k8s/train-single-job.yaml
experiments/ascend-910b/pytorch-backend/k8s/train-ddp-2card-job.yaml
```

注意：L4/L5 验证过程中发现官方 CANN 基础镜像缺少部分 Python 编译依赖。当前 `Dockerfile.cann` 已补齐这些依赖；需要重建并覆盖 backend tag 后，训练 Job 才能直接运行。

## 后续优化

- 删除重复的 CANN 环境变量片段。
- 评估 `profiles/ascend-910b.yaml` 中 CANN 用户态路径是否可以从 toolkit 注入中移除。
- 保留 driver 层由宿主机注入，CANN/PyTorch 层由 backend 镜像提供。
- 用同一 tag 重建 backend，固化 CANN Python 编译依赖。
