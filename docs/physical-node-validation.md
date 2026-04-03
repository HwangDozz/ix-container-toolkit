# 物理节点验证报告

> 验证日期：2026-04-02
> 验证节点：搭载天数（Iluvatar）BI-V150 GPU 的 Linux 节点
> 目的：确认代码中对设备节点格式、驱动路径、ixsmi 命令行为的假设是否与实际一致，并修正所有偏差。

---

## 1. 设备节点验证

### 验证方法

```bash
ls -la /dev/iluvatar*
ls /dev/ | grep -i iluvatar
```

### 结果

```
crw-rw-rw- 1 root root 508, 0 Mar  5 06:19 /dev/iluvatar0
crw-rw-rw- 1 root root 508, 1 Mar  5 06:19 /dev/iluvatar1
crw-rw-rw- 1 root root 508, 2 Mar  5 06:19 /dev/iluvatar2
crw-rw-rw- 1 root root 508, 3 Mar  5 06:19 /dev/iluvatar3
crw-rw-rw- 1 root root 508, 4 Mar  5 06:19 /dev/iluvatar4
crw-rw-rw- 1 root root 508, 5 Mar  5 06:19 /dev/iluvatar5
crw-rw-rw- 1 root root 508, 6 Mar  5 06:19 /dev/iluvatar6
crw-rw-rw- 1 root root 508, 7 Mar  5 06:19 /dev/iluvatar7
```

### 结论

| 假设 | 实际 | 是否一致 |
|------|------|---------|
| 设备名格式为 `/dev/iluvatar<N>` | ✓ 完全一致 | ✓ |
| 设备为字符设备 | ✓ `c`（major 508） | ✓ |
| 存在控制节点（如 `/dev/iluvatarctl`） | ✗ 不存在 | ✓ 代码跳过非数字后缀的逻辑正确 |
| 索引从 0 开始连续 | ✓ 0~7 | ✓ |

**代码无需修改。** `pkg/device/device.go` 中跳过非数字后缀节点的逻辑是正确的防御策略，即使当前不存在此类节点。

---

## 2. 驱动安装路径验证

### 验证方法

```bash
ls /usr/local/ | grep -i corex
ls -la /usr/local/corex
readlink /usr/local/corex
ls /usr/local/corex/lib64/
ls /usr/local/corex/lib/
ls /usr/local/corex/bin/
```

### 结果

```
# /usr/local/ 下的 corex 相关目录
corex          → 符号链接，指向 corex-4.3.0/
corex-4.3.0    → 实际安装目录

# 符号链接详情
lrwxrwxrwx 1 root root 12 Sep 11 2025 /usr/local/corex -> corex-4.3.0/

# lib64 内容（共 4 个文件）
libcuda.so  libcuda.so.1  libixml.so  libixthunk.so

# lib 内容（共 4 个文件，与 lib64 相同）
libcuda.so  libcuda.so.1  libixml.so  libixthunk.so

# bin 内容
corex-driver-uninstaller  iluvatar-bug-report.sh  iluvatarcorex.sh
ixsmi  logtools  pci_timeout.sh
```

### 结论

| 假设 | 实际 | 是否一致 |
|------|------|---------|
| 驱动根目录为 `/usr/local/corex` | ✓ | ✓（是符号链接） |
| 实际路径为版本化目录 | ✓ `corex-4.3.0/` | ✓ `resolveDriverPaths()` 逻辑设计正确 |
| lib64 包含驱动库 | ✓ 4 个 .so 文件 | ✓ |
| lib64 下有 python3 等大型子目录 | ✗ 不存在 | `so-only` 模式仍正确，本节点无需过滤 |
| bin 包含 ixsmi 等工具 | ✓ | ✓ |

**代码无需修改。** 驱动库数量少（仅 4 个），`so-only` 过滤模式开销可忽略，逻辑正确。

---

## 3. ixsmi 命令验证

