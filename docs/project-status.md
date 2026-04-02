# ix-container-toolkit 项目状态文档

> 更新日期：2026-04-02

---

## 一、项目背景

天数（Iluvatar）GPU 缺乏类似 NVIDIA Container Toolkit 的容器化支持。在 Kubernetes 中，即使通过官方 Device Plugin 调度了天数 GPU，容器内也无法自动访问 `/dev/iluvatar*` 设备节点和驱动库（`/usr/local/corex`），导致 GPU 应用无法运行。

本项目目标：为天数 GPU 实现一套**驱动与设备自动注入**工具链，使申请了 GPU 的 Pod 无需修改镜像，即可直接使用 GPU。

---

## 二、真实节点环境（已确认）

在本机节点上已验证以下实际配置，可作为后续开发的基准：

| 项目 | 实际值 |
|---|---|
| 设备节点 | `/dev/iluvatar0` ~ `/dev/iluvatar7`，共 8 张，字符设备，major=0x1fc（508）|
| 控制节点 | 无 `/dev/iluvatarctl`，全部为数字后缀 |
| 驱动库路径 | `/usr/local/corex/lib64/`（libcuda.so、libixml.so、libixthunk.so 等）|
| lib 软链接 | `/usr/local/corex/lib` → `lib64`（symlink）|
| 驱动工具路径 | `/usr/local/corex/bin/`（ixsmi、iluvatarcorex.sh、iluvatar-bug-report.sh 等）|
| OCI runtime | runc 1.3.3，路径 `/usr/sbin/runc` |
| containerd 配置 | `/etc/containerd/config.toml`，当前仅有 `runc` runtime，无 `ix` runtime |

**结论**：代码中的所有默认路径假设与实际节点完全吻合，无需修改。

---

## 三、已完成的功能

### 3.1 `ix-container-hook`（核心注入器）

**职责**：OCI prestart hook，由 runc 在容器进程启动前调用，负责将 GPU 设备和驱动注入容器 rootfs。

**已实现逻辑**：
1. 从 stdin 读取 OCI 容器状态 JSON，获取 bundle 路径
2. 解析 `bundle/config.json`，查找 `ILUVATAR_VISIBLE_DEVICES` 环境变量
3. 若未设置该变量，直接跳过（非 GPU 容器不受影响）
4. 若值为 `none`，静默跳过（已修复 bug，见第五节）
5. 调用 `device.Discover` 枚举 `/dev/iluvatar*`，按索引过滤出请求的设备
6. Bind-mount 设备节点到容器 rootfs（`/dev/iluvatarN` → `<rootfs>/dev/iluvatarN`）
7. Bind-mount 驱动库目录（`/usr/local/corex/lib64`、`/usr/local/corex/lib` → `<rootfs>/usr/local/corex/...`）
8. Bind-mount 驱动工具目录（`/usr/local/corex/bin` → `<rootfs>/usr/local/corex/bin`）
9. 写入 `<rootfs>/etc/ld.so.conf.d/ix-toolkit.conf`，使动态链接器找到驱动库

**实现文件**：
- `internal/hook/hook.go` — 核心逻辑
- `internal/hook/mount_linux.go` — Linux bind mount 实现（`syscall.MS_BIND|MS_REC`）
- `internal/hook/mount_other.go` — 非 Linux stub（用于 macOS 开发环境编译）

---

### 3.2 `ix-container-runtime`（runtime shim）

**职责**：作为 containerd 的 OCI runtime，透明包装 runc，按需注入 prestart hook。

**已实现逻辑**：
1. 拦截 `runc create` 命令，解析 `--bundle` 参数（支持 `--bundle=/path`、`--bundle /path`、`-bundle=`、`-bundle ` 四种格式）
2. 读取 `bundle/config.json`，检测容器 env 中是否存在 `ILUVATAR_VISIBLE_DEVICES=`
3. 若存在，将 `ix-container-hook` 前插到 `hooks.prestart` 数组头部，写回 `config.json`
4. 将所有参数原样转发给底层 runc（exec 方式，保持退出码一致）
5. 非 `create` 命令透明直通，不做任何修改

**实现文件**：`internal/runtime/runtime.go`

---

### 3.3 `ix-installer`（节点安装器）

**职责**：以 DaemonSet init container 运行，在每个 GPU 节点上完成一次性安装。

**已实现逻辑**：
1. 将 `ix-container-runtime` 和 `ix-container-hook` 二进制从镜像复制到宿主机（默认 `/usr/local/bin/`）
2. 生成并写入 `/etc/ix-toolkit/config.json`，支持通过环境变量覆盖驱动路径和日志级别
3. Patch `/etc/containerd/config.toml`，追加 `ix` runtime class 配置（幂等：已存在则跳过）
4. 可选：通过 `systemctl restart containerd` 重启 containerd（`RESTART_CONTAINERD=true` 时执行）

