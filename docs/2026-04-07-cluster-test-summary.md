# 2026-04-07 集群测试总结

> 目的：记录今天围绕统一 `xpu-runtime`、多架构镜像构建和 Ascend 910B 集群验证所完成的工作，作为明天继续推进测试和收口工作的直接上下文。

## 一、今天完成的工作

### 1. 统一运行时入口收口为 `xpu-runtime`

今天已完成代码侧收口：

- 集群统一只保留一个 `RuntimeClass`：`xpu-runtime`
- profile 不再单独配置 `runtimeClassName` 和 `handlerName`
- 节点侧实际注入行为由本机 active profile 决定
- `RuntimeClass` 不再承载 profile-specific scheduling

这使项目的职责边界更清晰：

- Pod 资源请求负责调度到正确节点
- `runtimeClassName: xpu-runtime` 负责启用 toolkit
- 节点 active profile 负责设备、驱动、工具和 linker 注入

### 2. 修复了异构 BuildKit 中的 `buildkitd-arm`

今天排查并修复了 `buildkitd-arm` 启动失败问题。

根因是：

- StatefulSet 启动参数里声明了 `--addr tcp://0.0.0.0:1234`
- `buildkitd.toml` 里又重复声明了同一个 grpc 地址
- `buildkitd` 启动时重复绑定端口，报 `bind: address already in use`

修复后，`buildkitd-arm` 已恢复可用，并能正常提供 `linux/arm64` worker。

### 3. 基于 BuildKit 构建并推送了 `xpu-toolkit:v1`

今天完成了镜像构建与推送：

- 仓库：`crater-harbor.act.buaa.edu.cn/xpu-huangsy`
- 镜像：`xpu-toolkit`
- 标签：`v1`

最终结果：

- `v1-amd64` 已推送
- `v1-arm64` 已推送
- `v1` 已作为多架构 manifest 成功推送
- Harbor 上可见统一的 `v1` artifact

### 4. 完成了 Ascend 910B 的集群侧 `xpu-runtime` 验证

本轮验证节点为 `kunlun-02`，使用 profile：

- `profiles/ascend-910b.yaml`

本轮已确认：

- `xpu-runtime` 能在集群中创建并被 Pod 正常引用
- installer 能在目标节点正确写入：
  - `/etc/accelerator-toolkit/config.json`
  - `/etc/accelerator-toolkit/profiles/active.yaml`
  - `/usr/local/bin/accelerator-container-runtime`
  - `/usr/local/bin/accelerator-container-hook`
  - `/etc/containerd/config.toml` 中的 `runtimes.xpu-runtime`
- 在宿主机重新加载 `containerd` 后，测试 Pod 能以 `runtimeClassName: xpu-runtime` 成功启动

测试 Pod：

- `crater-workspace/xpu-runtime-910b-test`

容器内已确认：

- `ASCEND_VISIBLE_DEVICES` 已注入
- `/etc/ld.so.conf.d/accelerator-toolkit.conf` 已写入
- 关键 Ascend 路径已存在
- `mount` 输出中能看到：
  - `/dev/davinci1`
  - `/dev/davinci_manager`

这说明当前主链已经成立：

- Pod 进入 `xpu-runtime`
- 节点读取本地 active profile
- runtime / hook 完成 profile 驱动的 env、linker、设备相关注入

## 二、今天补充得到的实际结论

### 1. 当前默认不应在 installer 中自动重启 `containerd`

这轮联调中，installer 在容器内直接执行宿主机 `systemctl restart containerd` 不稳定。

因此今天已经把渲染出的 DaemonSet 默认值调整为：

- `RESTART_CONTAINERD=false`

当前更稳妥的运维策略是：

- installer 负责落盘二进制、profile 和 `containerd` 配置
- 节点是否重启 `containerd`，由运维在确认宿主机环境后单独执行

### 2. `xpu-runtime` 模型已经可以支撑后续多 profile 扩展

在默认假设“用户会正确申请资源、Pod 会被正确调度、节点 profile 与物理设备一致”的前提下，当前模型已经自洽：

- 统一 `RuntimeClass` 简化了使用入口
- profile 继续保留节点事实表达能力
- 后续新增厂商样本时，不需要继续扩充新的 `RuntimeClass` 名称

## 三、明天继续要做的事

今天先收口到这里，剩余工作明天继续推进。明天建议按下面顺序继续：

1. 继续补强 910B 验证，增加更干净的设备节点与最小 workload 检查
2. 开始按同一套 `xpu-runtime` 流程验证 `310P`
3. 把今天的集群验证步骤整理成可复现的操作文档
4. 视验证结果决定是否继续收口 legacy 路径

## 四、当前阶段判断

到今天为止，项目已经从“profile 驱动框架基本成形”推进到“统一运行时入口已落地，并在 910B 节点上完成首轮集群验证”。

这意味着接下来的重点不再是重新设计入口，而是：

- 补强验证
- 扩大样本覆盖
- 逐步清理剩余兼容层
