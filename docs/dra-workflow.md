# DRA Driver Workflow

> accelerator-toolkit 项目基于 Kubernetes DRA（Dynamic Resource Allocation）的完整工作流程。

## 概述

accelerator-toolkit 为国产 GPU（天数 Iluvatar、华为 Ascend、沐曦 Metax）提供容器设备注入能力。通过 DRA + CDI 方案，将宿主机 GPU 设备和驱动库自动注入到容器中，用户 Pod 无需修改镜像即可使用 GPU。

与传统 Device Plugin 方案的核心区别：**DRA 让调度器直接看到每个设备的属性并参与匹配决策**，而传统方案下调度器只能看到 `nvidia.com/gpu: 1` 这样的标量计数。

## 核心组件与角色

```
┌─────────────────────────────────────────────────────────────────┐
│                        Kubernetes 集群                          │
│                                                                 │
│  ┌──────────────┐    ┌──────────────┐    ┌──────────────────┐  │
│  │ DeviceClass  │    │ ResourceSlice│    │ ResourceClaim    │  │
│  │ (管理员创建)  │    │ (DRA driver  │    │ (kubelet 创建)   │  │
│  │              │    │  上报)        │    │                  │  │
│  │ 定义设备筛选  │    │ 描述节点上有  │    │ 声明需要几块设备  │  │
│  │ 条件(CEL)    │    │ 哪些设备      │    │                  │  │
│  └──────┬───────┘    └──────┬───────┘    └────────┬─────────┘  │
│         │                   │                     │            │
│         └───────────────────┼─────────────────────┘            │
│                             ↓                                  │
│                   ┌──────────────────┐                         │
│                   │ API Server       │                         │
│                   │ DRA allocator    │                         │
│                   │ (匹配设备+写结果) │                         │
│                   └────────┬─────────┘                         │
│                            ↓                                   │
│                   ┌──────────────────┐                         │
│                   │ Scheduler        │                         │
│                   │ (读 nodeSelector │                         │
│                   │  选节点)          │                         │
│                   └────────┬─────────┘                         │
│                            ↓                                   │
│  ┌─────────────────────────────────────────────────────────┐  │
│  │                     目标节点                              │  │
│  │  ┌───────────┐    ┌───────────┐    ┌──────────────────┐ │  │
│  │  │  kubelet  │───→│ DRA Driver│───→│ containerd       │ │  │
│  │  │           │gRPC│ (DaemonSet│CDI │ (按 CDI spec     │ │  │
│  │  │           │    │  Pod)     │    │  注入设备)        │ │  │
│  │  └───────────┘    └───────────┘    └──────────────────┘ │  │
│  └─────────────────────────────────────────────────────────┘  │
└─────────────────────────────────────────────────────────────────┘
```

各组件职责：

| 组件 | 职责 | 创建者 |
|------|------|--------|
| **DeviceClass** | 定义设备筛选条件（CEL 表达式）和默认配置 | 管理员 |
| **ResourceSlice** | 描述节点上可用的设备及其属性 | DRA driver |
| **ResourceClaim** | 声明 Pod 需要的设备资源 | kubelet（来自模板） |
| **DRA allocator** | 匹配 ResourceClaim 与 ResourceSlice，写入分配结果 | API Server 内置 |
| **Scheduler** | 读取分配结果中的 nodeSelector，将 Pod 调度到目标节点 | K8s 内置 |
| **DRA driver** | 发现设备、上报 ResourceSlice、准备 CDI spec | 本项目实现 |

## 项目代码结构

