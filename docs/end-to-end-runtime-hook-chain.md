# ix-container-toolkit 全链路机制说明

> 更新日期：2026-04-02
> 目标：说明 `ix-container-toolkit` 从部署到生效的完整链路，包括各组件如何注册、如何交互、每一步凭什么判断已经生效

> 说明：本文中的静态 `deployments/runtimeclass/runtimeclass.yaml` 与 `deployments/daemonset/daemonset.yaml` 现在仅作为 Iluvatar 历史参考。当前主入口是 `profiles/*.yaml` + `accelerator-profile-render` + `make deploy`。

---

## 1. 先给结论

这个系统的核心思路不是直接改 Kubernetes，也不是直接改业务镜像，而是在节点侧插入一层自定义 OCI runtime shim：

1. `accelerator-installer` 把 `accelerator-container-runtime` 和 `accelerator-container-hook` 安装到宿主机
2. `accelerator-installer` patch 宿主机 `containerd` 配置，向 `containerd` 注册一个新的 runtime handler：`ix`
3. 用户 Pod 使用 `runtimeClassName: ix`
4. kubelet 通过 CRI 告诉 `containerd`：这个 Pod 要用 `ix` 这个 runtime handler
5. `containerd` 创建容器时，实际调用的是 `accelerator-container-runtime`
6. `accelerator-container-runtime` 在 `create` 阶段改写 OCI bundle 的 `config.json`，把 `accelerator-container-hook` 注入为 `prestart hook`
7. 底层 `runc` 执行容器创建，在容器进程启动前自动执行 `accelerator-container-hook`
8. `accelerator-container-hook` 读取容器 spec，识别 `ILUVATAR_COREX_VISIBLE_DEVICES`，把设备节点、驱动库、驱动工具和 `ld.so` 配置写进容器 rootfs
9. 容器主进程启动，此时容器内部已经能看到 `/dev/iluvatar*`、`/usr/local/corex/lib64`、`/usr/local/corex/bin/ixsmi`

本项目真正的关键点有两个：

- runtime 的职责是“把 hook 塞进 OCI spec”
- hook 的职责是“在 rootfs 里把 GPU 相关内容准备好”

---

## 2. 组件和职责

### 2.1 `accelerator-installer`

入口文件：
[main.go](/home/huangsy/project/ix-container-toolkit/cmd/accelerator-installer/main.go)

职责：

- 把二进制复制到宿主机
- 写宿主机配置文件 `/etc/accelerator-toolkit/config.json`
- patch `/etc/containerd/config.toml`
- 可选重启 `containerd`
- 给节点打 `iluvatar.ai/gpu=present` 标签

这一步决定了“节点是否具备使用 ix runtime 的条件”。

---

### 2.2 `accelerator-container-runtime`

入口文件：
[main.go](/home/huangsy/project/ix-container-toolkit/cmd/accelerator-container-runtime/main.go)

核心实现：
[runtime.go](/home/huangsy/project/ix-container-toolkit/internal/runtime/runtime.go)

职责：

- 作为 `containerd` 注册的 `BinaryName`
- 透明代理底层 `runc`
- 只在 `create` 阶段拦截 OCI bundle
- 如果容器请求了 GPU，就把 `accelerator-container-hook` 写进 `config.json` 的 `hooks.prestart`

这一步决定了“hook 有没有机会运行”。

---

### 2.3 `accelerator-container-hook`

入口文件：
[main.go](/home/huangsy/project/ix-container-toolkit/cmd/accelerator-container-hook/main.go)

核心实现：
[hook.go](/home/huangsy/project/ix-container-toolkit/internal/hook/hook.go)

职责：

- 读取 OCI hook stdin 里的 container state
- 找到 bundle 和 rootfs
- 读取 `config.json`
- 判断容器是否请求 GPU
- 找到宿主机上实际对应的 `/dev/iluvatar*`
- 把驱动文件和工具准备到容器 rootfs

这一步决定了“容器里最终能不能真正用 GPU”。

