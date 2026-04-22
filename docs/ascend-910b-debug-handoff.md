# Ascend 910B Debug Handoff

本文记录当前会话的工作状态，供切换到 `kunlun-02` 后继续调试。

## 当前目标

910B 训练场景闭环：

- toolkit 负责运行时注入设备、驱动库、工具链环境；
- PyTorch backend 镜像负责框架和厂商 Python 栈；
- 用户使用同一套训练代码和数据，在 910B 上无感运行。

## 已确认事实

- 910B 节点：`kunlun-02`
- 架构：`arm64`
- OS：`openEuler 22.03 (LTS-SP4)`
- Kernel：`5.10.0-303.0.0.206.oe2203sp4.aarch64`
- containerd：`containerd://1.7.28`
- 节点标签：
  - `accelerator=huawei-Ascend910`
  - `node.kubernetes.io/npu.chip.name=910B3`
  - `servertype=Ascend910B-20`
- 扩展资源：`huawei.com/Ascend910`
  - Capacity: `8`
  - Allocatable: `7`
- RuntimeClass：`xpu-runtime`
- active profile：`ascend-910b`
- toolkit DaemonSet Pod：`kube-system/accelerator-toolkit-9glff`，运行在 `kunlun-02`
- Ascend device plugin Pod：`kube-system/ascend-device-plugin-daemonset-tct2w`，运行在 `kunlun-02`

宿主机确认存在：

- `/dev/davinci0` 到 `/dev/davinci7`
- `/dev/davinci_manager`
- `/dev/devmm_svm`
- `/dev/hisi_hdc`

普通 Ascend workload 已验证可以看到设备：

- Pod：`crater-workspace/jpt-zhouxiao25-260421-56579-default0-0`
- 请求：`huawei.com/Ascend910: 2`
- annotation：`Ascend910-2,Ascend910-3`
- 容器内可见：
  - `/dev/davinci2`
  - `/dev/davinci3`
  - `/dev/davinci_manager`
  - `/dev/devmm_svm`
  - `/dev/hisi_hdc`
  - `ASCEND_VISIBLE_DEVICES=2,3`

## Backend 镜像状态

910B PyTorch backend 镜像已成功构建并推送：

```text
crater-harbor.act.buaa.edu.cn/xpu-huangsy/ascend-910b-pytorch-backend:dev
sha256:39031e3322204e6ec9de4474d902a38f296d1cf35ccd439c299eeb1f78ae0046
```

构建链路已验证：

- 本机已安装 `docker` CLI 和 `docker-buildx`
- buildx builder：`crater-buildkit`
- 远程 BuildKit：
  - `crater-images/buildkitd-arm-0`
  - `crater-images/buildkitd-arm-1`
  - `crater-images/buildkitd-x86-0/1/2`
- arm BuildKit 已修复，支持 `linux/arm64`

如需重新打开 buildx 后端端口转发：

```bash
kubectl port-forward -n crater-images svc/buildkitd-arm 1234:1234
kubectl port-forward -n crater-images svc/buildkitd-x86 1235:1234
```

## Smoke Job 失败现象

提交过：

```bash
kubectl apply -f experiments/ascend-910b/pytorch-backend/k8s/smoke-job.yaml
```

Pod：

```text
crater-workspace/ascend-910b-backend-smoke-s8hph
```

现象：

- Pod 成功调度到 `kunlun-02`
- `xpu-runtime` 生效
- backend 镜像成功拉取
- runtime 日志显示：
  - `injected extra OCI env from profile`
  - `injected accelerator-container-hook as prestart hook`
- smoke 脚本内看到：
  - `ASCEND_VISIBLE_DEVICES=4`
  - Ascend 环境变量存在
  - Ascend driver/toolkit 路径存在
- 但容器内没有设备：
  - `devices: []`
  - `/dev/davinci_manager: false`
  - `/dev/devmm_svm: false`
  - `/dev/hisi_hdc: false`
  - `npu_smi: null`
- 最终报错：

```text
RuntimeError: no /dev/davinci* devices found
```

