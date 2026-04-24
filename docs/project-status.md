# 项目状态

> 更新日期：2026-04-24

## 项目目标

`accelerator-container-toolkit` 的目标是把节点本地加速卡事实收敛到 YAML profile，由统一的 installer/runtime/hook 在 Kubernetes 容器启动前完成设备、驱动库、工具目录、linker 配置和必要环境变量注入。

核心目标：

- 集群入口统一为 `RuntimeClass xpu-runtime`。
- 节点差异由 active profile 表达，而不是散落在代码分支中。
- 用户业务镜像不需要了解设备节点、驱动路径和 OCI hook 细节。
- PyTorch、vLLM、DeepSpeed 等框架能力由 backend/业务镜像提供，不由 toolkit 安装。

## 组件边界

| 组件 | 职责 |
|---|---|
| `accelerator-installer` | DaemonSet init container，复制宿主机二进制、写 active profile、patch containerd、打节点标签 |
| `accelerator-container-runtime` | OCI runtime shim，拦截 `create`，注入 prestart hook、profile extra env 和 OCI linux devices |
| `accelerator-container-hook` | prestart hook，按 profile 注入设备、库、工具、linker 配置 |
| `accelerator-profile-render` | 从 profile 渲染 `RuntimeClass` 和 DaemonSet 部署清单 |

## 当前已完成

- profile 已成为主输入，核心组件均从 active profile 构造运行视图。
- `profiles/iluvatar-bi-v150.yaml`、`profiles/ascend-910b.yaml` 和 `profiles/metax-c500.yaml` 是当前 profile 样本。
- runtime handler 和 RuntimeClass 已统一为 `xpu-runtime`。
- DaemonSet/RuntimeClass 可由 profile 渲染，`make deploy` 是当前部署入口。
- runtime 已支持基于 profile selector env 注入 OCI `linux.devices` 和 device cgroup。
- hook 已支持 profile artifact 注入、`ld.so.conf.d` 写入和 `ldconfig`。
- Ascend 910B 已完成 CANN backend L3 smoke test：`torch_npu` 可用，单卡训练 10 step 完成。
- Metax C500 已完成 MACA PyTorch backend L3/L4/L5/L6 验证，并已通过 `xpu-runtime` L3 smoke 与 2 卡 DDP 验证。

## 当前边界

toolkit 负责：

- 设备节点和控制设备注入。
- 宿主机驱动库与工具路径注入。
- OCI hook、OCI device/cgroup 和 linker 配置。
- `ASCEND_VISIBLE_DEVICES` 等运行时选择器环境传递。

toolkit 不负责：

- 安装 PyTorch、`torch_npu`、vLLM、DeepSpeed 等框架。
- 安装完整 CANN 用户态到任意业务镜像。
- 适配 CUDA-only 算子或修改用户训练代码。
- 保证不同 backend 数值完全一致。

## 当前风险

- `profiles/ascend-910b.yaml` 中仍包含一部分 CANN/toolkit 路径注入。对于官方 CANN backend 镜像，这些路径主要起补充和兼容作用，后续可继续瘦身。
- `npu-smi` 当前 smoke 日志中仍未出现在容器 `PATH`，但不影响 PyTorch L3 smoke。
- Metax C500 的 xpu-runtime 接入当前需要显式设置 `METAX_VISIBLE_DEVICES=all` 触发 hook；自动从扩展资源推导 selector 仍需单独设计。
- 310P profile 仍是后续工作，不作为当前 910B 闭环阻塞项。
