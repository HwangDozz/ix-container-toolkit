# ix-container-toolkit 项目介绍（组会版）

> 生成日期：2026-04-08
> 适用场景：组会介绍、阶段汇报、项目入门说明
> 口径说明：仓库目录仍为 `ix-container-toolkit`，当前代码与部署入口已经统一收口到 `accelerator-*` 命名；本文统一称“本项目”。

## 一页结论

本项目要解决的问题很明确：在 Kubernetes 中，Device Plugin 往往只能完成资源分配，不能把宿主机上的驱动库、工具目录、linker 配置和补充环境一并准备进容器，导致“卡分到了，但容器里还是不能直接用加速卡”。

本项目给出的方案是：把节点本地的厂商差异收敛到一份 YAML profile，再用统一的 `accelerator-installer`、`accelerator-container-runtime`、`accelerator-container-hook` 三段式链路，在容器启动前自动完成设备和驱动注入。当前集群入口已经统一为 `RuntimeClass xpu-runtime`，而节点上真正注入什么内容，则由本机的 active profile 决定。

## 1. 项目目标

可以从四个层面理解项目目标：

1. 解决纯净业务镜像无法直接使用宿主机加速卡驱动的问题。
2. 把原先强绑定单一厂商的实现，演进为 profile 驱动的通用注入框架。
3. 统一集群使用入口，避免不同厂商继续扩张多套 `RuntimeClass` 和 runtime handler。
4. 让新厂商接入主要通过“新增 profile”完成，而不是继续修改核心代码。

换句话说，这个项目的核心不是再做一个新的 Device Plugin，而是在 Device Plugin 之后，补齐“容器运行时注入”这一层。

## 2. 整体设计

### 2.1 设计原则

- 统一入口：集群侧统一只保留一个 `RuntimeClass/handler = xpu-runtime`
- 事实外置：设备节点、选择器环境变量、驱动目录、工具目录、linker 策略等都写进 `profiles/*.yaml`
- 职责拆分：`runtime` 负责把 hook 注入 OCI spec，`hook` 负责把设备和驱动真正注入容器 rootfs

### 2.2 总体架构图

```text
profiles/*.yaml
  -> 描述厂商/节点运行时事实
  -> accelerator-profile-render
  -> 渲染 RuntimeClass / DaemonSet / Bundle
  -> make deploy
  -> accelerator-installer
  -> 复制二进制 + 写 active profile + patch containerd
  -> containerd（handler = xpu-runtime）
  -> accelerator-container-runtime
  -> 在 create 阶段注入 prestart hook
  -> accelerator-container-hook
  -> 按 active profile 注入设备 / 驱动库 / 工具 / linker
  -> 业务容器启动后直接可见加速卡环境
```

### 2.3 关键模块分工

| 模块 | 作用 | 当前定位 |
|---|---|---|
| `profiles/*.yaml` | 描述节点本地运行时事实 | 整个系统的事实源 |
| `accelerator-profile-render` | 从 profile 渲染 `RuntimeClass`、`DaemonSet` 和整包清单 | 部署入口 |
| `accelerator-installer` | 把二进制和 active profile 安装到宿主机，并 patch `containerd` | 节点初始化 |
| `accelerator-container-runtime` | 作为 runtime shim，在 `create` 阶段把 hook 注入 OCI spec | 注入入口 |
| `accelerator-container-hook` | 在容器启动前把设备、驱动库、工具目录、linker 配置写入 rootfs | 实际执行者 |

这里最值得强调的一点是：项目已经从“静态 YAML + 单厂商硬编码”转成了“统一运行时入口 + 节点 active profile”的模型。也就是说，集群看到的是统一的 `xpu-runtime`，节点差异则在 profile 中表达。

## 3. 工作流程

整个流程可以拆成“部署阶段”和“运行阶段”两部分。

### 3.1 部署阶段

1. 为某类节点准备对应 profile，例如 `profiles/iluvatar-bi-v150.yaml` 或 `profiles/ascend-910b.yaml`
2. 使用 `accelerator-profile-render` 或 `make deploy` 渲染并部署 `RuntimeClass xpu-runtime` 和 installer DaemonSet
3. `accelerator-installer` 在目标节点上执行，完成三件事：
   - 复制 `runtime` 和 `hook` 二进制到宿主机
   - 写入 `/etc/accelerator-toolkit/profiles/active.yaml`
   - patch `/etc/containerd/config.toml`，注册 `runtimes.xpu-runtime`
4. 节点在必要时重新加载 `containerd`，使新的 runtime handler 生效

### 3.2 容器启动阶段