```
accelerator-toolkit/
├── cmd/accelerator-dra-driver/     # DRA driver 二进制入口
├── pkg/
│   ├── dra/                        # DRA plugin 实现
│   │   ├── plugin.go               # PrepareResourceClaims / UnprepareResourceClaims
│   │   └── resourceslice.go        # ResourceSlice 构建器
│   ├── cdi/                        # CDI spec 生成器
│   │   ├── generator.go            # 按设备粒度生成 CDI spec
│   │   ├── types.go                # CDI 1.1.0 类型定义
│   │   └── writer.go               # 写入 /etc/cdi/<vendor>.json
│   ├── device/                     # 设备发现（厂商无关）
│   │   └── device.go               # glob 扫描 + UUID 映射
│   └── profile/                    # Profile 驱动配置
│       ├── profile.go              # Profile 结构体加载/校验
│       └── render.go               # K8s 清单渲染
├── profiles/                       # 厂商 profile YAML
│   ├── iluvatar-bi-v150.yaml
│   ├── ascend-910b.yaml
│   ├── metax-c500.yaml
│   └── nvidia-a100.yaml
└── deployments/dra-driver/         # K8s 部署清单
    ├── daemonset.yaml
    └── rbac.yaml
```

## 完整工作流程

整个 DRA 工作流程分为 6 个阶段，按时间顺序排列。

### 阶段 1：管理员准备资源

管理员需要完成两件事：创建 DeviceClass 和部署 DRA driver。

**创建 DeviceClass**

DeviceClass 是集群级资源（cluster-scoped），定义一类设备的筛选条件。用户 ResourceClaim 通过 `deviceClassName` 引用它。

```yaml
apiVersion: resource.k8s.io/v1
kind: DeviceClass
metadata:
  name: iluvatar.com/gpu
spec:
  selectors:
    - cel:
        expression: |
          device.driver == "gpu.accelerator-toolkit.io" &&
          device.attributes["gpu.accelerator-toolkit.io"].vendor.stringValue == "iluvatar"
  config:
    - opaque:
        driver: gpu.accelerator-toolkit.io
        parameters: {}
```

CEL 表达式中可访问的设备对象属性：

| 字段 | 类型 | 说明 |
|------|------|------|
| `device.driver` | string | DRA driver 名称 |
| `device.attributes` | map | 设备属性，按驱动前缀分组 |
| `device.capacity` | map | 设备容量（Quantity 类型） |

**部署 DRA Driver**

```bash
# 方式 1：使用 profile-render 工具
go run ./cmd/accelerator-profile-render bundle \
  --profile profiles/iluvatar-bi-v150.yaml \
  --image accelerator-toolkit/dra-driver:latest | kubectl apply -f -

# 方式 2：直接 apply 静态清单
kubectl apply -f deployments/dra-driver/rbac.yaml
kubectl apply -f deployments/dra-driver/daemonset.yaml
```

### 阶段 2：DRA Driver 发现并上报设备

每个节点上的 DRA driver 启动后，执行设备发现并向 API Server 上报 ResourceSlice。

```
DRA Driver 启动
     ↓
加载 profile YAML（如 /profiles/iluvatar-bi-v150.yaml）
     ↓
提取 DeviceGlobs（如 ["/dev/iluvatar*"]）
     ↓
filepath.Glob() 扫描 /dev/ 目录
     ↓
过滤字符设备文件（os.ModeDevice）
     ↓
提取文件名末尾数字作为 index（iluvatar0 → 0）
     ↓
[可选] 调用厂商 CLI 做 UUID 映射
     ↓
返回 []Device{Path, Index, UUID}
     ↓
kubeletplugin.Start() 向 kubelet 注册 gRPC 服务
     ↓
PublishResources() → 上报 ResourceSlice 到 API Server
```

厂商差异化的发现参数在 `profiles/` 下的 YAML 中定义：

| 厂商 | 设备 glob | 映射策略 | 映射命令 |
|------|-----------|----------|----------|
| 天数 (Iluvatar) | `/dev/iluvatar*` | `command-csv-index-uuid` | `ixsmi --query-gpu=index,uuid --format=csv` |
| 华为 (Ascend) | `/dev/davinci*` | `env-index-list` | 无，直接用 index |
| 沐曦 (Metax) | `/dev/dri/card*`, `/dev/dri/renderD*` | `env-all` | 无，暴露全部设备 |
| NVIDIA | `/dev/nvidia[0-9]*` | `delegate-runtime` | 委托给 nvidia-container-runtime |

上报后的 ResourceSlice 内容：

