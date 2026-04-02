# ix-toolkit — 项目概述与开发记录

## 开发目的

**问题背景**：天数（Iluvatar）GPU 没有像 NVIDIA 那样成熟的容器化支持工具链。在 Kubernetes 集群中，即使通过官方 Device Plugin 成功调度了天数 GPU，容器内也无法自动访问 `/dev/iluvatar*` 设备节点和驱动库（`/usr/local/corex`），导致 GPU 应用无法运行。

**目标**：参照 NVIDIA Container Toolkit 的设计，为天数 GPU 实现一个**自动将驱动和设备注入容器**的工具链，使申请了天数 GPU 的 Pod 无需任何镜像内修改，即可直接使用 GPU。

---

## 已完成的开发工作

### 项目信息
- **模块名**：`github.com/ix-toolkit/ix-toolkit`
- **语言**：Go 1.22+
- **目标平台**：Linux/amd64（在 macOS 上开发，交叉编译到 Linux）
- **编译命令**：`GOOS=linux GOARCH=amd64 go build ./...`（已验证通过）

---

## 项目文件结构

```
ix-toolkit/
├── cmd/
│   ├── ix-container-hook/main.go      # 二进制入口：OCI prestart hook
│   ├── ix-container-runtime/main.go   # 二进制入口：OCI runtime shim
│   └── ix-installer/main.go           # 二进制入口：节点安装器（DaemonSet init容器）
├── internal/
│   ├── hook/
│   │   ├── hook.go                    # 核心 hook 逻辑
│   │   ├── mount_linux.go             # Linux bind mount 实现（build tag: linux）
│   │   └── mount_other.go             # 非 Linux stub（build tag: !linux）
│   └── runtime/
│       └── runtime.go                 # OCI runtime shim 逻辑
├── pkg/
│   ├── config/config.go               # 配置结构体 + 默认值 + JSON 加载
│   ├── device/device.go               # /dev/iluvatar* 设备枚举与过滤
│   └── logger/logger.go               # logrus 封装
├── deployments/
│   ├── daemonset/daemonset.yaml        # Kubernetes DaemonSet 清单
│   └── rbac/rbac.yaml                  # ServiceAccount + ClusterRole + Binding
├── Dockerfile                          # 多阶段构建：builder + installer 镜像
├── Makefile                            # 构建、测试、镜像、部署命令
└── go.mod / go.sum
```

---

## 核心组件说明

### 1. `ix-container-hook`（最核心）
**OCI prestart hook**，由 runc 在容器进程启动前调用（stdin 接收 OCI 容器状态 JSON）。

执行流程：
1. 从 stdin 读取 OCI 容器状态，获取 bundle 路径
2. 解析 `bundle/config.json`，查找 `ILUVATAR_VISIBLE_DEVICES` 环境变量
3. 如果未设置该变量，跳过（非 GPU 容器不受影响）
4. 扫描 `/dev/iluvatar*`，按 index 过滤出请求的设备
5. Bind-mount 设备节点到容器 rootfs
6. Bind-mount 驱动库目录（`/usr/local/corex/lib64` 等）到容器内
7. Bind-mount 驱动工具二进制（`/usr/local/corex/bin`）到容器内
8. 写入 `/etc/ld.so.conf.d/ix-toolkit.conf`，让动态链接器找到驱动库

**关键配置项**（`/etc/ix-toolkit/config.json`）：
```json
{
  "hook": {
    "driverLibraryPaths": ["/usr/local/corex/lib64", "/usr/local/corex/lib"],
    "driverBinaryPaths": ["/usr/local/corex/bin"],
    "containerDriverRoot": "/usr/local/corex",
    "deviceListEnvvar": "ILUVATAR_VISIBLE_DEVICES"
  }
}
```

### 2. `ix-container-runtime`
**OCI runtime shim**，配置为 containerd 的 runtime，替代直接调用 runc。

- 拦截 `runc create` 命令
- 检查容器 spec 中是否有 `ILUVATAR_VISIBLE_DEVICES`
- 如果有，将 `ix-container-hook` 注入到 `config.json` 的 `hooks.prestart` 数组头部
- 然后将所有命令透明转发给真正的 runc

**containerd 配置（自动写入）**：
```toml
[plugins."io.containerd.grpc.v1.cri".containerd.runtimes.ix]
  runtime_type = "io.containerd.runc.v2"
  [plugins."io.containerd.grpc.v1.cri".containerd.runtimes.ix.options]
    BinaryName = "/usr/local/bin/ix-container-runtime"
```

### 3. `ix-installer`
**节点安装器**，作为 DaemonSet 的 init container 运行。

- 将二进制从镜像复制到宿主机（通过 hostPath volume）
- 写入 `/etc/ix-toolkit/config.json` 到宿主机
- Patch containerd 的 `config.toml`（幂等操作，不重复写入）
- 可选：通过 `systemctl restart containerd` 重启 containerd

---

## 工作流程（完整链路）

```
用户 Pod spec:
  resources:
    limits:
      iluvatar.ai/gpu: "1"
  env:
    - name: ILUVATAR_VISIBLE_DEVICES
      value: "0"
          ↓
天数官方 Device Plugin（已部署）分配 /dev/iluvatar0
          ↓
containerd 调用 ix-container-runtime create --bundle <path>
          ↓
ix-container-runtime 发现 ILUVATAR_VISIBLE_DEVICES，向 config.json 注入 prestart hook
          ↓
runc 执行 prestart hook → 调用 ix-container-hook（stdin = OCI state JSON）
          ↓
ix-container-hook bind-mount：
  /dev/iluvatar0            → <rootfs>/dev/iluvatar0
  /usr/local/corex/lib64    → <rootfs>/usr/local/corex/lib64
  /usr/local/corex/bin      → <rootfs>/usr/local/corex/bin
  写入 /etc/ld.so.conf.d/ix-toolkit.conf
          ↓
容器进程启动，GPU 和驱动已就绪
```

---

## 已知的待确认事项（需在物理节点验证）

1. **设备节点格式**：已假设为 `/dev/iluvatar0`、`/dev/iluvatar1`…，需确认实际命名规律（是否包含控制节点如 `/dev/iluvatarctl`）
2. **驱动库路径**：默认配置为 `/usr/local/corex/lib64` 和 `/usr/local/corex/lib`，需确认天数驱动的实际安装路径
3. **containerd 配置路径**：默认为 `/etc/containerd/config.toml`，部分发行版可能不同
4. **RuntimeClass**：Pod 是否需要指定 `runtimeClassName: ix`，或者是否通过其他方式触发

## 下一步开发建议

1. 在节点上确认驱动路径和设备节点格式，更新 `pkg/config/config.go` 的 `Defaults()`
2. 编写 Kubernetes `RuntimeClass` 资源清单（`deployments/runtimeclass.yaml`）
3. 实现单元测试（`pkg/device`、`internal/hook` 的逻辑均可 mock 测试）
4. 实现 CDI（Container Device Interface）spec 生成器（`pkg/cdi`），作为 hook 方案的替代/补充
5. 构建 CI/CD 流水线，自动交叉编译并推送镜像

---

## 快速开始（在新机器上继续开发）

```bash
# 克隆/复制项目后
cd ix-toolkit
go mod download

# 本地编译验证（macOS）
go build ./...

# 交叉编译到 Linux
GOOS=linux GOARCH=amd64 make build

# 构建 Docker 镜像
make docker-build IMAGE_REGISTRY=your-registry/ix-toolkit

# 部署到集群
make deploy
```