```text
业务 Pod 申请厂商资源
  -> Device Plugin 分配设备
  -> Device Plugin 向容器注入 selector env
  -> Pod 指定 runtimeClassName: xpu-runtime
  -> kubelet / containerd 调用 accelerator-container-runtime create --bundle <path>
  -> runtime 读取 OCI spec，检查 selectorEnvVars
  -> runtime 把 accelerator-container-hook 写入 hooks.prestart
  -> runc 在 prestart 阶段执行 hook
  -> hook 读取 active profile + OCI state
  -> hook 注入设备节点、驱动库、工具目录、linker、extra env
  -> 容器主进程启动
  -> 容器内可直接访问加速卡运行环境
```

### 3.3 运行阶段的关键判断点

- Pod 侧是否显式指定 `runtimeClassName: xpu-runtime`
- Device Plugin 是否已经把 selector env 注入到 OCI spec
- `containerd` 是否已经加载 `runtimes.xpu-runtime`
- `runtime` 是否在 `create` 阶段把 hook 写进 `hooks.prestart`
- `hook` 是否根据 active profile 成功完成设备、artifact 和 linker 注入

这条链路的本质是：

- `RuntimeClass` 负责把请求导向统一入口
- active profile 负责描述节点差异
- runtime/hook 负责把 profile 转成容器内实际可用的运行环境

## 4. 项目效果

### 4.1 架构效果

- 已经实现统一运行时入口：集群只保留一个 `xpu-runtime`
- 已经实现 profile 驱动：同一套二进制可以在不同节点按 active profile 工作
- 已经形成清晰分层：profile 表达事实，installer 负责落盘，runtime 注入 hook，hook 完成 rootfs 注入

这意味着后续新增厂商时，主要工作会转移到 profile、factsheet 和验证，而不是继续往核心代码堆硬编码。

### 4.2 已验证效果

截至 2026-04-07，文档里已经确认了几类关键结果：

1. Iluvatar 真实节点端到端验证已经通过  
   已验证 `runtime shim` 注入 hook、设备节点注入、驱动库目录注入、工具目录注入、`ld.so.conf.d` 写入，以及 `none` 场景静默跳过。

2. Ascend 910B 集群侧验证已经通过  
   在 `kunlun-02` 节点上，`profiles/ascend-910b.yaml` 已完成首轮集群验证；测试 Pod 以 `runtimeClassName: xpu-runtime` 成功启动，容器内已确认存在 `ASCEND_VISIBLE_DEVICES`、关键驱动路径、控制设备节点以及 `accelerator-toolkit.conf`。

3. 统一入口已经落地  
   项目不再为不同 profile 维护多套 `RuntimeClass` 名称，而是统一由 `xpu-runtime` 进入，再由节点上的 active profile 决定真实注入行为。

4. 多架构分发链已经具备  
   已完成 `xpu-toolkit:v1` 多架构镜像构建与推送，可同时覆盖 `amd64` 和 `arm64` 节点，单个 DaemonSet 镜像入口即可适配混合集群。

### 4.3 当前阶段判断

当前项目已经从“单厂商专用注入工具”推进到“profile 驱动的通用加速卡容器注入框架”阶段，主链已经成立，且 910B 已经完成第二厂商样本闭环验证。

如果在组会上用一句话总结当前效果，可以这样概括：

> 本项目已经把“申请资源之后，如何让容器真正拿到设备、驱动、工具和 linker 环境”这条链路做成了统一的、可扩展的 profile 驱动框架，并在 Iluvatar 与 Ascend 910B 两类样本上得到了实际验证。

## 5. 汇报时可直接使用的结束语

本阶段最重要的成果，不是单点支持了某一张卡，而是把项目收口成了一个清晰的通用框架：集群入口统一为 `xpu-runtime`，节点差异下沉到 active profile，真正的设备和驱动注入由 runtime/hook 在容器启动前自动完成。这样后续继续扩 310P 或更多厂商时，工程复杂度会明显低于继续维护多套专用实现。

## 6. 参考文档

- [仓库入口说明](../README.md)
- [项目状态文档](./project-status.md)
- [全链路机制说明](./end-to-end-runtime-hook-chain.md)
- [首版 Generic Profile Schema](./generic-profile-schema.md)
- [2026-04-03 工作总结](./2026-04-03-work-summary.md)
- [2026-04-07 集群测试总结](./2026-04-07-cluster-test-summary.md)
- [RuntimeClass 与 Hook 联调验证报告](./runtime-hook-validation.md)
- [Ascend 910B Validation Record](./ascend-910b-validation.md)