该 Job 已清理：

```bash
kubectl delete job -n crater-workspace ascend-910b-backend-smoke --ignore-not-found=true
```

## 当前判断

backend 镜像不是当前阻塞点。

阻塞点在 toolkit 的设备注入链路：

- 使用 Device Plugin 的普通 Ascend Pod 有设备；
- 使用 `xpu-runtime` 的 smoke Pod 没有设备；
- toolkit runtime 原实现只注入 prestart hook 和环境变量；
- hook 在 prestart 阶段 bind mount `/dev/davinci*` 和控制设备；
- 但最终业务进程内看不到设备，疑似 prestart bind mount 时序或 OCI spec 中设备/cgroup 没有被 runc 原生处理。

## 本会话已做的代码修复

已修改：

- `internal/runtime/runtime.go`
- `internal/runtime/runtime_test.go`

核心改动：

- runtime 在 `create` 阶段仍然注入 prestart hook；
- 同时解析 profile selector env，例如 `ASCEND_VISIBLE_DEVICES=4`；
- 通过 profile 的 `deviceGlobs` 找到选中设备，例如 `/dev/davinci4`；
- 通过 profile 的 `controlDeviceGlobs` 找到控制设备；
- 写入 OCI spec：
  - `linux.devices`
  - `linux.resources.devices`
- hook 继续负责驱动库、工具目录、`ld.so.conf.d` 等 rootfs 注入。

本地验证：

```bash
GOCACHE=/tmp/ix-container-toolkit-gocache \
GOMODCACHE=/tmp/ix-container-toolkit-gomodcache \
go test ./internal/runtime ./pkg/device ./pkg/runtimeview ./pkg/profile ./pkg/config
```

结果：

```text
ok github.com/accelerator-toolkit/accelerator-toolkit/internal/runtime
ok github.com/accelerator-toolkit/accelerator-toolkit/pkg/device
ok github.com/accelerator-toolkit/accelerator-toolkit/pkg/runtimeview
ok github.com/accelerator-toolkit/accelerator-toolkit/pkg/profile
ok github.com/accelerator-toolkit/accelerator-toolkit/pkg/config
```

注意：该修复还没有构建 toolkit 镜像并部署到集群。

## 建议的 kunlun-02 Debug 顺序

### 1. 先确认当前运行时 bundle 中是否有设备

创建一个长时间 sleep 的 smoke Pod，使用：

- `runtimeClassName: xpu-runtime`
- `requests/limits: huawei.com/Ascend910: 1`
- backend 镜像：

```text
crater-harbor.act.buaa.edu.cn/xpu-huangsy/ascend-910b-pytorch-backend:dev
```

命令建议：

```bash
sleep 3600
```

然后在 `kunlun-02` 上查 containerd bundle：

```bash
find /run/containerd/io.containerd.runtime.v2.task/k8s.io -name config.json
```

重点看目标容器的 `config.json`：

- `.process.env` 是否有 `ASCEND_VISIBLE_DEVICES`
- `.hooks.prestart` 是否有 `accelerator-container-hook`
- `.linux.devices` 是否有 `/dev/davinciN` 和控制设备
- `.linux.resources.devices` 是否放行对应 major/minor
- `.mounts` 是否有 hook bind mount 进来的 Ascend 路径

### 2. 如果旧版本 runtime 的 OCI spec 没有 devices

这是当前预期现象。继续构建并部署本会话中的 runtime 修复版本。

### 3. 构建并部署修复后的 toolkit 镜像

建议镜像名使用临时 debug tag，例如：

```text
crater-harbor.act.buaa.edu.cn/xpu-huangsy/accelerator-toolkit-installer:ascend-910b-oci-dev
```

本机 buildx 方式：

```bash
kubectl port-forward -n crater-images svc/buildkitd-arm 1234:1234
kubectl port-forward -n crater-images svc/buildkitd-x86 1235:1234
docker buildx build \
  --builder crater-buildkit \
  --platform linux/amd64,linux/arm64 \
  -t crater-harbor.act.buaa.edu.cn/xpu-huangsy/accelerator-toolkit-installer:ascend-910b-oci-dev \
  --push .
```