---

## 3. 系统是如何注册到 Kubernetes 和 containerd 中的

### 3.1 DaemonSet 如何把 installer 跑到节点上

当前推荐入口：

- `go run ./cmd/accelerator-profile-render daemonset --profile profiles/iluvatar-bi-v150.yaml --image <image>`
- `make render-daemonset PROFILE=profiles/iluvatar-bi-v150.yaml IMAGE=<image>`
- `make deploy PROFILE=profiles/iluvatar-bi-v150.yaml IMAGE=<image>`

历史参考文件：
[daemonset.yaml](/home/huangsy/project/ix-container-toolkit/deployments/daemonset/daemonset.yaml)

这个 DaemonSet 的关键点：

- 跑在 `kube-system`
- `nodeSelector: iluvatar.ai/gpu=present`
- `initContainer` 使用特权模式
- 宿主机根目录通过 `hostPath` 挂进容器的 `/host`

关键配置：

```yaml
initContainers:
  - name: accelerator-installer
    securityContext:
      privileged: true
    env:
      - name: HOST_MOUNT
        value: /host
      - name: HOST_BIN_DIR
        value: /usr/local/bin
      - name: HOST_CONFIG_DIR
        value: /etc/accelerator-toolkit
      - name: RESTART_CONTAINERD
        value: "true"
    volumeMounts:
      - name: host-root
        mountPath: /host
```

这表示：

- installer 容器里执行的“写文件”其实是写宿主机
- `/host/usr/local/bin/...` 实际对应宿主机 `/usr/local/bin/...`
- `/host/etc/containerd/config.toml` 实际对应宿主机 `/etc/containerd/config.toml`

所以 installer 并不是“给容器安装二进制”，而是在“借助特权容器改宿主机”。

---

### 3.2 installer 如何向宿主机注册 runtime

核心代码：
[main.go](/home/huangsy/project/ix-container-toolkit/cmd/accelerator-installer/main.go)

`copyBinaries()` 做的事情：

- 把镜像里的
  - `/usr/local/bin/accelerator-container-runtime`
  - `/usr/local/bin/accelerator-container-hook`
- 复制到宿主机：
  - `/usr/local/bin/accelerator-container-runtime`
  - `/usr/local/bin/accelerator-container-hook`

`writeConfig()` 做的事情：

- 生成宿主机 [config.json](/etc/accelerator-toolkit/config.json)

这里会写入关键配置：

- `underlyingRuntime: "runc"`
- `hookPath: "/usr/local/bin/accelerator-container-hook"`
- `deviceListEnvvar: "ILUVATAR_COREX_VISIBLE_DEVICES"`
- `driverLibraryPaths`
- `driverBinaryPaths`

`patchContainerd()` 做的事情：

- 打开宿主机 [config.toml](/etc/containerd/config.toml)
- 追加：

```toml
[plugins."io.containerd.grpc.v1.cri".containerd.runtimes.ix]
  runtime_type = "io.containerd.runc.v2"
  [plugins."io.containerd.grpc.v1.cri".containerd.runtimes.ix.options]
    BinaryName = "/usr/local/bin/accelerator-container-runtime"
```

这段配置的意义是：

- 对 `containerd` 来说，新增了一个 runtime handler，名字叫 `ix`
- 这个 handler 仍然属于 `io.containerd.runc.v2`
- 但执行 runtime 时，不再直接调系统默认 `runc`
- 而是先调 `/usr/local/bin/accelerator-container-runtime`

也就是说：

- `handler = ix`
- `BinaryName = /usr/local/bin/accelerator-container-runtime`

这两个配置把 Kubernetes 的 `RuntimeClass` 和宿主机上的 runtime 二进制接起来了。

---

### 3.3 为什么还需要重启 containerd

因为 `patchContainerd()` 改的是配置文件，不是运行中的内存状态。

如果只写了 `/etc/containerd/config.toml`，但没重启 `containerd`，运行中的 `containerd` 仍然不知道有 `ix` 这个 handler。