```yaml
apiVersion: resource.k8s.io/v1
kind: ResourceSlice
metadata:
  name: node1-gpu-accelerator-toolkit-io-0
spec:
  driver: gpu.accelerator-toolkit.io
  pool:
    name: node1
    generation: 1
    resourceSliceCount: 1
  nodeName: node1
  devices:
    - name: GPU-aaaa                      # 设备 UUID（无 UUID 时用 "index-N"）
      attributes:
        gpu.accelerator-toolkit.io:
          vendor: {stringValue: iluvatar}
          model: {stringValue: BI-V150}
          uuid: {stringValue: GPU-aaaa}
          path: {stringValue: /dev/iluvatar0}
    - name: GPU-bbbb
      attributes:
        gpu.accelerator-toolkit.io:
          vendor: {stringValue: iluvatar}
          model: {stringValue: BI-V150}
          uuid: {stringValue: GPU-bbbb}
          path: {stringValue: /dev/iluvatar1}
    # ... 每块 GPU 一个条目
```

| 字段 | 说明 |
|------|------|
| `spec.driver` | DRA driver 名称，与 kubeletplugin 注册时一致 |
| `spec.pool.name` | 资源池名称，通常为节点名 |
| `spec.pool.generation` | 资源版本号，每次更新递增 |
| `spec.pool.resourceSliceCount` | 同 pool 的 ResourceSlice 数量（设备 >128 时拆分） |
| `spec.nodeName` | 绑定到哪个节点 |
| `spec.devices[].name` | 设备唯一名称（UUID 或 index-N） |
| `spec.devices[].attributes` | 设备属性，带驱动前缀，用于 CEL 选择器匹配 |

### 阶段 3：用户提交 Pod

用户创建需要 GPU 的 Pod，通过 ResourceClaim 声明资源需求：

```yaml
apiVersion: v1
kind: Pod
metadata:
  name: my-training-job
spec:
  resourceClaims:
    - name: gpu
      resourceClaimTemplateName: gpu-claim
  containers:
    - name: trainer
      image: my-training:latest
      resources:
        claims:
          - name: gpu
        limits:
          memory: 64Gi
---
apiVersion: resource.k8s.io/v1
kind: ResourceClaimTemplate
metadata:
  name: gpu-claim
spec:
  spec:
    devices:
      requests:
        - name: gpu-0
          exactly:
            deviceClassName: iluvatar.com/gpu   # 引用 DeviceClass
            selectors:                           # 可选：追加 CEL 选择器
              - cel:
                  expression: |
                    device.attributes["gpu.accelerator-toolkit.io"].model.stringValue == "BI-V150"
            allocationMode: ExactCount
            count: 1
```

`DeviceRequest` 支持两种模式：
- **`exactly`**：精确请求，指定 DeviceClass、选择器、数量
- **`firstAvailable`**：优先级列表，调度器按顺序尝试，选择第一个能满足的（需要 `DRAPrioritizedList` feature gate）

### 阶段 4：API Server 分配设备

这是 DRA 与传统方案最关键的区别——**设备匹配在调度器选节点之前完成**。

```
传统 Device Plugin：
  scheduler 看资源计数 → 选节点 → kubelet 分配设备
  scheduler 不知道设备属性

DRA：
  API Server allocator 看设备属性做匹配 → 写入 nodeSelector
  scheduler 拿到 nodeSelector 选节点 → kubelet 准备设备
```

API Server 内置的 DRA allocator 处理流程：

```
1. 读取 ResourceClaim.spec.devices.requests
2. 解析 deviceClassName → 查找 DeviceClass
3. 合并选择器：DeviceClass.selectors ∪ Request.selectors（取交集）
4. 遍历该 driver 的所有 ResourceSlice 中的设备
5. 对每个设备执行所有 CEL 表达式
6. 选出全部匹配的设备
7. 按 AllocationMode 确定分配数量
   - ExactCount（默认）：分配 count 个
   - All：分配所有匹配设备
8. 写入 ResourceClaim.status.allocation
```

分配结果写入 ResourceClaim.status：

```yaml
status:
  allocation:
    devices:
      results:
        - request: gpu-0
          driver: gpu.accelerator-toolkit.io
          pool: node1
          device: GPU-aaaa
    nodeSelector:
      nodeSelectorTerms:
        - matchFields:
            - key: metadata.name
              operator: In
              values: [node1]      # 限定 Pod 只能调度到 node1
```