### 验证方法

```bash
# 测试是否在 PATH 中
which ixsmi

# 测试直接运行（不带 LD_LIBRARY_PATH）
/usr/local/corex/bin/ixsmi --help

# 测试带库路径运行
LD_LIBRARY_PATH=/usr/local/corex/lib64:/usr/local/corex/lib \
  /usr/local/corex/bin/ixsmi --help

# 验证 --query-gpu 参数支持
LD_LIBRARY_PATH=/usr/local/corex/lib64:/usr/local/corex/lib \
  /usr/local/corex/bin/ixsmi --query-gpu=index,uuid --format=csv
```

### 结果

```
# which ixsmi
ixsmi: command not found   ← 不在 PATH 中

# 不带库路径运行
/usr/local/corex/bin/ixsmi: error while loading shared libraries:
  libixml.so: cannot open shared object file: No such file or directory

# --query-gpu 实际输出
index, uuid
0, GPU-5020332b-19bd-52dd-9a9c-00496701884f
1, GPU-f7e62b92-ddc0-53dd-969f-e3abfbe116e2
2, GPU-c22ac027-569b-548c-93dd-5ec7ef8eca9a
3, GPU-3767df9b-5e64-5279-9def-4e8e28374bf9
4, GPU-d928a36e-49dd-5bb0-81f5-be21c925b406
5, GPU-4c7792e8-d1c5-5e7c-93f1-41a7ebd6ccbe
6, GPU-f8e20f7e-3ff3-5c25-874a-5380336d7e5a
7, GPU-8241332d-37cb-5585-8482-a32f429b4bdc
```

### 结论

| 假设 | 实际 | 是否一致 |
|------|------|---------|
| ixsmi 在系统 PATH 中 | ✗ 不在 PATH | **不一致，需修复** |
| ixsmi 可以直接运行 | ✗ 需要 LD_LIBRARY_PATH | **不一致，需修复** |
| `--query-gpu=index,uuid --format=csv` 语法支持 | ✓ 完全支持 | ✓ |
| 输出格式为 `index, uuid` 的 CSV | ✓ 完全一致 | ✓ |
| GPU 型号 | Iluvatar BI-V150 × 8 | 记录 |

---

## 4. 发现的问题及修复

### 问题一：ixsmi 不在 PATH 且需要 LD_LIBRARY_PATH

**根本原因：** 天数驱动安装后，`/usr/local/corex/bin` 未加入系统 `PATH`，且 ixsmi 依赖的 `libixml.so` 在 `/usr/local/corex/lib64` 中，也未加入 `ld.so.cache`，导致直接调用会同时报"命令未找到"和"共享库找不到"两类错误。

**修复位置：** `pkg/device/device.go` — `ixsmiQuery()` 函数

**修复方式：** 在代码中硬编码已知的绝对路径作为首选，PATH 查找作为兜底；并通过 `cmd.Env` 显式注入 `LD_LIBRARY_PATH`：

```go
// 修复前：依赖 PATH 查找，不带库路径
cmd := exec.Command("ixsmi", "--query-gpu=index,uuid", "--format=csv")

// 修复后：优先绝对路径，注入库路径
candidates := []string{
    "/usr/local/corex/bin/ixsmi",
    "/usr/local/corex-4.3.0/bin/ixsmi",
}
// ... 找到后：
cmd := exec.Command(ixsmiPath, "--query-gpu=index,uuid", "--format=csv")
cmd.Env = append(os.Environ(),
    "LD_LIBRARY_PATH=/usr/local/corex/lib64:/usr/local/corex/lib:...",
)
```

---

### 问题二（P0）：DaemonSet keep-alive 容器使用 distroless 镜像但执行 shell

**根本原因：** `gcr.io/distroless/static:nonroot` 镜像不包含 `/bin/sh`，`command: ["/bin/sh", "-c", "while true; do sleep 86400; done"]` 会导致容器以 `exec format error` 或 `no such file` 反复崩溃重启，造成 DaemonSet Pod 永远处于 `CrashLoopBackOff`。

