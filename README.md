# accelerator-container-toolkit

通用的加速卡容器运行时注入工具链。项目目标是把“节点本地驱动事实”收敛到 YAML profile，由同一套 `runtime` / `hook` / `installer` 在不同节点上按 profile 完成设备、驱动库、工具和链接器配置注入。

## 解决的问题

在 Kubernetes 中，Device Plugin 往往只能完成资源分配和部分设备节点注入，但不会自动完成下面这些事情：

- 挂载厂商驱动目录和工具目录
- 设置 `LD_LIBRARY_PATH`
- 写入 `ld.so.conf.d`
- 注入 OCI hook

因此，纯净镜像即使拿到了设备节点，也往往无法直接加载宿主机驱动。

## 当前方案

项目采用三段式架构：

- `accelerator-container-runtime`：作为 containerd runtime shim，拦截 `create`，按需把 hook 注入 OCI spec
- `accelerator-container-hook`：作为 OCI prestart hook，在容器启动前挂载设备、驱动库、驱动工具，并刷新 `ld.so.cache`
- `accelerator-installer`：以 DaemonSet init container 形式把二进制、profile 和兼容配置安装到宿主机，并 patch containerd

当前主入口不是手工维护静态 `RuntimeClass` / `DaemonSet` YAML，而是：

- `profiles/*.yaml` 作为厂商事实输入
- `accelerator-profile-render` 负责从 profile 渲染 Kubernetes 清单
- `make deploy` / `make undeploy` 基于 profile 渲染 `RuntimeClass + DaemonSet`

当前运行时入口约定为：

- 集群统一只保留一个 `RuntimeClass`：`xpu-runtime`
- Pod 显式指定 `runtimeClassName: xpu-runtime` 以启用 toolkit
- Pod 调度到哪个节点由资源请求和调度规则决定
- 节点上的 active profile 决定该节点实际注入的设备、驱动、工具和 linker 配置

执行模型如下：

```text
节点安装 active profile
        ↓
accelerator-installer 复制二进制与 active profile
并 patch containerd runtime handler
        ↓
containerd 调用 accelerator-container-runtime create
        ↓
accelerator-container-runtime 读取节点 active profile
按 selector env 决定是否注入 accelerator-container-hook
        ↓
accelerator-container-hook 读取同一份 active profile
并按 profile 注入设备、artifact、extra env 和 linker
        ↓
容器进程启动
```

## 当前支持方式

项目现在支持“同一套二进制，不同节点使用不同 profile”：

- 每个节点宿主机保存一份 active profile，默认路径为 `/etc/accelerator-toolkit/profiles/active.yaml`
- `accelerator-container-runtime`、`accelerator-container-hook`、`accelerator-installer` 启动时都要求能成功加载这份 profile
- 如果 profile 缺失或校验失败，组件会打印错误并退出，不再回退到旧的厂商默认值
- `profiles/iluvatar-bi-v150.yaml`、`profiles/ascend-910b.yaml`、`profiles/metax-c500.yaml` 和 `profiles/nvidia-a100.yaml` 是当前仓库里的已知样本
- 混合架构集群通过多架构 installer 镜像分发，同一个 DaemonSet 即可覆盖 `amd64` / `arm64`

## 项目边界

accelerator-container-toolkit 负责的是宿主机驱动层，不负责把完整 AI 软件栈打进容器。

适合由本项目注入：

- profile 中声明的设备节点与控制节点
- profile 中声明的驱动共享库目录
- profile 中声明的工具目录或运行时辅助目录
- profile 中声明的 linker 配置与运行时额外环境变量

不由本项目负责：

- PyTorch、vLLM、DeepSpeed 等 Python 框架
- 编译器、头文件、示例和文档
- 用户业务镜像的应用层依赖

## 当前状态

截至 2026-04-24，下面这些结论已经在仓库文档和真实节点验证中确认：