这也是我们联调时出现这个报错的原因：

```text
no runtime for "ix" is configured
```

这个报错的准确含义不是“YAML 写错了”，而是：

- `RuntimeClass ix` 存在
- kubelet 也把这个 handler 名传给了 `containerd`
- 但当前运行中的 `containerd` 没有把 `ix` 注册进自己的 runtime handler 表里

凭据：

- 宿主机 [config.toml](/etc/containerd/config.toml) 已有 `runtimes.ix`
- 但 Pod 仍报 `no runtime for "ix" is configured`
- 重启 `containerd` 后，Pod 立即能进入 `Running`

所以“配置存在”和“配置已生效”是两回事。

---

## 4. Kubernetes 这一侧是如何触发 runtime 的

### 4.1 `RuntimeClass` 如何工作

当前推荐入口：

- `go run ./cmd/accelerator-profile-render runtimeclass --profile profiles/iluvatar-bi-v150.yaml`
- `make render-runtimeclass PROFILE=profiles/iluvatar-bi-v150.yaml`

历史参考文件：
[runtimeclass.yaml](/home/huangsy/project/ix-container-toolkit/deployments/runtimeclass/runtimeclass.yaml)

核心内容：

```yaml
apiVersion: node.k8s.io/v1
kind: RuntimeClass
metadata:
  name: ix
handler: ix
```

`metadata.name` 是用户 Pod 里要写的名字：

```yaml
spec:
  runtimeClassName: ix
```

`handler: ix` 是传给 kubelet / CRI / containerd 的 handler 名。

也就是说：

- Pod 写的是 `runtimeClassName`
- kubelet 真正下发给 `containerd` 的是 `handler`
- `handler` 再去匹配 `containerd` 配置里的 `runtimes.ix`

---

### 4.2 业务 Pod 如何走到 `accelerator-container-runtime`

当 Pod 指定：

```yaml
spec:
  runtimeClassName: ix
```

并且这个 `RuntimeClass` 的 `handler` 也是 `ix` 时，kubelet 在创建 sandbox / container 时会告诉 `containerd`：

- 这个工作负载要用 runtime handler `ix`

如果 `containerd` 已加载前面的配置，它就会选择：

- `runtime_type = io.containerd.runc.v2`
- `BinaryName = /usr/local/bin/accelerator-container-runtime`

因此这次创建容器时，不会直接跑系统 `runc`，而是先跑：

- `/usr/local/bin/accelerator-container-runtime`

这一步的凭据：

- `RuntimeClass ix` 存在
- Pod describe 中 `Runtime Class Name: ix`
- `crictl inspectp` 中能看到：
  - `runtimeHandler: "ix"`
  - `runtimeOptions.binary_name: "/usr/local/bin/accelerator-container-runtime"`

这三条一起才能证明：

- 不是 YAML 写了 `runtimeClassName` 就算成功
- 而是 kubelet 和 `containerd` 的实际调用路径已经切到了 `accelerator-container-runtime`

---

## 5. runtime 是如何识别容器并注入 hook 的

### 5.1 `accelerator-container-runtime` 的启动方式

入口：
[main.go](/home/huangsy/project/ix-container-toolkit/cmd/accelerator-container-runtime/main.go)

它启动时会：

1. 从 `ACCELERATOR_CONFIG_FILE` 或默认路径加载 [config.json](/etc/accelerator-toolkit/config.json)
2. 创建 logger
3. 调 `rt.Exec(os.Args)`

注意这里没有用普通 CLI flag 解析器，因为它必须保留 `containerd/runc` 调用时的原始参数。

---

### 5.2 它为什么只拦截 `create`

核心逻辑在 [runtime.go](/home/huangsy/project/ix-container-toolkit/internal/runtime/runtime.go)：

```go
cmd, bundlePath := parseArgs(args[1:])
if cmd != "create" || bundlePath == "" {
    return r.delegate(args)
}
```

