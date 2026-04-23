# Ascend 910B CANN Backend 手动构建

> 适用节点：`kunlun-02`

## 目标

基于华为昇腾官方 CANN Ubuntu 镜像构建 PyTorch backend：

```text
swr.cn-south-1.myhuaweicloud.com/ascendhub/cann:8.5.1-910b-ubuntu22.04-py3.11
```

backend 镜像包含：

- CANN 8.5.1 用户态
- Python 3.11
- PyTorch 2.7.1
- torch_npu 2.7.1.post2
- smoke test 脚本

toolkit 仍负责注入：

- `/dev/davinci*`
- Ascend 控制设备
- 宿主机驱动库
- `ASCEND_VISIBLE_DEVICES`
- OCI runtime / cgroup device 配置

## 文件

Dockerfile：

```text
experiments/ascend-910b/pytorch-backend/Dockerfile.cann
```

构建上下文：

```text
experiments/ascend-910b/pytorch-backend
```

## 构建

进入仓库根目录：

```bash
cd /path/to/ix-container-toolkit
```

使用 Docker：

```bash
docker build \
  -f experiments/ascend-910b/pytorch-backend/Dockerfile.cann \
  -t crater-harbor.act.buaa.edu.cn/xpu-huangsy/ascend-910b-pytorch-backend:cann851-py311-torch271-npu271post2 \
  experiments/ascend-910b/pytorch-backend
```

如果节点使用 `nerdctl`：

```bash
nerdctl build \
  -f experiments/ascend-910b/pytorch-backend/Dockerfile.cann \
  -t crater-harbor.act.buaa.edu.cn/xpu-huangsy/ascend-910b-pytorch-backend:cann851-py311-torch271-npu271post2 \
  experiments/ascend-910b/pytorch-backend
```

## 上传

使用 Docker：

```bash
docker push \
  crater-harbor.act.buaa.edu.cn/xpu-huangsy/ascend-910b-pytorch-backend:cann851-py311-torch271-npu271post2
```

使用 `nerdctl`：

```bash
nerdctl push \
  crater-harbor.act.buaa.edu.cn/xpu-huangsy/ascend-910b-pytorch-backend:cann851-py311-torch271-npu271post2
```

## Smoke Job 镜像

将 smoke Job 镜像改为：

```yaml
image: crater-harbor.act.buaa.edu.cn/xpu-huangsy/ascend-910b-pytorch-backend:cann851-py311-torch271-npu271post2
```

然后执行：

```bash
kubectl delete job -n crater-workspace ascend-910b-backend-smoke-minimal --ignore-not-found=true
kubectl apply -f experiments/ascend-910b/pytorch-backend/k8s/smoke-job-minimal.yaml
kubectl logs -n crater-workspace job/ascend-910b-backend-smoke-minimal
```

## 成功标准

日志中应看到：

- `torch_npu_imported: true`
- `npu_is_available: true`
- `npu_device_count: 1`
- `tensor_ops`
- `backward`
- `train_steps` 完成 10 step