**支持的环境变量**：

| 变量 | 默认值 | 说明 |
|---|---|---|
| `HOST_BIN_DIR` | `/usr/local/bin` | 宿主机二进制安装路径 |
| `HOST_CONFIG_DIR` | `/etc/ix-toolkit` | 宿主机配置目录 |
| `HOST_MOUNT` | `/host` | 宿主机根目录挂载点 |
| `RESTART_CONTAINERD` | `""` | 设为 `true` 时重启 containerd |
| `IX_DRIVER_LIB_PATHS` | `/usr/local/corex/lib64:/usr/local/corex/lib` | 驱动库路径（冒号分隔）|
| `IX_DRIVER_BIN_PATHS` | `/usr/local/corex/bin` | 驱动工具路径 |
| `IX_LOG_LEVEL` | `info` | 日志级别 |

**实现文件**：`cmd/ix-installer/main.go`

---

### 3.4 基础设施

| 组件 | 状态 | 说明 |
|---|---|---|
| `pkg/config` | 完成 | 配置结构体、默认值、JSON 加载、`applyDefaults` 补全空字段 |
| `pkg/device` | 完成 | `/dev/iluvatar*` 枚举（跳过非数字后缀）、all/none/索引过滤 |
| `pkg/logger` | 完成 | logrus 封装，支持文件和 stderr 双输出 |
| `Dockerfile` | 完成 | 多阶段构建：Go builder + 最小 installer 镜像 |
| `Makefile` | 完成 | build / test / docker-build / docker-push / deploy / undeploy |
| DaemonSet YAML | 完成 | kube-system 命名空间，nodeSelector `iluvatar.ai/gpu=present`，init container 模式 |
| RBAC YAML | 完成 | ServiceAccount + ClusterRole + ClusterRoleBinding |
| 单元测试 | 完成 | 35 个用例，覆盖 config / device / hook / runtime 全部核心逻辑 |

---

## 四、完整工作流程

```
用户 Pod spec:
  resources.limits.iluvatar.ai/gpu: "1"
  env:
    ILUVATAR_VISIBLE_DEVICES: "0"
          │
          ▼
天数官方 Device Plugin
  分配 /dev/iluvatar0，将其写入容器 cgroup
          │
          ▼
containerd 调用 ix-container-runtime create --bundle <path>
          │
          ▼  (内部)
ix-container-runtime:
  1. 读取 bundle/config.json
  2. 发现 ILUVATAR_VISIBLE_DEVICES → 注入 prestart hook
  3. 将 ix-container-hook 写入 hooks.prestart[0]
  4. exec runc create --bundle <path> ...
          │
          ▼
runc 在容器进程启动前执行 prestart hook:
  调用 ix-container-hook（stdin = OCI state JSON）
          │
          ▼
ix-container-hook:
  bind-mount /dev/iluvatar0          → <rootfs>/dev/iluvatar0
  bind-mount /usr/local/corex/lib64 → <rootfs>/usr/local/corex/lib64
  bind-mount /usr/local/corex/lib   → <rootfs>/usr/local/corex/lib
  bind-mount /usr/local/corex/bin   → <rootfs>/usr/local/corex/bin
  写入 <rootfs>/etc/ld.so.conf.d/ix-toolkit.conf
          │
          ▼
容器进程启动
  GPU 设备 /dev/iluvatar0 可访问
  libcuda.so 等驱动库可加载
  ixsmi 等工具可执行
```

---

## 五、发现并修复的 Bug

### `ILUVATAR_VISIBLE_DEVICES=none` 导致 hook 报错

**现象**：设置 `ILUVATAR_VISIBLE_DEVICES=none` 时，hook 报错 `no Iluvatar devices found`，而非静默跳过。

**根因**：

```
visibleDevices = "none"  // 非空，跳过第一个短路
device.Discover("none")  // 正确返回空列表
len(devs) == 0           // 进入错误分支，报错退出
```

`hook.go` 中第一个短路仅处理 `visibleDevices == ""`（变量未设置），未处理显式设为 `none` 的情况。

**修复位置**：`internal/hook/hook.go`，在 `device.Discover` 调用前增加：

```go
if strings.EqualFold(strings.TrimSpace(visibleDevices), "none") {
    h.log.Debug("ILUVATAR_VISIBLE_DEVICES=none, skipping GPU injection")
    return nil
}
```

---

## 六、端到端验证结果

在本机（含 8 张天数 GPU 的真实节点）完成了完整链路的手动验证：

### 验证步骤

1. 编译二进制（`go build`，Linux/amd64）
2. 构造 OCI bundle（`config.json` 含 `ILUVATAR_VISIBLE_DEVICES=0,1`，rootfs 为空目录）
3. 调用 `ix-container-runtime create --bundle <bundle>`，验证 hook 注入
4. 以 OCI state JSON 为 stdin 调用 `ix-container-hook`，验证注入结果
5. 用 `findmnt` 验证 bind mount 真实生效