原因很简单：

- `delete`、`kill`、`start`、`exec` 这些阶段都不适合修改 OCI spec
- 只有 `create` 时，OCI bundle 的 `config.json` 还在创建流程里，允许被改写

所以 runtime 的职责不是接管整个生命周期，而是只在最早期插入 hook。

---

### 5.3 runtime 如何判断“这个容器值得注入 hook”

`injectHook()` 会打开 bundle 下的 `config.json`，解析为 OCI spec，然后调用：

```go
r.containerRequestsGPU(&spec)
```

实现逻辑是：

- 读取 `spec.Process.Env`
- 查找 `cfg.Hook.DeviceListEnvvar + "="`

当前默认配置来自 [config.go](/home/huangsy/project/ix-container-toolkit/pkg/config/config.go)：

```go
DeviceListEnvvar: "ILUVATAR_COREX_VISIBLE_DEVICES"
```

也就是说 runtime 并不是靠：

- Kubernetes 资源声明
- 容器名
- namespace

来判断是否注入 hook，而是靠“OCI spec 里有没有这个环境变量”。

这与真实集群行为是匹配的，因为当前天数 Device Plugin 注入到容器里的就是：

- `ILUVATAR_COREX_VISIBLE_DEVICES=<GPU UUID,...>`

因此 runtime 的判断条件实际上是：

- 这个容器已经被 Device Plugin 选中过，并拿到了 GPU 可见性信息

---

### 5.4 runtime 是怎么把 hook 写进 OCI spec 的

如果容器请求 GPU，`injectHook()` 会在 `spec.Hooks.Prestart` 头部插入：

```go
specs.Hook{
    Path: r.cfg.HookPath,
}
```

当前默认 `HookPath` 是：

- `/usr/local/bin/accelerator-container-hook`

写回后，bundle 的 `config.json` 就变成了：

- 这个容器在 `runc create/start` 期间，必须执行一个 OCI `prestart hook`

注意这里 runtime 本身不会直接运行 hook。  
它做的事情只有一件：

- 修改 `config.json`

真正执行 hook 的是底层 `runc`。

---

### 5.5 这一步已经生效的凭据是什么

最强的凭据有两类。

第一类：宿主机 runtime debug 日志  
在 [ix-toolkit.log](/var/log/ix-toolkit.log) 中已经出现：

- `parsed runtime arguments`
- `intercepting container create`
- `injected accelerator-container-hook as prestart hook`

这说明：

- `containerd` 确实调用了 `accelerator-container-runtime`
- 它确实看到了 `create --bundle ...`
- 它确实把 hook 写进了 OCI spec

第二类：运行中的 OCI spec  
如果查看业务容器对应 bundle 的 `config.json`，可以看到 `hooks.prestart` 包含：

- `/usr/local/bin/accelerator-container-hook`

这说明 hook 不是理论上会运行，而是已经被明确写入容器创建流程。

---

## 6. hook 是如何被 `runc` 识别并执行的

### 6.1 为什么不需要额外注册 hook

hook 不需要向 `containerd` 单独注册。

原因是 OCI hook 的生效方式本来就不是“向 containerd 注册一个 hook 名称”，而是：

- 在 OCI bundle 的 `config.json` 里写 `hooks.prestart`

只要 `runc` 读取到的 `config.json` 含有：

- `hooks.prestart[].path = /usr/local/bin/accelerator-container-hook`

它就会在容器主进程启动前执行这个二进制，并把 OCI container state JSON 通过 stdin 传给它。

所以：

- runtime 负责写配置
- runc 负责执行 hook
- hook 本身不需要在 `containerd` 再登记一次

---

### 6.2 hook 启动后第一件事做什么

入口：
[main.go](/home/huangsy/project/ix-container-toolkit/cmd/accelerator-container-hook/main.go)

它启动后会：

1. 加载 [config.json](/etc/accelerator-toolkit/config.json)
2. 创建 logger
3. 执行 `h.Run(os.Stdin)`

