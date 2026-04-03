# 天数 GPU Pod 对比分析报告

> 分析日期：2026-04-02
> 分析节点：inspur-01
> GPU 型号：Iluvatar BI-V150 (32GB) × 8（宿主机共 8 张）

---

## 一、实验设置

| 属性 | corex 镜像 Pod | Ubuntu 纯净镜像 Pod |
|------|----------------|---------------------|
| Pod 名称 | `sg-huangsy-260402-346e3-default0-0` | `sg-huangsy-260402-440ca-default0-0` |
| 镜像 | `tianshu/corex:4.3.0` | `library/ubuntu:22.04` |
| GPU 请求 | `iluvatar.com/gpu: 2` | `iluvatar.com/gpu: 2` |
| 调度节点 | inspur-01 | inspur-01 |
| 调度器 | volcano | volcano |

两个 Pod 的 spec 完全相同（资源、安全上下文、volume 等），唯一区别是镜像。

---

## 二、天数 Device Plugin 的工作内容

通过对比两个 Pod，Device Plugin 对**所有申请了 `iluvatar.com/gpu` 的 Pod**（无论镜像）执行以下操作：

### 2.1 注入设备节点

| Pod | 设备节点 | 设备 major:minor |
|-----|----------|-----------------|
| corex 镜像 | `/dev/iluvatar2`, `/dev/iluvatar3` | 508:2, 508:3 |
| Ubuntu 镜像 | `/dev/iluvatar1`, `/dev/iluvatar4` | 508:1, 508:4 |

**关键发现：**
- 设备编号是宿主机全局编号（不是从 0 开始的连续编号）
- 设备 major number 统一为 `508`
- 权限为 `crw-rw-rw-`（666），容器内任意用户均可访问
- **没有** `/dev/iluvatarctl` 等控制节点

### 2.2 注入环境变量

Device Plugin 向容器注入了两个环境变量：

```
ILUVATAR_COREX_VISIBLE_DEVICES=GPU-c22ac027-569b-548c-93dd-5ec7ef8eca9a,GPU-3767df9b-5e64-5279-9def-4e8e28374bf9
ILUVATAR_COREX_REPLICA_DEVICES=GPU-c22ac027-569b-548c-93dd-5ec7ef8eca9a::0,GPU-3767df9b-5e64-5279-9def-4e8e28374bf9::0
```

**关键发现：**
- 环境变量名为 `ILUVATAR_COREX_VISIBLE_DEVICES`（不是之前假设的 `ILUVATAR_VISIBLE_DEVICES`）
- 值为 GPU UUID 格式（`GPU-xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx`），不是数字 index
- `ILUVATAR_COREX_REPLICA_DEVICES` 附加了 `::0` 后缀，应为副本/vGPU 编号

### 2.3 Device Plugin 没有做的事

- **没有** 挂载任何驱动库目录
- **没有** 挂载 `/usr/local/corex` 或其下任何路径
- **没有** 设置 `LD_LIBRARY_PATH`、`PATH`、`PYTHONPATH`
- **没有** 写入 `/etc/ld.so.conf.d/` 配置
- **没有** 注入 OCI prestart hook
- **没有** 使用自定义 RuntimeClass

---

## 三、两个 Pod 的挂载对比

### 3.1 Kubernetes 层面的 Volume Mount（两者完全一致）

| 挂载路径 | 来源 | 说明 |
|----------|------|------|
| `/dev/shm` | emptyDir (Memory) | 共享内存 |
| `/crater-start.sh` | ConfigMap `custom-start-configmap` | 启动脚本 |
| `/etc/volcano` | ConfigMap `*-svc` | Volcano 调度信息 |
| `/var/run/secrets/kubernetes.io/serviceaccount` | projected volume | K8s ServiceAccount token |

### 3.2 文件系统层面的差异（来自镜像本身）

| 路径 | corex 镜像 | Ubuntu 镜像 |
|------|-----------|-------------|
| `/usr/local/corex` → `/usr/local/corex-4.3.0` | 存在（镜像内置，~16.5GB） | **不存在** |
| `/usr/local/openmpi` | 存在（镜像内置） | **不存在** |
| `/dev/iluvatar*` | 2 个设备节点 | 2 个设备节点 |
| `/etc/ld.so.conf.d/` | 标准系统配置（无 corex 条目） | 标准系统配置 |

**两者在 mount 层面没有任何 GPU 相关的额外挂载。所有 GPU 驱动和工具链内容均来自镜像的 overlay 文件系统层，不是外部挂载。**

---

## 四、天数官方镜像 (corex:4.3.0) 包含的完整环境

### 4.1 核心驱动库（`/usr/local/corex/lib64/`）

与宿主机内核驱动通信的底层库：