### 阶段 5：Scheduler 调度 + kubelet 准备设备

Scheduler 读取 ResourceClaim.status.allocation 中的 nodeSelector，将 Pod 调度到目标节点。

目标节点上的 kubelet 发现 ResourceClaim 已分配，通过 gRPC 调用 DRA driver：

```
NodePrepareResources(claimUID, claimName)
```

DRA driver 的 `PrepareResourceClaims` 处理：

```
1. 从 claim.status.allocation.devices.results 找到分配的设备 GPU-aaaa
2. 匹配到本地设备 /dev/iluvatar0
3. 调用 cdi.NewGenerator(profile, [GPU-aaaa]) 生成 CDI spec
4. 调用 cdi.WriteSpec() 写入 /etc/cdi/iluvatar.json
5. 返回 CDI device ID: "iluvatar.com/gpu=GPU-aaaa"
```

### 阶段 6：containerd 注入设备

kubelet 将 CDI device ID 传递给 containerd，containerd 按 CDI spec 注入设备：

```yaml
# /etc/cdi/iluvatar.json
cdiVersion: 1.1.0
kind: iluvatar.com/gpu
devices:
  - name: GPU-aaaa
    containerEdits:
      env:
        - ILUVATAR_COREX_VISIBLE_DEVICES=all
      deviceNodes:
        - hostPath: /dev/iluvatar0
          path: /dev/iluvatar0
          permissions: rwm
      mounts:
        - hostPath: /usr/local/corex/lib64/libcuda.so
          containerPath: /usr/local/corex/lib64/libcuda.so
          type: bind
          options: [rbind, ro]
        # ... 其他驱动库
      hooks:
        prestart:
          - hookName: ldconfig
            path: /sbin/ldconfig
```

containerd 执行：
- bind-mount `/dev/iluvatar0` 到容器
- bind-mount 驱动库到容器内的 `/usr/local/corex/...`
- 执行 ldconfig hook 刷新动态链接器缓存
- 容器启动，可直接访问 GPU

## DRA Driver 的三个阶段

| 阶段 | 时机 | 作用 | 交互对象 |
|------|------|------|----------|
| **注册** | 启动时 | 发现设备，发布 ResourceSlice | API Server（写入 ResourceSlice） |
| **待机** | 运行时 | ResourceSlice 持续存在供 allocator 查询 | 无主动交互 |
| **准备** | Pod 调度后 | 生成 CDI spec，注入设备到容器 | kubelet（gRPC 调用） |

DRA driver 不参与分配决策，它是**设备信息的发布者**和**CDI spec 的生成者**。分配决策完全由 API Server 的 DRA allocator 完成。

## 与旧方案对比

| 维度 | 旧方案（hook/runtime） | DRA 方案 |
|------|----------------------|----------|
| 设备分配 | Device Plugin + 环境变量 | DRA allocator + ResourceClaim |
| 设备注入 | OCI prestart hook 手动 bind-mount | CDI spec + containerd 原生支持 |
| 驱动库挂载 | hook 里手动 mount | CDI spec 声明，containerd 执行 |
| ldconfig | hook 里 exec ldconfig | CDI hook 声明 |
| 组件数 | 3 个（runtime shim + hook + installer） | 1 个（dra-driver） |
| 调度感知 | scheduler 只看资源计数 | scheduler 通过 nodeSelector 知道具体设备位置 |
| K8s 版本要求 | 任意 | 1.31+（DRA beta），1.35+（推荐） |
| containerd 版本 | 任意 | 2.x（CDI 原生支持） |

## 多厂商支持

DRA driver 通过 profile 机制支持多厂商。切换厂商只需：

1. 替换 profile YAML 文件
2. 更新 DaemonSet 的 `--profile` 参数和 `nodeSelector`
3. 创建对应的 DeviceClass

各厂商的差异（设备节点、驱动路径、映射命令、环境变量）全部在 profile 中声明，DRA driver 核心代码无需修改。