- 核心链路已实现
- 已引入首版 YAML profile schema 与多个样本 profile
- `pkg/profile` 已支持 YAML profile 加载、校验和未知字段拦截
- `pkg/runtimeview` 已把 active profile 收敛成统一运行视图
- installer / runtime / hook 已支持显式 `ACCELERATOR_PROFILE_FILE`
- installer 会把活动 profile 复制到宿主机 `/etc/accelerator-toolkit/profiles/active.yaml`
- RuntimeClass 已改为可由 profile 渲染生成
- DaemonSet 已改为可由 profile 渲染生成
- runtime handler 与 `RuntimeClass` 已统一收敛为 `xpu-runtime`
- `pkg/device` 已支持基于 profile 的 `deviceGlobs`、mapping command、mapping env、mapping parser
- runtime 已支持基于 profile 向 OCI spec 注入 `extraEnv`
- runtime 已支持 `env-all` profile 的默认 selector 注入
- 已支持不同 selector env 和不同设备映射命令
- 已支持 opaque device identifier 到设备节点映射
- mapping command 不可用时有降级路径
- hook 已支持写入 `ld.so.conf.d` 并运行 `ldconfig`
- 主执行链已要求 profile 必选，不再内置单厂商默认值
- Ascend 910B 已完成 profile/runtime/backend L3 smoke 闭环
- Metax C500 已完成 MACA PyTorch backend 与 xpu-runtime 验证，包括无显式 selector env 的 `env-all` 默认注入路径
- NVIDIA A100 已完成 `xpu-runtime` delegate 验证，实际设备和驱动注入由 NVIDIA runtime wrapper 完成

稳定状态文档见：

- [项目状态](docs/project-status.md)
- [验证结果](docs/validation-results.md)
- [Ascend 910B Backend](docs/ascend-910b-backend.md)

## 部署入口

推荐入口：

- `make render-runtimeclass PROFILE=profiles/<vendor>.yaml`
- `make render-daemonset PROFILE=profiles/<vendor>.yaml IMAGE=<registry>/installer:<tag>`
- `make render-bundle PROFILE=profiles/<vendor>.yaml IMAGE=<registry>/installer:<tag>`
- `make deploy PROFILE=profiles/<vendor>.yaml IMAGE=<registry>/installer:<tag>`

说明：

- 不同 profile 仍决定节点上的实际注入行为
- 但渲染得到的 `RuntimeClass` 名称固定为 `xpu-runtime`
- 渲染得到的 installer DaemonSet 默认使用 `RESTART_CONTAINERD=false`
- 若节点侧确实需要重新加载新的 runtime handler，应在确认宿主机环境后单独重启 `containerd`

说明：

- `deployments/runtimeclass/runtimeclass.yaml`
- `deployments/daemonset/daemonset.yaml`

现在只作为历史参考清单保留，不再是部署主入口。

## 本地构建

- `make build` 默认跟随当前 `go env GOOS/GOARCH` 生成本机可执行文件
- `make build` 会同时写入平铺产物 `bin/` 和架构分层产物 `bin/<os>-<arch>/`
- 若要为单一目标节点显式构建，可覆盖：`make build GOOS=linux GOARCH=amd64`
- 构建产物包含：`accelerator-container-runtime`、`accelerator-container-hook`、`accelerator-installer`、`accelerator-profile-render`

## 多架构镜像与 DaemonSet 分发

- 推荐发布入口：`make docker-build-multiarch IMAGE=<registry>/installer:<tag>`
- 若二进制已由外部流水线预构建到 `bin/linux-amd64/`、`bin/linux-arm64/`，可用：`make docker-build-prebuilt-multiarch IMAGE=<registry>/installer:<tag>`
- 这两个目标都会发布 `linux/amd64,linux/arm64` 多架构 manifest，而不是单一架构镜像
- DaemonSet 保持单个镜像引用即可；Kubernetes 会在 `amd64`/`arm64` 节点上自动拉取匹配架构的 installer 镜像变体
- installer 镜像里携带的 `accelerator-container-runtime` 和 `accelerator-container-hook` 也是对应节点架构的版本，因此复制到宿主机后可直接执行
- 若远端 buildkit 需要显式代理，可通过 `DOCKER_BUILD_ARGS` 透传，例如：`make docker-build-multiarch DOCKER_BUILD_ARGS='--build-arg HTTP_PROXY=http://host:port --build-arg HTTPS_PROXY=http://host:port'`

## 快速定位

- [docs/README.md](docs/README.md)：文档索引
- [docs/project-status.md](docs/project-status.md)：项目状态与边界
- [docs/runtime-architecture.md](docs/runtime-architecture.md)：runtime/hook 执行链路
- [docs/generic-profile-schema.md](docs/generic-profile-schema.md)：profile schema
- [docs/hardware-profile-facts.md](docs/hardware-profile-facts.md)：硬件 profile 事实
- [docs/ascend-910b-backend.md](docs/ascend-910b-backend.md)：Ascend 910B backend 构建与 smoke 结果
- [docs/validation-results.md](docs/validation-results.md)：验证结果汇总