| 库文件 | 说明 |
|--------|------|
| `libcuda.so` / `libcuda.so.1` | CUDA Driver API（用户态驱动） |
| `libixthunk.so` | 天数 GPU thunk 层（内核态桥接） |
| `libixml.so` | 天数 GPU 管理库（ixsmi 依赖） |
| `libixptiinject.so` | PTI 注入库 |
| `libixsaninject.so` | Sanitizer 注入库 |
| `libixsysinject.so` | 系统 profiling 注入库 |
| `libixsysmetric.so` | 系统指标采集库 |
| `libixToolsExt.so` | 工具扩展库 |

### 4.2 CUDA 兼容运行时库（`/usr/local/corex/lib64/`）

天数提供的 CUDA 兼容层（CUDA 10.2 接口兼容）：

| 库文件 | 说明 |
|--------|------|
| `libcudart.so.10.2` | CUDA Runtime |
| `libcublas.so.10` / `libcublasLt.so.10` | cuBLAS 线性代数库 |
| `libcudnn.so.7` | cuDNN 深度学习原语库 |
| `libnccl.so.2` | NCCL 多卡通信库 |
| `libixattn.so` / `libixattnbkd.so` | 天数 Attention 算子库 |
| `libixkninject.so` | Kernel 注入库 |
| `libc10.so` / `libc10_cuda.so` | PyTorch C10 后端库 |

共计 63 个 `.so` 文件，约 14GB。

### 4.3 GPU 工具（`/usr/local/corex/bin/`）

共 133 个可执行文件：

| 类别 | 工具 |
|------|------|
| **GPU 管理** | `ixsmi`（GPU 状态监控，类似 nvidia-smi） |
| **编译器** | `nvcc`（CUDA 编译器）, `clang`/`clang++` (Clang 18.1) |
| **调试/性能** | `ixgdb`, `ixprof`, `ixsys`, `ixsan` |
| **LLVM 工具链** | `llc`, `lli`, `lld`, `llvm-*` 系列（完整 LLVM 后端） |
| **MLIR** | `mlir-opt`, `mlir-translate` 等 |
| **其他** | `fatbinary`, `PTXToLLVM`, `ixAssembler`, `ixobjdump` |

### 4.4 开发头文件（`/usr/local/corex/include/`，~187MB）

`cuda.h`, `cuda_runtime.h`, `cublas.h`, `cudnn.h`, CUB, ATen, c10, caffe2 等完整 CUDA 兼容开发头文件。

### 4.5 Python AI 框架生态（`/usr/local/corex/lib64/python3/dist-packages/`）

362 个 Python 包，关键框架：

| 包名 | 版本 |
|------|------|
| PyTorch | 2.4.1+corex.4.3.0 |
| torchvision | 0.19.1a0+corex.4.3.0 |
| torchaudio | 2.4.1+corex.4.3.0 |
| vLLM | 0.8.3+corex.4.3.0 |
| DeepSpeed | 0.16.4+corex.4.3.0 |
| Megatron-DeepSpeed | 2.6.0+corex.4.3.0 |
| Flash Attention | 2.6.3+corex.4.3.0 |
| APEX | 0.1+corex.4.3.0 |
| Triton | 3.1.0+corex.4.3.0 |
| Transformers | 4.53.1 |
| Accelerate | 1.8.1 |

所有 `+corex.4.3.0` 后缀的包都是天数针对其 GPU 定制编译的版本。

### 4.6 通信库

`/usr/local/openmpi/` — 完整的 OpenMPI 安装，用于多节点分布式训练。

### 4.7 环境变量

corex 镜像通过 Dockerfile 或 `/usr/local/corex/enable` 脚本设置：

```bash
COREX_VERSION=4.3.0
PATH=/usr/local/corex-4.3.0/bin:/usr/local/corex-4.3.0/lib64/python3/dist-packages/bin:/usr/local/openmpi/bin:$PATH
LD_LIBRARY_PATH=/usr/local/corex-4.3.0/lib64:/usr/local/openmpi/lib:/usr/local/lib:
PYTHONPATH=/usr/local/corex-4.3.0/lib64/python3/dist-packages
LANG=en_US.utf8
LC_ALL=en_US.utf8
```

---

## 五、ix-toolkit 能力分析：可注入 vs 缺少

### 5.1 ix-toolkit 可以注入（通过宿主机 bind-mount）

以下内容存在于宿主机上（由宿主机驱动安装），ix-toolkit 的 OCI hook 可以将其注入到任意纯净容器中：