只验证 `kunlun-02` 时，也可以先只构建：

```bash
docker buildx build \
  --builder crater-buildkit \
  --platform linux/arm64 \
  -t crater-harbor.act.buaa.edu.cn/xpu-huangsy/accelerator-toolkit-installer:ascend-910b-oci-dev \
  --push .
```

部署：

```bash
make deploy \
  PROFILE=profiles/ascend-910b.yaml \
  IMAGE_REGISTRY=crater-harbor.act.buaa.edu.cn/xpu-huangsy \
  IMAGE_TAG=ascend-910b-oci-dev
```

注意：`Makefile` 默认镜像名会拼成：

```text
$(IMAGE_REGISTRY)/installer:$(IMAGE_TAG)
```

如果希望镜像名是 `accelerator-toolkit-installer`，需要直接设置 `IMAGE=`：

```bash
make deploy \
  PROFILE=profiles/ascend-910b.yaml \
  IMAGE=crater-harbor.act.buaa.edu.cn/xpu-huangsy/accelerator-toolkit-installer:ascend-910b-oci-dev
```

### 4. 等 toolkit DaemonSet 更新并检查 kunlun-02

```bash
kubectl rollout status daemonset/accelerator-toolkit -n kube-system
kubectl get pods -n kube-system -o wide | grep accelerator-toolkit
kubectl logs -n kube-system <accelerator-toolkit-pod-on-kunlun-02>
```

需要确认 installer 已重新写入：

- `/usr/local/bin/accelerator-container-runtime`
- `/usr/local/bin/accelerator-container-hook`
- `/etc/accelerator-toolkit/config.json`
- active profile

必要时检查节点上的二进制时间戳或 hash。

### 5. 重跑 smoke

```bash
kubectl apply -f experiments/ascend-910b/pytorch-backend/k8s/smoke-job.yaml
kubectl logs -n crater-workspace job/ascend-910b-backend-smoke
```

成功的第一阶段标准：

- 容器内可见 `/dev/davinciN`
- 容器内可见：
  - `/dev/davinci_manager`
  - `/dev/devmm_svm`
  - `/dev/hisi_hdc`
- `npu-smi` 可执行或至少路径可见
- `torch` / `torch_npu` import 成功
- `torch_npu.npu.is_available()` 返回 true

## 如果修复后仍失败

按下面分支定位。

### A. OCI spec 有 devices，但容器内仍无设备

重点查：

- runc 是否接收并应用了 `linux.devices`
- device cgroup 是否阻止访问
- containerd shim 目录中最终 spec 是否被二次覆盖
- rootless/userns/seccomp 是否影响设备创建

### B. OCI spec 没有 devices

重点查：

- runtime 是否确实更新到了新二进制
- runtime 是否进入 `create` 分支
- `ASCEND_VISIBLE_DEVICES` 是否在 runtime 读取 spec 时已经存在
- profile 是否是 `ascend-910b`
- runtime 是否能在宿主机 namespace 看到 `/dev/davinci*`

### C. devices 有了，但 torch_npu 不可用

这时才回到 backend 镜像问题：

- `torch` / `torch_npu` 版本是否匹配；
- CANN runtime/toolkit 路径是否正确；
- `LD_LIBRARY_PATH`、`ASCEND_OPP_PATH`、`PYTHONPATH` 是否完整；
- `npu-smi info` 是否正常。

## 镜像拉取注意

如果需要 `busybox:1.36`，不要直接用 Docker Hub，使用：

```text
crater-harbor.act.buaa.edu.cn/docker.io/library/busybox:1.36
```

## 未处理事项

- 当前代码改动未提交。
- toolkit 修复镜像未构建、未部署。
- smoke Job 需要在修复版本部署后重跑。
- git status 中存在未跟踪文件：

```text
research/direction-a-practical-work-literature-baselines.md
```

该文件不是本轮修复核心，未处理。
