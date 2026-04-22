# Ascend 910B Runtime 与 PyTorch Backend 闭环记录

> 更新日期：2026-04-22
> 目标：围绕 Ascend 910B，推进“toolkit 运行时注入 + PyTorch backend 镜像 + smoke test”的第一条训练闭环。

## 1. 当前目标

先不扩展到所有国产卡，当前只验证 Ascend 910B：

```text
backend 镜像提供 PyTorch/torch_npu 框架能力
toolkit profile 注入 910B 设备、驱动库、工具和环境变量
同一份 smoke test 在 xpu-runtime 下完成单卡训练 10 step
```

## 2. 已确认的 910B 集群事实

当前 kube context：

```text
kubernetes-admin@kubernetes
```

910B 节点：

| 项 | 值 |
|---|---|
| 节点 | `kunlun-02` |
| 架构 | `arm64` |
| OS | `openEuler 22.03 (LTS-SP4)` |
| Kernel | `5.10.0-303.0.0.206.oe2203sp4.aarch64` |
| 容器运行时 | `containerd://1.7.28` |
| 节点标签 | `accelerator=huawei-Ascend910` |
| 芯片标签 | `node.kubernetes.io/npu.chip.name=910B3` |
| 服务器类型 | `servertype=Ascend910B-20` |
| 资源名 | `huawei.com/Ascend910` |
| Capacity | `8` |
| Allocatable | `7` |

当前 910B 相关系统 Pod：

| Namespace | Pod | 节点 | 状态 |
|---|---|---|---|
| `kube-system` | `accelerator-toolkit-9glff` | `kunlun-02` | Running |
| `kube-system` | `ascend-device-plugin-daemonset-tct2w` | `kunlun-02` | Running |

已注册 RuntimeClass：

```text
xpu-runtime -> xpu-runtime
```

当前运行中的 910B workload 样本：

| 项 | 值 |
|---|---|
| Pod | `crater-workspace/jpt-zhouxiao25-260421-56579-default0-0` |
| 节点 | `kunlun-02` |
| 镜像 | `gpu-harbor.act.buaa.edu.cn/user-zhouxiao25/ascend/vllm-ascend:v0.11.0-crater-v0.0.2` |
| 请求资源 | `huawei.com/Ascend910: 2` |
| Device Plugin annotation | `Ascend910-2,Ascend910-3` |

## 3. 当前 backend 镜像准备

已新增 backend 镜像模板：

```text
experiments/ascend-910b/pytorch-backend/Dockerfile
```

当前默认以现有可运行 Ascend 镜像作为 seed：

```text
gpu-harbor.act.buaa.edu.cn/user-zhouxiao25/ascend/vllm-ascend:v0.11.0-crater-v0.0.2
```

这是为了先跑通闭环，不代表最终最小镜像。后续应替换成更干净的 `torch + torch_npu` 基础镜像。

backend 镜像当前只新增：

- smoke test 脚本
- 默认启动命令

不额外打入：

- `/dev/davinci*`
- `/usr/local/Ascend/driver`
- toolkit 已负责注入的宿主机驱动库

## 4. Smoke Test 分级

脚本：

```text
experiments/ascend-910b/pytorch-backend/smoke_tests/ascend_910b_smoke.py
```

分级：

| Level | 验证内容 |
|---|---|
| `l0` | `ASCEND_VISIBLE_DEVICES`、`/dev/davinci*`、控制设备、Ascend 路径、`npu-smi` |
| `l1` | `import torch`、`import torch_npu`、`torch.npu.is_available()` |
| `l2` | NPU tensor、matmul、backward |
| `l3` | 单卡 MLP 训练 10 step |

Kubernetes Job 模板：

```text
experiments/ascend-910b/pytorch-backend/k8s/smoke-job.yaml
```

该 Job 使用：

- `runtimeClassName: xpu-runtime`
- `nodeSelector: accelerator=huawei-Ascend910`
- `huawei.com/Ascend910: 1`

## 5. 下一步执行

1. 构建并推送 backend 镜像：

```bash
docker buildx build \
  --platform linux/arm64 \
  -f experiments/ascend-910b/pytorch-backend/Dockerfile \
  -t crater-harbor.act.buaa.edu.cn/xpu-huangsy/ascend-910b-pytorch-backend:dev \
  --push \
  experiments/ascend-910b/pytorch-backend
```

2. 提交 smoke Job：

```bash
kubectl apply -f experiments/ascend-910b/pytorch-backend/k8s/smoke-job.yaml
```

3. 查看日志：

```bash
kubectl logs -n crater-workspace job/ascend-910b-backend-smoke
```

4. 清理 Job：

```bash
kubectl delete -f experiments/ascend-910b/pytorch-backend/k8s/smoke-job.yaml
```

## 6. 第一阶段成功标准

第一阶段只要求跑通基础闭环：

- Pod 使用 `runtimeClassName: xpu-runtime`
- Pod 调度到 `kunlun-02`
- Device Plugin 注入 `ASCEND_VISIBLE_DEVICES`
- toolkit 注入设备节点、控制设备、Ascend 库路径和 linker 配置
- backend 镜像内 `torch_npu` 可正常 import
- `torch.npu.is_available()` 为 true
- 单卡训练 10 step 完成

性能优化、镜像瘦身、多卡 DDP 暂不作为第一阶段完成标准。

