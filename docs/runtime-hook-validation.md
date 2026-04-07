# RuntimeClass 与 Hook 联调验证报告

> 说明：本文保留了早期 `ix` 命名联调记录。当前实现中统一 `RuntimeClass` / handler 已收敛为 `xpu-runtime`，阅读本文时应将历史 `ix` 口径理解为当前统一运行时入口的前身。
> 验证日期：2026-04-02
> 验证范围：统一 `RuntimeClass xpu-runtime` 的前身链路验证、`containerd` runtime 注册、`accelerator-container-runtime` hook 注入、`accelerator-container-hook` 设备与驱动注入
> 验证节点：当前物理宿主机（同时为目标 Kubernetes 节点）

---

## 1. 测试目标

本次测试的目标不是只确认 Pod 能启动，而是完整验证以下链路：

1. Kubernetes `RuntimeClass xpu-runtime` 能被 kubelet 接受
2. `containerd` 已正确注册 `xpu-runtime` runtime handler
3. `accelerator-container-runtime` 能在 `create` 阶段把 `accelerator-container-hook` 注入 OCI spec
4. `accelerator-container-hook` 能识别 Device Plugin 注入的天数 GPU 信息
5. `accelerator-container-hook` 能把设备节点、驱动库、驱动工具和 `ld.so` 配置正确写入容器

---

## 2. 测试步骤

### 2.1 前置检查

先确认宿主机已经具备以下条件：

- 存在宿主机二进制：
  - `/usr/local/bin/accelerator-container-runtime`
  - `/usr/local/bin/accelerator-container-hook`
- `containerd` 配置文件 `/etc/containerd/config.toml` 中已经注册 `xpu-runtime` runtime
- 集群中已存在 `RuntimeClass xpu-runtime`

---

### 2.2 创建验证 Pod

为了避免测试 YAML 和真实业务差异过大，验证 Pod 直接仿照现有业务 Pod：

- 原 Pod：`crater-workspace/sg-huangsy-260402-440ca-default0-0`
- 验证文件：[sg-huangsy-260402-440ca-default0-0-ix.yaml](/home/huangsy/project/ix-container-toolkit/tmp/sg-huangsy-260402-440ca-default0-0-ix.yaml)

处理原则：

- 保留原 Pod 的镜像、资源、节点、调度器、挂载、容忍度等配置
- 新增 `runtimeClassName: xpu-runtime`
- 删除 `uid`、`resourceVersion`、`status` 等服务器生成字段
- 仅修改 Pod 名称

---

### 2.3 验证 `RuntimeClass -> containerd`

首次创建验证 Pod 时，Pod 未能启动，报错：

```text
Failed to create pod sandbox: rpc error: code = Unknown desc = failed to get sandbox runtime: no runtime for "xpu-runtime" is configured
```

这一步证明：

- `RuntimeClass xpu-runtime` 已被 kubelet 接受
- kubelet 已尝试按 `runtimeClassName: xpu-runtime` 创建 sandbox
- 但运行中的 `containerd` 还没有加载 `xpu-runtime` runtime 配置

处理方式：

- 检查 `/etc/containerd/config.toml`
- 手动重启 `containerd`

重启后，验证 Pod 进入 `Running`，说明：

- `RuntimeClass xpu-runtime` 生效
- `containerd` 已加载 `xpu-runtime` runtime

---

### 2.4 初次检查容器内结果

Pod 首次 `Running` 后，进入容器检查：

```bash
env | grep ILUVATAR
ls -l /dev/iluvatar*
ls -ld /usr/local/corex /usr/local/corex/lib /usr/local/corex/lib64 /usr/local/corex/bin
ls -l /usr/local/corex/bin/ixsmi
cat /etc/ld.so.conf.d/accelerator-toolkit.conf
```

观察到：

- `ILUVATAR_COREX_VISIBLE_DEVICES` 存在
- `/dev/iluvatar*` 存在
- 但 `/usr/local/corex` 相关驱动目录和 `ixsmi` 没有按预期出现

这说明设备节点不能作为 hook 生效的充分证据，因为它们可能只是 Device Plugin 注入的结果。

---

### 2.5 开启 debug 日志并检查宿主机证据

为了确认问题是在 runtime 还是在 hook，本次排查把宿主机配置切到 debug：

- 修改 [config.json](/etc/accelerator-toolkit/config.json)
  - `logLevel: debug`
  - `logFile: /var/log/ix-toolkit.log`

然后重点检查：