**修复位置：** `deployments/daemonset/daemonset.yaml`

**修复方式：** 将主容器替换为 `pause` 镜像——这是 Kubernetes 原生的"什么都不做"容器，专为此类场景设计：

```yaml
# 修复前
- name: ix-toolkit-sleep
  image: gcr.io/distroless/static:nonroot
  command: ["/bin/sh", "-c", "while true; do sleep 86400; done"]

# 修复后
- name: ix-toolkit-pause
  image: registry.k8s.io/pause:3.9
```

---

### 问题三（P0）：缺少 RuntimeClass 资源

**根本原因：** Pod 需要通过 `runtimeClassName: ix` 指定使用 `accelerator-container-runtime`，但集群中没有对应的 `RuntimeClass` 资源，`kubectl apply` 后 Pod 会因找不到 runtime class 而无法调度。

**修复位置：** 新增 `deployments/runtimeclass/runtimeclass.yaml`，更新 `Makefile` 的 `deploy`/`undeploy` target。

**修复方式：**

```yaml
apiVersion: node.k8s.io/v1
kind: RuntimeClass
metadata:
  name: ix
handler: ix  # 对应 containerd config.toml 中注册的 runtime 名称
```

---

### 问题四（P1）：ixsmi 不可用时整体失败

**根本原因：** UUID 格式的 `ILUVATAR_COREX_VISIBLE_DEVICES`（由 Device Plugin 注入）需要 ixsmi 解析，若 ixsmi 调用失败则直接返回 error，导致整个容器创建失败，影响所有 GPU 容器。

**修复位置：** `pkg/device/device.go` — `filterByUUID()` 函数

**修复方式：** 新增 `filterByUUIDPositional()` 降级路径：当 ixsmi 不可用时，按 UUID 在列表中的出现顺序与设备索引做位置映射（第 N 个 UUID → 索引 N 的设备），并输出 Warn 日志标记降级行为：

```go
func filterByUUID(...) ([]Device, error) {
    uuidMap, err := IxsmiQueryFunc()
    if err != nil {
        log.WithError(err).Warn("ixsmi unavailable, falling back to positional UUID→index mapping")
        return filterByUUIDPositional(all, uuids, log)  // 降级，不 fatal
    }
    // ... 正常流程
}
```

---

### 问题五（P1）：写入 ld.so.conf.d 后未更新 ld.so.cache

**根本原因：** 许多容器镜像的应用程序依赖 `ld.so.cache`（由 `ldconfig` 生成）而非实时读取 `ld.so.conf.d`。hook 只写了配置文件，不运行 `ldconfig`，导致动态链接器在容器启动时找不到新挂载的驱动库。

**修复位置：**
- `internal/hook/mount_linux.go`：新增 `runLdconfig(rootfs string)` 函数
- `internal/hook/mount_other.go`：新增 no-op stub（非 Linux 平台）
- `internal/hook/hook.go`：在 `injectDriverLibraries()` 末尾调用 `runLdconfig()`

**修复方式：** 利用 `exec.Cmd.SysProcAttr.Chroot` 在 fork 出的子进程中切换根目录后运行 `ldconfig`，避免影响 hook 进程自身的工作目录：

```go
func runLdconfig(rootfs string) error {
    cmd := exec.Command(ldconfig)
    cmd.SysProcAttr = &syscall.SysProcAttr{Chroot: rootfs}
    // ldconfig 在容器 rootfs 内读取 /etc/ld.so.conf.d/accelerator-toolkit.conf
    // 并更新 /etc/ld.so.cache
    return cmd.CombinedOutput()
}
```

失败时仅输出 Warn 日志，不阻断容器启动（某些极简镜像可能没有 `/etc/ld.so.conf.d/`）。

---