而 `Run()` 的第一件事是：

- 从 stdin 读取 OCI container state

这个 state 里有至少两个关键字段：

- `ID`
- `Bundle`

随后 hook 会再去 `Bundle/config.json` 里加载完整 OCI spec。

也就是说，hook 拿到上下文的方式是：

- stdin 拿 state
- bundle 拿 spec

---

## 7. hook 是如何识别“该给这个容器注入哪几张卡”的

### 7.1 识别来源不是 Pod YAML，而是 OCI spec 环境变量

hook 的 `visibleDevices()` 会扫描 `spec.Process.Env`，找：

- `ILUVATAR_COREX_VISIBLE_DEVICES=...`

这个值不是 hook 自己生成的，而是 Device Plugin 注入给容器的。

因此链路关系是：

1. Pod 请求了 `iluvatar.ai/gpu`
2. Device Plugin 为它分配 GPU
3. Device Plugin 把 `ILUVATAR_COREX_VISIBLE_DEVICES` 注入容器 spec
4. runtime 发现这个 env，决定注入 hook
5. hook 再读同一个 env，决定要暴露哪几张卡

这就是 runtime 和 hook 的配合点：

- runtime 用它判断“要不要插 hook”
- hook 用它判断“具体注入哪些设备”

---

### 7.2 为什么这里支持 UUID

真实环境里，这个 env 的值不是纯数字索引，而是 GPU UUID，例如：

```text
ILUVATAR_COREX_VISIBLE_DEVICES=GPU-5020332b-...,GPU-8241332d-...
```

设备识别逻辑在：
[device.go](/home/huangsy/project/ix-container-toolkit/pkg/device/device.go)

`device.Discover()` 的流程是：

1. 枚举宿主机 `/dev/iluvatar*`
2. 如果 env 是 `all` / `none` / 数字索引，按常规方式处理
3. 如果 env 看起来像 `GPU-...`，走 UUID 分支
4. 调用 `ixsmi --query-gpu=index,uuid --format=csv`
5. 把 UUID 映射回设备索引
6. 再用索引匹配宿主机真实设备节点

这就是为什么容器里可以请求：

- `GPU-8241332d-...`

而 hook 最终仍然能找到：

- `/dev/iluvatar7`

---

### 7.3 这一步生效的凭据是什么

宿主机 debug 日志已经记录到：

- `resolved UUID-to-index mapping`
- `device injected`

并且容器内可以看到对应设备节点：

- `/dev/iluvatar0`
- `/dev/iluvatar7`

这说明：

- hook 读到了 UUID
- UUID 成功解析成索引
- 对应设备节点成功进入了容器

---

## 8. hook 是如何把文件“放进容器里”的

### 8.1 先说 rootfs 是什么

hook 拿到 OCI spec 后，会取：

- `spec.Root.Path`

如果是相对路径，就用 `state.Bundle` 拼成绝对路径。

因此 `rootfs` 实际上是：

- 当前这个容器在宿主机上的根文件系统目录

后续所有注入动作，都是在修改这个 rootfs。

不是在修改镜像，也不是在容器里执行 shell。

---

### 8.2 设备节点是如何进入容器的

实现逻辑：

```go
target := filepath.Join(rootfs, dev.Path)
ensureFile(target)
bindMount(dev.Path, target)
```

其中：

- `dev.Path` 是宿主机 `/dev/iluvatarN`
- `target` 是 `<rootfs>/dev/iluvatarN`

所以设备节点注入方式是：

- 在容器 rootfs 中创建一个占位文件
- 把宿主机真实设备节点 bind mount 到这个路径

这部分仍然使用的是 bind mount，因为设备节点本来就应该以设备文件的形式传递。

---

### 8.3 驱动库最初为什么没有成功

最初的设计思路是：

- 对驱动目录或驱动文件做 bind mount

但联调时我们发现：

- 容器里路径出现了
- 文件名也出现了
- 但普通文件是 `0` 字节