- `/var/log/ix-toolkit.log`
- 容器 OCI spec 中是否存在 `hooks.prestart`
- `crictl inspect` / `crictl inspectp` 的 runtime 信息

---

### 2.6 修复后重建验证 Pod

每次修复后都按以下方式复测：

```bash
kubectl -n crater-workspace delete pod sg-huangsy-260402-440ca-default0-0-ix --grace-period=0 --force
kubectl apply -f /home/huangsy/project/ix-container-toolkit/tmp/sg-huangsy-260402-440ca-default0-0-ix.yaml
kubectl -n crater-workspace get pod sg-huangsy-260402-440ca-default0-0-ix -o wide
kubectl -n crater-workspace exec sg-huangsy-260402-440ca-default0-0-ix -- bash
```

说明：

- `hook` 只会在容器创建时执行一次
- 所以每次改动 `accelerator-container-hook` 或 `accelerator-container-runtime` 后，都必须重建 Pod 才能看到结果

---

## 3. 测试中发现的问题与修复

### 3.1 `RuntimeClass` YAML 使用了非法字段

问题：

- [runtimeclass.yaml](/home/huangsy/project/ix-container-toolkit/deployments/runtimeclass/runtimeclass.yaml) 里包含 `scheduling.nodeClassification`
- API Server 返回 strict decoding error

修复：

- 改为合法字段：
  - `scheduling.nodeSelector`
  - `scheduling.tolerations`

结果：

- `RuntimeClass xpu-runtime` 可以正常 `kubectl apply`

---

### 3.2 `containerd` 已 patch，但未重启

问题：

- `/etc/containerd/config.toml` 中已存在 `ix` runtime 配置
- 但运行中的 `containerd` 未加载新配置

现象：

- 验证 Pod 报 `no runtime for "ix" is configured`

修复：

- 手动重启 `containerd`

结果：

- 验证 Pod 可以按 `runtimeClassName: xpu-runtime` 成功启动

---

### 3.3 `accelerator-container-runtime` 参数解析有 bug，导致 hook 未注入

问题：

- [runtime.go](/home/huangsy/project/ix-container-toolkit/internal/runtime/runtime.go) 的参数解析逻辑会把 `--root`、`--log` 这类全局参数后的值误判成子命令
- 结果没能正确识别 `create --bundle ...`
- `accelerator-container-hook` 没被注入到 OCI spec 的 `hooks.prestart`

修复：

- 修正 `parseArgs()`
- 正确跳过以下前置参数的取值：
  - `--root`
  - `-root`
  - `--log`
  - `--log-format`
- 增加对 `-b` 和 `-b=...` 的支持
- 增加 debug 日志，记录原始 `argv`、解析出的 `cmd` 和 `bundle`
- 补充单元测试

涉及文件：

- [runtime.go](/home/huangsy/project/ix-container-toolkit/internal/runtime/runtime.go)
- [runtime_test.go](/home/huangsy/project/ix-container-toolkit/internal/runtime/runtime_test.go)

结果：

- 宿主机日志出现：
  - `intercepting container create`
  - `injected accelerator-container-hook as prestart hook`

这证明 runtime 已经能正确拦截 `create` 并注入 hook。

---

### 3.4 驱动路径 symlink 导致重复处理

问题：

- 宿主机上 `/usr/local/corex/lib -> lib64`
- 解析真实路径后，`lib` 和 `lib64` 指向同一个目录
- 导致驱动库处理逻辑重复执行

修复：

- 对解析后的 `DriverLibraryPaths` 和 `DriverBinaryPaths` 做去重
- 保留原顺序
- 补充对应测试

涉及文件：

- [hook.go](/home/huangsy/project/ix-container-toolkit/internal/hook/hook.go)
- [hook_test.go](/home/huangsy/project/ix-container-toolkit/internal/hook/hook_test.go)

结果：

- 宿主机日志中的 `shared libraries injected` 不再重复出现

---

### 3.5 目录出现了，但普通文件是 `0` 字节占位文件

问题：

- 修复前，容器里已经能看到：
  - `/usr/local/corex/lib64/libcuda.so.1`
  - `/usr/local/corex/bin/ixsmi`
- 但它们是 `0` 字节空文件

这说明：

- rootfs 路径创建成功了
- 但普通文件 bind mount 在这个环境里没有真正进入容器 namespace

修复：

- 不再依赖普通文件 bind mount
- 改为：
  - 普通文件复制到容器 rootfs
  - symlink 按 symlink 重新创建
  - 子目录递归复制
- 同时显式补齐 `/usr/local/corex/lib -> lib64`

涉及文件：