### 验证结果

| 验证项 | 结果 |
|---|---|
| runtime shim 注入 hook | PASS — `hooks.prestart[0]` 正确写入 config.json |
| `/dev/iluvatar0,1` bind-mount | PASS — `findmnt` 确认挂载，设备号 major=0x1fc 与宿主机一致 |
| `lib64/` 驱动库挂载 | PASS — libcuda.so 等文件通过挂载在 rootfs 内可见 |
| `bin/` 工具目录挂载 | PASS — ixsmi 等工具在 rootfs 内可见 |
| `ld.so.conf.d/ix-toolkit.conf` | PASS — 文件正确写入，包含两条库路径 |
| `none` 静默跳过 | PASS（修复后）— 退出码 0，rootfs/dev 为空 |
| 非 GPU 容器不受影响 | PASS — 无 env 变量时 hook 直接返回 0 |

---

## 七、待完成的工作

### 高优先级（影响集群可用性）

#### 1. Kubernetes RuntimeClass 清单（缺失）

缺少 `deployments/runtimeclass.yaml`，Pod 无法通过 `runtimeClassName: ix` 触发 runtime shim。

需要创建：

```yaml
apiVersion: node.k8s.io/v1
kind: RuntimeClass
metadata:
  name: ix
handler: ix
```

同时需确认 containerd config 中 handler 名称与 RuntimeClass `handler` 字段匹配。

#### 2. Device Plugin 联动验证

当前验证仅覆盖 hook 链路，未验证天数官方 Device Plugin 是否会自动设置 `ILUVATAR_VISIBLE_DEVICES`。需确认：
- Device Plugin 是否通过 env 注入该变量（还是需要用户手动设置）
- 若需手动设置，是否需要 Webhook 自动注入

#### 3. `ix-installer` 集成测试

installer 的二进制复制、containerd patch、systemctl 重启逻辑缺乏自动化测试，目前只能在真实节点手动验证。

---

### 中优先级

#### 4. CDI（Container Device Interface）支持

CDI 是比 hook 更标准的设备注入方式，containerd 1.7+ 原生支持。当前 containerd config 中已有 `cdi_spec_dirs` 配置项但 `enable_cdi = false`。

需要：
- 实现 `pkg/cdi` 包，生成符合 CDI spec 的 JSON 文件（写入 `/etc/cdi/`）
- 在 installer 中写入 CDI spec 并启用 `enable_cdi = true`
- 作为 hook 方案的替代或并行方案

#### 5. DaemonSet 主容器优化

当前主容器使用 `gcr.io/distroless/static:nonroot` + `sleep 86400` 保活，镜像拉取依赖外网。可替换为 `pause` 镜像或直接使用 `busybox:1.35`。

#### 6. 节点标签自动化

DaemonSet 的 `nodeSelector: iluvatar.ai/gpu: present` 需要节点预先打标签，目前未提供自动打标签的机制。

---

### 低优先级

#### 7. CI/CD 流水线

目前需手动执行 `make docker-build && make docker-push`，建议添加 GitHub Actions 或 Tekton Pipeline 实现：
- 自动交叉编译（GOOS=linux GOARCH=amd64）
- 自动构建并推送镜像
- 自动运行单元测试

#### 8. ix-installer 单元测试

`cmd/ix-installer/main.go` 的 `copyBinaries`、`writeConfig`、`patchContainerd` 函数目前无测试覆盖，可通过传入临时目录路径实现 mock 文件系统测试。

#### 9. 卸载流程

当前 `make undeploy` 只删除 DaemonSet 和 RBAC，未清理：
- 宿主机上的二进制（`/usr/local/bin/ix-container-*`）
- 宿主机上的配置（`/etc/ix-toolkit/`）
- containerd config 中注入的 `ix` runtime 段落

---

## 八、单元测试覆盖概览

| 包 | 测试数 | 覆盖的核心场景 |
|---|---|---|
| `pkg/config` | 7 | Defaults 默认值、文件不存在、合法/非法 JSON、部分字段默认补全、已有值不覆盖 |
| `pkg/device` | 10 | filterByIndex（单个/多个/不存在/非法/空格/重复）、Discover none/大小写/非法索引 |
| `internal/hook` | 12 | visibleDevices（有值/未设/nil/自定义/空值）、injectLdSoConf（创建/幂等/空路径）、Run（none 跳过/无 GPU 跳过/非法 JSON/bundle 不存在）|
| `internal/runtime` | 17 | parseArgs（4 种 flag 格式/start/delete/空/全局 flag）、containerRequestsGPU（有/无/nil/空值/自定义 envvar）、injectHook（注入/跳过/前插/不存在 bundle）|
| **合计** | **46** | |

运行命令：

```bash
go test ./pkg/config/... ./pkg/device/... ./internal/hook/... ./internal/runtime/... -v
```