这说明在当前环境里：

- rootfs 路径创建成功
- 但普通文件 bind mount 没真正进入最终容器 namespace

所以原来的“对普通库文件 bind mount”策略在这个环境里不可靠。

---

### 8.4 现在驱动库和工具是如何成功进入容器的

修复后，策略变成了三种分开处理。

普通文件：

- 直接复制到容器 rootfs

symlink：

- 在容器里按原关系重建 symlink

目录：

- 递归复制目录内容

相关实现：

- [hook.go](/home/huangsy/project/ix-container-toolkit/internal/hook/hook.go)
- [mount_linux.go](/home/huangsy/project/ix-container-toolkit/internal/hook/mount_linux.go)

例如：

- `/usr/local/corex/lib64/libcuda.so.1` 是普通文件，所以复制
- `/usr/local/corex/lib64/libcuda.so -> libcuda.so.1` 是 symlink，所以重建 symlink
- `/usr/local/corex/bin/logtools` 是目录，所以递归复制

这也是为什么现在容器里 finally 能看到：

- 真实大小的 `.so` 文件
- 真实大小的 `ixsmi`
- 正确的 `lib -> lib64`

---

### 8.5 为什么还要写 `ld.so.conf.d`

即使文件已经复制到容器里，动态链接器默认也不一定会去 `/usr/local/corex/lib64` 找库。

因此 hook 还会写：

- `/etc/ld.so.conf.d/accelerator-toolkit.conf`

内容是：

```text
/usr/local/corex/lib64
```

然后尝试在 rootfs 里执行 `ldconfig`，刷新 `ld.so.cache`。

这一步的目的是：

- 让容器内程序在运行 `ixsmi` 或 GPU 应用时，动态链接器能找到 `libcuda.so`、`libixml.so`、`libixthunk.so`

否则就会出现之前在物理机验证中看到的错误：

- `error while loading shared libraries: libixml.so: cannot open shared object file`

---

## 9. 这条链路为什么现在可以认定已经打通

要说“全链路打通”，至少要有下面几层证据同时成立。

### 9.1 部署层证据

- 宿主机有：
  - `/usr/local/bin/accelerator-container-runtime`
  - `/usr/local/bin/accelerator-container-hook`
- 宿主机有：
  - [config.json](/etc/accelerator-toolkit/config.json)
- 宿主机 `containerd` 配置有：
  - `runtimes.ix`
  - `BinaryName = "/usr/local/bin/accelerator-container-runtime"`

这说明节点侧安装已经完成。

---

### 9.2 Kubernetes / CRI 层证据

- 集群存在 `RuntimeClass ix`
- 验证 Pod 指定了 `runtimeClassName: ix`
- `crictl inspectp` 里能看到：
  - `runtimeHandler: "ix"`
  - `runtimeOptions.binary_name: "/usr/local/bin/accelerator-container-runtime"`

这说明 kubelet 和 `containerd` 的调用链已经切换到了 `accelerator-container-runtime`。

---

### 9.3 runtime 层证据

宿主机日志出现：

- `intercepting container create`
- `injected accelerator-container-hook as prestart hook`

这说明 runtime 已成功修改 OCI spec。

---

### 9.4 hook 层证据

宿主机日志出现：

- `hook invoked`
- `injecting Iluvatar GPU into container`
- `resolved UUID-to-index mapping`
- `device injected`
- `shared libraries injected (so-only mode)`
- `driver binary dir injected`

这说明 hook 不只是被注入了，而且实际运行了。

---

### 9.5 容器内结果证据

容器内已经确认存在：

- `/usr/local/corex/lib -> lib64`
- `/usr/local/corex/lib64/libcuda.so.1`
- `/usr/local/corex/lib64/libixml.so`
- `/usr/local/corex/lib64/libixthunk.so`
- `/usr/local/corex/bin/ixsmi`
- `/etc/ld.so.conf.d/accelerator-toolkit.conf`