| 内容 | 宿主机路径 | 容器目标路径 | 说明 |
|------|-----------|-------------|------|
| **GPU 驱动库** | `/usr/local/corex/lib64/libcuda.so*` | `/usr/local/corex/lib64/` | 用户态驱动，与内核模块通信 |
| | `/usr/local/corex/lib64/libixthunk.so` | | GPU thunk 层 |
| | `/usr/local/corex/lib64/libixml.so` | | GPU 管理库 |
| | `/usr/local/corex/lib64/libix*.so` | | 其他天数驱动组件 |
| **CUDA 运行时** | `/usr/local/corex/lib64/libcudart.so*` | `/usr/local/corex/lib64/` | CUDA Runtime |
| | `/usr/local/corex/lib64/libcublas*.so*` | | cuBLAS |
| | `/usr/local/corex/lib64/libcudnn.so*` | | cuDNN |
| | `/usr/local/corex/lib64/libnccl.so*` | | NCCL |
| **GPU 管理工具** | `/usr/local/corex/bin/ixsmi` | `/usr/local/corex/bin/` | GPU 监控 |
| **ld.so 配置** | (hook 动态生成) | `/etc/ld.so.conf.d/accelerator-toolkit.conf` | 让容器内 linker 找到驱动库 |
| **环境变量** | (hook 读取已有值) | `LD_LIBRARY_PATH`, `PATH` | 通过修改 OCI spec 注入 |

这些内容在宿主机安装驱动后就存在于 `/usr/local/corex-x.x.x/` 下，是与内核模块版本匹配的，**必须从宿主机挂载**（不能由镜像自带，否则版本不匹配会导致驱动故障）。

### 5.2 ix-toolkit 不应注入（应由镜像/用户自行提供）

以下内容属于**开发套件和应用层框架**，它们：
- 体量巨大（Python 包 + 编译器 > 14GB）
- 版本选择是用户的需求（不同用户需要不同版本的 PyTorch/vLLM）
- 与宿主机驱动版本解耦（只需 CUDA Runtime ABI 兼容即可）

| 内容 | 大小 | 原因 |
|------|------|------|
| **Python AI 框架**（PyTorch, vLLM, DeepSpeed, Transformers 等） | ~12GB | 用户需按需选择版本，应打入业务镜像 |
| **LLVM/Clang 编译工具链** | ~2.3GB | 开发编译环境，非运行时必须 |
| **CUDA 开发头文件**（`include/`） | ~187MB | 编译期依赖，运行时不需要 |
| **示例和文档**（`examples/`, `share/`） | - | 参考资料 |
| **OpenMPI** | - | 可选通信库，用户按需安装 |
| **CUDA 编译器 nvcc** | - | 编译期工具 |

### 5.3 需要项目补充的能力（当前缺少）

| 缺少的能力 | 说明 | 建议 |
|-----------|------|------|
| **UUID → 设备节点映射** | Device Plugin 传入 UUID，但 hook 需要知道 UUID 对应哪个 `/dev/iluvatarN`。当前代码假设传入的是数字 index。 | 通过 `ixsmi` 或 sysfs 查询 UUID 与设备编号的映射关系 |
| **环境变量名适配** | 当前代码使用 `ILUVATAR_VISIBLE_DEVICES`，实际为 `ILUVATAR_COREX_VISIBLE_DEVICES` | 更新 `pkg/config/config.go` 中的默认值 |
| **版本化驱动路径** | 宿主机实际路径为 `/usr/local/corex-4.3.0/`，`/usr/local/corex` 是符号链接 | hook 应 resolve symlink 或支持版本化路径 |
| **细粒度库过滤** | 宿主机 `lib64/` 包含 14GB 内容（含 Python 包），全量挂载不合适 | 只挂载 `*.so*` 动态库（排除 Python 包子目录和静态库） |

---

## 六、结论与架构建议

```
┌──────────────────────────────────────────────────────────────────┐
│                        用户 Pod                                   │
│                                                                  │
│  ┌─────────────────────┐  ┌───────────────────────────────────┐  │
│  │  镜像自带（用户选择）  │  │  ix-toolkit 从宿主机注入           │  │
│  │                     │  │                                   │  │
│  │  • Python 3.10      │  │  • /dev/iluvatarN (设备节点)       │  │
│  │  • PyTorch 2.x      │  │  • libcuda.so (驱动)              │  │
│  │  • vLLM / DeepSpeed │  │  • libixthunk.so (thunk)          │  │
│  │  • Transformers     │  │  • libcudart.so (CUDA Runtime)    │  │
│  │  • 业务代码         │  │  • libcublas.so (cuBLAS)          │  │
│  │                     │  │  • libcudnn.so (cuDNN)            │  │
│  │                     │  │  • libnccl.so (NCCL)              │  │
│  │                     │  │  • ixsmi (管理工具)                │  │
│  │                     │  │  • LD_LIBRARY_PATH / PATH 配置     │  │
│  │                     │  │  • /etc/ld.so.conf.d 配置          │  │
│  └─────────────────────┘  └───────────────────────────────────┘  │
└──────────────────────────────────────────────────────────────────┘
```

**核心思路：ix-toolkit 负责"驱动层"（与宿主机内核匹配的部分），镜像负责"应用层"（用户自选版本的 AI 框架）。这与 NVIDIA Container Toolkit 的架构完全一致。**