- [hook.go](/home/huangsy/project/ix-container-toolkit/internal/hook/hook.go)
- [mount_linux.go](/home/huangsy/project/ix-container-toolkit/internal/hook/mount_linux.go)
- [mount_other.go](/home/huangsy/project/ix-container-toolkit/internal/hook/mount_other.go)

结果：

- 容器内的驱动库和 `ixsmi` 变成了真实文件，不再是空占位

---

## 4. 最终验证结果

在完成上述修复后，重新构建、安装宿主机二进制并重建验证 Pod，容器内最终检查结果如下：

### 4.1 路径结构正确

```text
/usr/local/corex
/usr/local/corex/lib -> lib64
/usr/local/corex/lib64
/usr/local/corex/bin
```

### 4.2 驱动库为真实文件

实际检查到：

- `/usr/local/corex/lib64/libcuda.so.1` 大小 `12912552`
- `/usr/local/corex/lib64/libixml.so` 大小 `478208`
- `/usr/local/corex/lib64/libixthunk.so` 大小 `1384648`

并且：

- `/usr/local/corex/lib64/libcuda.so` 为指向 `libcuda.so.1` 的 symlink

### 4.3 驱动工具为真实文件

实际检查到：

- `/usr/local/corex/bin/ixsmi` 大小 `914048`
- `/usr/local/corex/bin` 下其他工具与脚本也已存在

### 4.4 `ld.so` 配置正确

容器内 [accelerator-toolkit.conf](/etc/ld.so.conf.d/accelerator-toolkit.conf) 内容为：

```text
/usr/local/corex/lib64
```

### 4.5 宿主机日志显示完整链路已打通

宿主机 [ix-toolkit.log](/var/log/ix-toolkit.log) 中已确认出现：

- `intercepting container create`
- `injected accelerator-container-hook as prestart hook`
- `hook invoked`
- `injecting Iluvatar GPU into container`
- `resolved UUID-to-index mapping`
- `device injected`
- `shared libraries injected (so-only mode)`
- `driver binary dir injected`

结论：

- `RuntimeClass xpu-runtime` 生效
- `containerd` 已使用 `accelerator-container-runtime`
- `accelerator-container-runtime` 已正确向 OCI spec 注入 `accelerator-container-hook`
- `accelerator-container-hook` 已成功识别并处理天数 GPU
- 容器内设备、驱动库、驱动工具和动态链接器配置均已正确注入

---

## 5. 本次验证过程中更新的代码

本次联调直接修复了以下实现：

- [runtime.go](/home/huangsy/project/ix-container-toolkit/internal/runtime/runtime.go)
- [runtime_test.go](/home/huangsy/project/ix-container-toolkit/internal/runtime/runtime_test.go)
- [hook.go](/home/huangsy/project/ix-container-toolkit/internal/hook/hook.go)
- [hook_test.go](/home/huangsy/project/ix-container-toolkit/internal/hook/hook_test.go)
- [mount_linux.go](/home/huangsy/project/ix-container-toolkit/internal/hook/mount_linux.go)
- [mount_other.go](/home/huangsy/project/ix-container-toolkit/internal/hook/mount_other.go)
- [runtimeclass.yaml](/home/huangsy/project/ix-container-toolkit/deployments/runtimeclass/runtimeclass.yaml)

---

## 6. 接下来要做的事

当前宿主机已经通过手工覆盖二进制完成验证，但要把本次修复正式带入集群，还需要继续做以下工作：

1. 重新构建包含最新代码的部署镜像
2. 更新 DaemonSet，确保所有 GPU 节点安装到同一版本的 `accelerator-container-runtime` 和 `accelerator-container-hook`
3. 在至少一个额外 GPU 节点重复做一次验证，排除节点差异
4. 补一个真正调用 GPU 驱动或 `ixsmi` 的功能测试 Pod，而不只是验证文件存在
5. 视运维需要决定是否把宿主机 [config.json](/etc/accelerator-toolkit/config.json) 的日志级别从 `debug` 调回 `info`

---

## 7. 总结

本次联调最终确认，项目原始设计方向是可行的，真实问题集中在实现细节：

- `RuntimeClass` YAML 字段错误
- `containerd` 配置变更后未重启
- `accelerator-container-runtime` 的参数解析不适配真实 `containerd` 调用方式
- `hook` 对 symlink 路径做了重复处理
- `hook` 使用普通文件 bind mount 的策略不适合当前运行环境

这些问题修复后，`containerd + RuntimeClass + hook + 驱动注入` 这整条链路已经在真实节点上验证通过。