并且这些文件不是空占位，而是有真实大小，例如：

- `libcuda.so.1` 大小 `12912552`
- `ixsmi` 大小 `914048`

这说明 rootfs 注入已经落到容器里了。

---

## 10. 本次联调中最关键的几个误区

### 10.1 “Pod Running” 不代表 hook 已生效

Pod 能跑起来，只能说明：

- `containerd` 至少接受了这个 runtime handler

但不能说明：

- hook 已注入
- 驱动已注入
- GPU 可用

必须继续查宿主机日志和容器内文件。

---

### 10.2 “容器里有 `/dev/iluvatar*`” 也不代表 hook 已生效

因为这些设备节点也可能是 Device Plugin 自己加进去的。

真正能说明 hook 生效的证据是：

- OCI spec 有 `prestart hook`
- 宿主机日志有 `hook invoked`
- 容器里出现 `/usr/local/corex`、`ixsmi`、`ld.so.conf.d/accelerator-toolkit.conf`

---

### 10.3 “配置文件里有 `runtimes.ix`” 不等于 containerd 已生效

因为运行中的 `containerd` 可能还没 reload/restart。

我们已经实际遇到过：

- 配置文件里有 `ix`
- 但 Pod 仍报 `no runtime for "ix" is configured`

所以必须区分：

- 文件已写入
- 进程已加载

---

## 11. 当前系统边界

截至目前，可以确认：

- 节点安装链路是通的
- runtime 注册链路是通的
- hook 注入链路是通的
- UUID 到设备索引解析是通的
- 驱动文件和工具注入链路是通的

但还需要继续做的，是更偏“功能验证”的一层：

- 在容器里实际运行 `ixsmi`
- 运行依赖 `libcuda.so` 的程序
- 在多个节点复测

也就是说，当前已经可以说：

- “runtime/hook 全链路机制已经打通”

但更严格地说，还应继续验证：

- “业务 GPU 工作负载在所有目标节点上都能稳定工作”

---

## 12. 推荐的阅读顺序

如果你要继续维护这套链路，建议按这个顺序读代码：

1. [daemonset.yaml](/home/huangsy/project/ix-container-toolkit/deployments/daemonset/daemonset.yaml)
2. [runtimeclass.yaml](/home/huangsy/project/ix-container-toolkit/deployments/runtimeclass/runtimeclass.yaml)
3. [main.go](/home/huangsy/project/ix-container-toolkit/cmd/accelerator-installer/main.go)
4. [config.go](/home/huangsy/project/ix-container-toolkit/pkg/config/config.go)
5. [main.go](/home/huangsy/project/ix-container-toolkit/cmd/accelerator-container-runtime/main.go)
6. [runtime.go](/home/huangsy/project/ix-container-toolkit/internal/runtime/runtime.go)
7. [main.go](/home/huangsy/project/ix-container-toolkit/cmd/accelerator-container-hook/main.go)
8. [hook.go](/home/huangsy/project/ix-container-toolkit/internal/hook/hook.go)
9. [device.go](/home/huangsy/project/ix-container-toolkit/pkg/device/device.go)
10. [mount_linux.go](/home/huangsy/project/ix-container-toolkit/internal/hook/mount_linux.go)

如果你想看这次真实联调里遇到的问题和修复过程，再读：

- [runtime-hook-validation.md](/home/huangsy/project/ix-container-toolkit/docs/runtime-hook-validation.md)

---

## 13. 最后的总结

这套系统的生效机制可以压缩成一句话：

`RuntimeClass` 负责把容器创建请求导向 `accelerator-container-runtime`，`accelerator-container-runtime` 负责把 `accelerator-container-hook` 写进 OCI spec，`accelerator-container-hook` 负责把 GPU 设备和驱动文件准备进容器 rootfs，最终让业务容器在不改镜像的前提下获得天数 GPU 能力。

本次联调已经证明这条链路在真实节点上是可以跑通的，且每一层都已经拿到了对应的代码依据和运行时证据。
