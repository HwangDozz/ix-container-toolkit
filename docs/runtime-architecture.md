# 运行时架构

> 更新日期：2026-04-23

## 执行链路

```text
Pod runtimeClassName: xpu-runtime
        ↓
kubelet / CRI
        ↓
containerd runtime handler: xpu-runtime
        ↓
accelerator-container-runtime create
        ↓
修改 OCI bundle config.json
        ↓
runc prestart
        ↓
accelerator-container-hook
        ↓
容器进程启动
```

## installer

`accelerator-installer` 以 DaemonSet init container 运行，主要副作用：

- 复制 `accelerator-container-runtime` 到宿主机 `/usr/local/bin/`。
- 复制 `accelerator-container-hook` 到宿主机 `/usr/local/bin/`。
- 写入 `/etc/accelerator-toolkit/config.json`。
- 写入 `/etc/accelerator-toolkit/profiles/active.yaml`。
- patch containerd runtime handler。
- 根据 profile 给节点打标签。

默认 `RESTART_CONTAINERD=false`。如宿主机确实需要重载 containerd，应由运维侧单独执行。

## runtime

`accelerator-container-runtime` 是底层 OCI runtime 的 shim。它只在 `create` 阶段修改 OCI spec：

- 判断容器是否存在 profile selector env，例如 `ASCEND_VISIBLE_DEVICES`。
- 注入 `accelerator-container-hook` 为 prestart hook。
- 注入 profile extra env，但不覆盖容器已有 env。
- 根据 selector env 和 profile device globs 注入 `linux.devices`。
- 注入对应 device cgroup allow 规则。

非 `create` 子命令直接透传到底层 runtime。

## hook

`accelerator-container-hook` 在 prestart 阶段执行：

- 读取 OCI state，定位容器 rootfs。
- 按 active profile 解析可见设备。
- 注入 profile artifacts：
  - `device-nodes`
  - `shared-libraries`
  - `directory`
- 写入 profile linker 配置。
- 在可用时运行 `ldconfig`。

## profile 关系

active profile 是三段链路共享的事实来源：

```text
/etc/accelerator-toolkit/profiles/active.yaml
```

集群入口固定为 `xpu-runtime`，实际注入行为由节点上的 active profile 决定。
