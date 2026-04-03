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
- `profiles/iluvatar-bi-v150.yaml` 和 `profiles/ascend-910b.yaml` 是当前仓库里的已知样本
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

截至 2026-04-03，下面这些结论已经在仓库文档和真实节点验证中确认：

- 核心链路已实现
- 已引入首版 YAML profile schema 与多个样本 profile
- `pkg/profile` 已支持 YAML profile 加载、校验和未知字段拦截
- `pkg/runtimeview` 已把 active profile 收敛成统一运行视图
- installer / runtime / hook 已支持显式 `ACCELERATOR_PROFILE_FILE`
- installer 会把活动 profile 复制到宿主机 `/etc/accelerator-toolkit/profiles/active.yaml`
- RuntimeClass 已改为可由 profile 渲染生成
- DaemonSet 已改为可由 profile 渲染生成
- `pkg/device` 已支持基于 profile 的 `deviceGlobs`、mapping command、mapping env、mapping parser
- runtime 已支持基于 profile 向 OCI spec 注入 `extraEnv`
- 已支持不同 selector env 和不同设备映射命令
- 已支持 opaque device identifier 到设备节点映射
- mapping command 不可用时有降级路径
- hook 已支持写入 `ld.so.conf.d` 并运行 `ldconfig`
- 主执行链已要求 profile 必选，不再内置单厂商默认值

今天这轮工作的阶段总结见：

- [2026-04-03 工作总结](/home/huangsy/project/ix-container-toolkit/docs/2026-04-03-work-summary.md)

## 部署入口

推荐入口：

- `make render-runtimeclass PROFILE=profiles/<vendor>.yaml`
- `make render-daemonset PROFILE=profiles/<vendor>.yaml IMAGE=<registry>/installer:<tag>`
- `make render-bundle PROFILE=profiles/<vendor>.yaml IMAGE=<registry>/installer:<tag>`
- `make deploy PROFILE=profiles/<vendor>.yaml IMAGE=<registry>/installer:<tag>`

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

- [CLAUDE.md](/home/huangsy/project/ix-container-toolkit/CLAUDE.md)：项目背景、当前架构、边界和风险
- [docs/2026-04-03-work-summary.md](/home/huangsy/project/ix-container-toolkit/docs/2026-04-03-work-summary.md)：今天这轮大重构的阶段总结、当前判断和下一步建议
- [docs/project-status.md](/home/huangsy/project/ix-container-toolkit/docs/project-status.md)：项目状态与已完成功能
- [docs/pod-analysis.md](/home/huangsy/project/ix-container-toolkit/docs/pod-analysis.md)：真实 Pod 对比与 Device Plugin 行为分析
- [docs/physical-node-validation.md](/home/huangsy/project/ix-container-toolkit/docs/physical-node-validation.md)：物理节点验证记录