### 问题六（P2）：节点标签需手动打，DaemonSet nodeSelector 无法自动匹配

**根本原因：** DaemonSet 使用 `nodeSelector: iluvatar.ai/gpu: "present"` 限制只在 GPU 节点运行，但没有任何机制自动给节点打这个标签，需要运维手动执行 `kubectl label node`，部署流程不完整。

**修复位置：** `cmd/accelerator-installer/main.go` 新增 `labelNode()` 步骤；`deployments/daemonset/daemonset.yaml` 通过 Downward API 注入 `NODE_NAME`；`deployments/rbac/rbac.yaml` 已有 `patch nodes` 权限（无需修改）。

**修复方式：** installer 在安装步骤中通过 in-cluster ServiceAccount token 直接调用 Kubernetes API（JSON Merge Patch）给当前节点打标签，失败时仅 Warn 不阻断安装：

```go
// PATCH /api/v1/nodes/<NODE_NAME>
// Content-Type: application/merge-patch+json
// {"metadata": {"labels": {"iluvatar.ai/gpu": "present"}}}
```

---

### 问题七（P2）：DaemonSet 镜像地址硬编码

**根本原因：** `daemonset.yaml` 中 `image: ix-toolkit/installer:latest` 是占位符，实际部署需要真实的镜像仓库地址，若直接 `kubectl apply` 会拉取失败。

**修复位置：** `Makefile` — `deploy` target

**修复方式：** `make deploy` 时用 `sed` 将占位符替换为 `IMAGE_REGISTRY/installer:IMAGE_TAG`，YAML 文件本身保留可读的占位符：

```bash
make deploy IMAGE_REGISTRY=myregistry.example.com/ix-toolkit IMAGE_TAG=v1.0
```

---

### 问题八：测试文件中环境变量名过时

**根本原因：** 代码已将环境变量从 `ILUVATAR_VISIBLE_DEVICES` 更名为 `ILUVATAR_COREX_VISIBLE_DEVICES`（与天数官方命名保持一致），但测试文件未同步更新，导致 `go test ./...` 失败。

**修复位置：** `pkg/config/config_test.go`、`internal/hook/hook_test.go`、`internal/runtime/runtime_test.go`

**修复方式：** 批量替换：
```bash
sed -i 's/ILUVATAR_VISIBLE_DEVICES/ILUVATAR_COREX_VISIBLE_DEVICES/g' \
  pkg/config/config_test.go \
  internal/hook/hook_test.go \
  internal/runtime/runtime_test.go
```

修复后 `go test ./...` 全部通过。

---

## 5. 验证后配置参考

以下为在本节点（Iluvatar BI-V150 × 8）上经过验证的最终配置：

### `/etc/accelerator-toolkit/config.json`

```json
{
  "underlyingRuntime": "runc",
  "hookPath": "/usr/local/bin/accelerator-container-hook",
  "hook": {
    "driverLibraryPaths": ["/usr/local/corex/lib64", "/usr/local/corex/lib"],
    "driverBinaryPaths": ["/usr/local/corex/bin"],
    "containerDriverRoot": "/usr/local/corex",
    "deviceListEnvvar": "ILUVATAR_COREX_VISIBLE_DEVICES",
    "libraryFilterMode": "so-only",
    "libraryExcludeDirs": ["python3", "cmake", "clang"]
  },
  "logLevel": "info"
}
```

### 节点信息

| 项目 | 值 |
|------|----|
| GPU 型号 | Iluvatar BI-V150 |
| GPU 数量 | 8 |
| 设备节点 | `/dev/iluvatar0` ~ `/dev/iluvatar7` |
| 驱动版本目录 | `/usr/local/corex-4.3.0/` |
| ixsmi 路径 | `/usr/local/corex/bin/ixsmi` |
| 所需 LD_LIBRARY_PATH | `/usr/local/corex/lib64:/usr/local/corex/lib` |
