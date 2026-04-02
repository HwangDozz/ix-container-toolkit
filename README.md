# ix-container-toolkit

天数（Iluvatar）GPU 的容器运行时注入工具链。项目目标是对标 NVIDIA Container Toolkit，把宿主机上的 GPU 设备节点、驱动库和基础工具按需注入容器，让申请了天数 GPU 的 Pod 可以在不内置宿主机驱动的前提下运行。

## 解决的问题

在 Kubernetes 中，天数官方 Device Plugin 能分配 GPU 资源并把 `/dev/iluvatarN` 注入容器，但它不会自动完成下面这些事情：

- 挂载 `/usr/local/corex` 驱动目录
- 设置 `LD_LIBRARY_PATH`
- 写入 `ld.so.conf.d`
- 注入 OCI hook

因此，纯净镜像即使拿到了 GPU 设备节点，也往往无法直接加载驱动库。

## 当前方案

项目采用三段式架构：

- `ix-container-runtime`：作为 containerd runtime shim，拦截 `create`，按需把 hook 注入 OCI spec
- `ix-container-hook`：作为 OCI prestart hook，在容器启动前挂载设备、驱动库、驱动工具，并刷新 `ld.so.cache`
- `ix-installer`：以 DaemonSet init container 形式把二进制和配置安装到宿主机，并 patch containerd

完整链路如下：

```text
Pod 申请 iluvatar.ai/gpu
        ↓
Device Plugin 注入 /dev/iluvatarN
并设置 ILUVATAR_COREX_VISIBLE_DEVICES=GPU-...
        ↓
containerd 调用 ix-container-runtime create
        ↓
ix-container-runtime 注入 ix-container-hook
        ↓
runc 执行 prestart hook
        ↓
ix-container-hook:
  - UUID -> /dev/iluvatarN 映射
  - bind-mount 设备节点
  - bind-mount /usr/local/corex/lib64、lib、bin
  - 写入 ld.so.conf.d
  - 运行 ldconfig
        ↓
容器进程启动
```

## 已确认的环境事实

基于真实节点验证，当前项目默认假设如下：

- GPU 设备节点为 `/dev/iluvatar0` 到 `/dev/iluvatar7`
- 没有 `/dev/iluvatarctl` 这类控制节点
- 驱动根目录是 `/usr/local/corex`
- 实际安装目录是版本化目录，例如 `/usr/local/corex-4.3.0`
- Device Plugin 使用的环境变量是 `ILUVATAR_COREX_VISIBLE_DEVICES`
- 该变量的值是 GPU UUID 列表，不是数字索引
- `ixsmi --query-gpu=index,uuid --format=csv` 可用于 UUID 到 index 映射

## 项目边界

ix-container-toolkit 负责的是宿主机驱动层，不负责把完整 AI 软件栈打进容器。

适合由本项目注入：

- `/dev/iluvatarN`
- `libcuda.so`、`libixthunk.so`、`libixml.so`
- CUDA 兼容运行时动态库
- `ixsmi` 等基础 GPU 管理工具

不由本项目负责：

- PyTorch、vLLM、DeepSpeed 等 Python 框架
- 编译器、头文件、示例和文档
- 用户业务镜像的应用层依赖

## 当前状态

截至 2026-04-02，下面这些结论已经在仓库文档和真实节点验证中确认：

- 核心链路已实现
- 已支持 `ILUVATAR_COREX_VISIBLE_DEVICES`
- 已支持 UUID 到设备节点映射
- `ixsmi` 不可用时有降级路径
- hook 已支持写入 `ld.so.conf.d` 并运行 `ldconfig`
- 已补齐 RuntimeClass 资源

## 快速定位

- [CLAUDE.md](/home/huangsy/project/ix-container-toolkit/CLAUDE.md)：项目背景、当前架构、边界和风险
- [docs/project-status.md](/home/huangsy/project/ix-container-toolkit/docs/project-status.md)：项目状态与已完成功能
- [docs/pod-analysis.md](/home/huangsy/project/ix-container-toolkit/docs/pod-analysis.md)：真实 Pod 对比与 Device Plugin 行为分析
- [docs/physical-node-validation.md](/home/huangsy/project/ix-container-toolkit/docs/physical-node-validation.md)：物理节点验证记录
