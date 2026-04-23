# Ascend 910B Minimal PyTorch Backend

> 更新日期：2026-04-22

## 目标

当前 `experiments/ascend-910b/pytorch-backend/Dockerfile` 基于已有 vLLM Ascend 镜像扩展，只适合验证 runtime 注入链路，不作为最终 PyTorch backend 基线。

最小 PyTorch backend 的职责边界：

- 镜像内提供 Python、PyTorch、`torch_npu` 和 smoke test。
- toolkit/profile 注入设备节点、宿主机 Ascend 驱动库、CANN 路径和运行时环境变量。
- 镜像不内置 vLLM、Transformers、DeepSpeed、用户代码、模型权重、数据或 `/dev/davinci*`。

## 官方版本依据

参考：

- Ascend Extension for PyTorch 7.3.0 二进制安装文档：<https://www.hiascend.com/document/detail/zh/Pytorch/730/configandinstg/instg/docs/zh/installation_guide/installing_PyTorch.md>
- Ascend Extension for PyTorch 7.3.0 安装前准备：<https://www.hiascend.com/document/detail/zh/Pytorch/730/configandinstg/instg/docs/zh/installation_guide/preparing_installation.md>
- Ascend PyTorch 仓库版本配套表：<https://github.com/Ascend/pytorch>

关键约束：

- `torch_npu` 安装前需要有配套 CANN/驱动/固件环境；容器运行时由 toolkit 注入宿主机 CANN 环境。
- 官方建议 Python 3.11 及以上有更好的调度性能。
- `torch_npu` 版本需要匹配 CANN 版本：
  - CANN 8.3.RC1：`torch 2.7.1` + `torch_npu 2.7.1`，分支 `v2.7.1-7.2.0`。
  - CANN 8.5.0：`torch 2.7.1` + `torch_npu 2.7.1.post2`，分支 `v2.7.1-7.3.0`。

## 当前默认选择

`kunlun-02` 宿主机 CANN 已确认为 8.5.0，因此默认构建 7.3.0 配套线：

```text
Python: 3.11
PyTorch: 2.7.1+cpu
torch_npu: 2.7.1.post2
CANN userspace: ascend-toolkit 8.5.0, including OPP
arch: linux/arm64
```

原因：当前 `kunlun-02` 已通过的 seed smoke 日志显示 PyTorch 为 `2.7.1+cpu`，而官方配套表中 `torch_npu 2.7.1.post2` 对应 CANN 8.5.0。

当前实验版从 `kunlun-02` 宿主机打包 CANN toolkit，并在构建时解包到 backend 镜像：

```text
/usr/local/Ascend/ascend-toolkit
```

最终镜像不继承 seed 镜像的 vLLM、Transformers、业务代码或模型内容。

## 构建

先从 `kunlun-02` 打包 CANN toolkit：

```bash
mkdir -p experiments/ascend-910b/pytorch-backend/cann

kubectl run ascend-cann-pack \
  -n crater-workspace \
  --image=crater-harbor.act.buaa.edu.cn/docker.io/library/busybox:1.36 \
  --restart=Never \
  --overrides='{"spec":{"nodeSelector":{"kubernetes.io/hostname":"kunlun-02"},"containers":[{"name":"ascend-cann-pack","image":"crater-harbor.act.buaa.edu.cn/docker.io/library/busybox:1.36","command":["sleep","3600"],"volumeMounts":[{"name":"ascend","mountPath":"/host/usr/local/Ascend","readOnly":true}]}],"volumes":[{"name":"ascend","hostPath":{"path":"/usr/local/Ascend","type":"Directory"}}]}}'

kubectl wait -n crater-workspace --for=condition=Ready pod/ascend-cann-pack --timeout=60s

kubectl exec -n crater-workspace ascend-cann-pack -- \
  tar -C /host/usr/local/Ascend -czf - cann-8.5.1 cann ascend-toolkit \
  > experiments/ascend-910b/pytorch-backend/cann/ascend-toolkit-8.5.0.tar.gz

kubectl delete pod -n crater-workspace ascend-cann-pack --ignore-not-found=true
```

然后构建 backend 镜像：

```bash
docker buildx build \
  --builder crater-buildkit \
  --platform linux/arm64 \
  -f experiments/ascend-910b/pytorch-backend/Dockerfile.minimal \
  --build-arg APT_MIRROR=https://mirrors.tuna.tsinghua.edu.cn/debian \
  -t crater-harbor.act.buaa.edu.cn/xpu-huangsy/ascend-910b-pytorch-backend:minimal-cann850-py311-torch271-npu271post2 \
  --push \
  experiments/ascend-910b/pytorch-backend
```

如果远程 BuildKit 访问 PyPI 较慢，可以传入 pip 镜像源或代理：

```bash
docker buildx build \
  --builder crater-buildkit \
  --platform linux/arm64 \
  -f experiments/ascend-910b/pytorch-backend/Dockerfile.minimal \
  --build-arg APT_MIRROR=https://mirrors.tuna.tsinghua.edu.cn/debian \
  --build-arg PIP_INDEX_URL=https://pypi.tuna.tsinghua.edu.cn/simple \
  --build-arg PIP_TRUSTED_HOST=pypi.tuna.tsinghua.edu.cn \
  -t crater-harbor.act.buaa.edu.cn/xpu-huangsy/ascend-910b-pytorch-backend:minimal-cann850-py311-torch271-npu271post2 \
  --push \
  experiments/ascend-910b/pytorch-backend
```

如果集群内有 PyTorch wheel 缓存，也可以改写 `TORCH_WHL_URL`，避免直接访问 `download.pytorch.org`。

如需回退验证 CANN 8.3.RC1 配套线，可改用 `torch_npu 2.7.1`：

```bash
docker buildx build \
  --builder crater-buildkit \
  --platform linux/arm64 \
  -f experiments/ascend-910b/pytorch-backend/Dockerfile.minimal \
  --build-arg APT_MIRROR=https://mirrors.tuna.tsinghua.edu.cn/debian \
  --build-arg TORCH_NPU_VERSION=2.7.1 \
  -t crater-harbor.act.buaa.edu.cn/xpu-huangsy/ascend-910b-pytorch-backend:minimal-py311-torch271-npu271 \
  --push \
  experiments/ascend-910b/pytorch-backend
```

## 验证

```bash
kubectl delete job -n crater-workspace ascend-910b-backend-smoke-minimal --ignore-not-found=true
kubectl apply -f experiments/ascend-910b/pytorch-backend/k8s/smoke-job-minimal.yaml
kubectl logs -n crater-workspace job/ascend-910b-backend-smoke-minimal
```

成功标准仍然是 L3：

- runtime 注入 `ASCEND_VISIBLE_DEVICES`、`/dev/davinciN` 和控制设备。
- `import torch`、`import torch_npu` 成功。
- `torch.npu.is_available()` 为 true。
- 单卡 MLP 训练 10 step 完成。
