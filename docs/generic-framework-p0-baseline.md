# ix-container-toolkit 通用化改造 P0 基线

> 更新日期：2026-04-03
> 作用：作为 `docs/generic-framework-plan.md` 中 `P0` 阶段的事实输入，收敛当前代码真实行为、Iluvatar 硬编码项和首版 profile 字段映射。

## 一、结论先行

当前仓库已经不是“只有 Iluvatar 专用设计草图”，而是已经具备可运行的 Iluvatar 单厂商链路：

- `accelerator-installer` 负责写宿主机配置、注册 `containerd` runtime、给节点打标签
- `accelerator-container-runtime` 负责在 `create` 阶段向 OCI spec 注入 `prestart hook`
- `accelerator-container-hook` 负责在容器启动前把设备、驱动库、驱动工具和 linker 配置注入到容器 rootfs

因此，后续通用化改造的起点不是重写 runtime/hook/installer，而是：

- 把现有 Iluvatar 事实从代码里抽取出来
- 把 Iluvatar 专用常量迁入 profile
- 把当前按路径类别编码的注入逻辑演进为 profile 驱动逻辑

## 二、当前代码真实行为

### 2.1 `accelerator-container-runtime`

入口：

- `cmd/accelerator-container-runtime/main.go`
- `internal/runtime/runtime.go`

真实行为：

- 只拦截 OCI runtime 的 `create` 子命令
- 解析 `--bundle` / `-bundle` / `-b` 参数
- 读取 bundle 内 `config.json`
- 检测容器 env 中是否包含配置项 `hook.deviceListEnvvar`
- 若命中，则将 `cfg.HookPath` 前插到 `hooks.prestart`
- 其余命令原样委托给底层 `runc`

当前限制：

- 当前“是否需要注入”的判断只有 env 命中这一种策略
- hook stage 固定为 `prestart`
- 仍然默认依赖 Iluvatar 语义的 env 名和值格式

### 2.2 `accelerator-container-hook`

入口：

- `cmd/accelerator-container-hook/main.go`
- `internal/hook/hook.go`

真实行为：

- 从 stdin 读取 OCI state
- 打开 bundle 内 `config.json`
- 从 `hook.deviceListEnvvar` 读取可见设备值
- `""` 且 `disableRequire=false` 时直接跳过
- `none` 时直接跳过
- 通过 `pkg/device.Discover()` 枚举并过滤宿主机设备
- 将匹配设备注入 `<rootfs>/dev/...`
- 注入驱动库目录和驱动工具目录
- 写入 `<rootfs>/etc/ld.so.conf.d/accelerator-toolkit.conf`
- 在 rootfs 内执行 `ldconfig`

真实注入模型：

- 设备节点：bind mount
- 共享库：`so-only` 模式下复制普通文件、重建 symlink、对子目录做 bind mount
- 驱动工具目录：由 hook 按目录注入
- linker 配置：写配置文件 + 运行 `ldconfig`

当前限制：

- 注入行为仍按“设备 / 库 / 工具 / linker”几类逻辑硬编码
- OCI env 本身并没有被 hook 改写
- artifact 还不是声明式数据模型

### 2.3 `pkg/device`

入口：

- `pkg/device/device.go`

真实行为：

- 枚举 `/dev/iluvatar*`
- 跳过非数字后缀节点
- 支持 `all` / `none` / index 列表 / UUID 列表
- UUID 模式优先调用 `ixsmi --query-gpu=index,uuid --format=csv`
- `ixsmi` 失败时降级为 positional UUID -> index 映射

当前限制：

- 设备前缀固定为 `/dev/iluvatar`
- UUID 查询命令、参数和 `LD_LIBRARY_PATH` 拼装都硬编码为 Iluvatar/corex 事实
- fallback 只有现有几种内置策略，尚未配置化

### 2.4 `accelerator-installer`

入口：

- `cmd/accelerator-installer/main.go`

真实行为：

- 将 `accelerator-container-runtime`、`accelerator-container-hook` 复制到宿主机
- 写入 `/etc/accelerator-toolkit/config.json`
- patch `/etc/containerd/config.toml`，注册 `runtimes.ix`
- 可选重启 `containerd`
- 调 Kubernetes API 给节点打 `iluvatar.ai/gpu=present`

当前限制：

- runtime handler 固定为 `ix`
- 生成的配置模型仍是 Iluvatar 专用 `config.json`
- 节点标签和值固定为 `iluvatar.ai/gpu=present`

## 三、现有能力与文档目标的边界

下面这些能力，代码已经具备，不应在后续设计中再被当作“待实现目标”：

| 能力 | 当前状态 |
|---|---|
| runtime 在 `create` 阶段注入 hook | 已实现 |
| hook 从 OCI state + spec 读取上下文 | 已实现 |
| `ILUVATAR_COREX_VISIBLE_DEVICES` 检测 | 已实现 |
| `none` 静默跳过 | 已实现 |
| UUID -> index 映射 | 已实现 |
| `ixsmi` 不可用时降级 | 已实现 |
| driver path symlink 解析与去重 | 已实现 |
| `ld.so.conf.d` 写入 | 已实现 |
| `ldconfig` 执行 | 已实现 |
| RuntimeClass 部署清单 | 已实现 |
| DaemonSet `pause` keep-alive | 已实现 |
| 节点自动打标 | 已实现 |

下面这些能力，仍然属于通用化阶段的新增工作或未完全完成的工作：

| 能力 | 当前状态 |
|---|---|
| YAML profile schema | 未实现 |
| profile 加载/校验层 | 未实现 |
| 将 handler / RuntimeClass / node label 参数化 | 未实现 |
| 将设备映射策略配置化 | 已有首版实现，未完全去除兼容层 |
| 将注入行为改为 artifact 驱动 | 已有首版实现，旧路径仍保留 |
| 多厂商 profile 样本 | 未实现 |
| Ascend 310P / 910B facts/profile | 未实现 |

## 四、Iluvatar 硬编码项清单

### 4.1 命名与运行时

| 硬编码项 | 当前值 | 位置 | 备注 |
|---|---|---|---|
| runtime handler | `ix` | `cmd/accelerator-installer/main.go` | 应迁入 profile/runtime |
| RuntimeClass 名称 | `ix` | `deployments/runtimeclass/runtimeclass.yaml` | 应与 handler 解耦 |
| hook 二进制名 | `accelerator-container-hook` | `pkg/config/config.go` 等 | 可保留默认值，但不应成为 vendor 语义载体 |
| runtime 二进制名 | `accelerator-container-runtime` | 安装与部署清单 | 同上 |

### 4.2 Kubernetes 语义

| 硬编码项 | 当前值 | 位置 | 备注 |
|---|---|---|---|
| 资源名 | `iluvatar.ai/gpu` | `pkg/config/config.go` | 后续应迁入 profile/kubernetes |
| 节点标签 | `iluvatar.ai/gpu=present` | installer / DaemonSet / RuntimeClass | 后续应参数化 |
| RuntimeClass scheduling | Iluvatar 节点标签/容忍度 | `deployments/runtimeclass/runtimeclass.yaml` | 应按 profile 生成 |

### 4.3 设备发现与映射

| 硬编码项 | 当前值 | 位置 | 备注 |
|---|---|---|---|
| 设备前缀 | `/dev/iluvatar` | `pkg/config/config.go` | 后续应变为 `device.deviceGlobs` |
| 可见设备 env | `ILUVATAR_COREX_VISIBLE_DEVICES` | `pkg/config/config.go` | 后续应支持多 env/多格式 |
| UUID 工具路径候选 | `/usr/local/corex/bin/ixsmi`、`/usr/local/corex-4.3.0/bin/ixsmi` | `pkg/device/device.go` | 当前完全是 Iluvatar 假设 |
| UUID 查询命令 | `ixsmi --query-gpu=index,uuid --format=csv` | `pkg/device/device.go` | 后续应建模为 mapping command |
| UUID 查询运行环境 | `LD_LIBRARY_PATH=/usr/local/corex/...` | `pkg/device/device.go` | 后续应迁入 mapping env |

### 4.4 文件注入

| 硬编码项 | 当前值 | 位置 | 备注 |
|---|---|---|---|
| 容器内驱动根目录 | `/usr/local/corex` | `pkg/config/config.go` | 后续应迁入 profile/inject |
| 驱动库路径 | `/usr/local/corex/lib64`、`/usr/local/corex/lib` | `pkg/config/config.go` | 仍是 Iluvatar 默认 |
| 驱动工具路径 | `/usr/local/corex/bin` | `pkg/config/config.go` | 同上 |
| linker 配置文件名 | `accelerator-toolkit.conf` | `internal/hook` | 应抽象成 linker policy |
| 共享库过滤模式默认值 | `so-only` | `pkg/config/config.go` | 可保留为通用默认，但不应只由 Iluvatar 配置驱动 |

### 4.5 部署与镜像

| 硬编码项 | 当前值 | 位置 | 备注 |
|---|---|---|---|
| DaemonSet 镜像名 | `ix-toolkit/installer:latest` | `deployments/daemonset/daemonset.yaml` | 部署层硬编码，与通用化无强耦合，但需记录 |
| Host 配置目录 | `/etc/accelerator-toolkit` | installer/config | 可保留项目级默认值 |
| Host 二进制目录 | `/usr/local/bin` | installer/config | 可保留项目级默认值 |

## 五、Iluvatar 首版 profile 草稿字段表

本节不是最终 schema，只是把“当前 Iluvatar 事实”先映射成可抽象字段。

### 5.1 `metadata`

| 字段 | Iluvatar 当前值 | 来源 |
|---|---|---|
| `name` | `iluvatar-bi-v150` | P0 草稿命名 |
| `vendor` | `Iluvatar` | 项目现状 |
| `modelFamily` | `BI-V150` | 真实节点验证 |
| `version` | `v1alpha1` | schema 草稿版本 |

### 5.2 `runtime`

| 字段 | Iluvatar 当前值 | 来源 |
|---|---|---|
| `handlerName` | `ix` | installer / RuntimeClass |
| `runtimeClassName` | `ix` | deployment |
| `underlyingRuntime` | `runc` | `pkg/config` 默认值 |
| `hookStage` | `prestart` | runtime 实现事实 |
| `hookBinary` | `/usr/local/bin/accelerator-container-hook` | `pkg/config` 默认值 |

### 5.3 `kubernetes`

| 字段 | Iluvatar 当前值 | 来源 |
|---|---|---|
| `resourceNames` | `["iluvatar.ai/gpu"]` | `pkg/config.ResourceName` |
| `nodeLabels` | `{"iluvatar.ai/gpu":"present"}` | installer / DaemonSet |
| `runtimeClassScheduling.nodeSelector` | `{"iluvatar.ai/gpu":"present"}` | RuntimeClass 清单 |
| `runtimeClassScheduling.tolerations` | `[{key:"iluvatar.ai/gpu",operator:"Exists",effect:"NoSchedule"}]` | RuntimeClass 清单 |

### 5.4 `device`

| 字段 | Iluvatar 当前值 | 来源 |
|---|---|---|
| `selectorEnvVars` | `["ILUVATAR_COREX_VISIBLE_DEVICES"]` | `pkg/config` |
| `selectorFormats` | `["all","none","index-list","uuid-list"]` | `pkg/device` 行为 |
| `deviceGlobs` | `["/dev/iluvatar*"]` | `pkg/device` |
| `controlDeviceGlobs` | `[]` | 真实节点未发现控制节点 |
| `mappingStrategy.primary` | `command-csv-index-uuid` | `pkg/device.ixsmiQuery()` |
| `mappingStrategy.fallback` | `uuid-positional` | `pkg/device.filterByUUIDPositional()` |
| `mappingCommand.pathCandidates` | `["/usr/local/corex/bin/ixsmi","/usr/local/corex-4.3.0/bin/ixsmi","ixsmi"]` | `pkg/device` |
| `mappingCommand.args` | `["--query-gpu=index,uuid","--format=csv"]` | `pkg/device` |
| `mappingEnv` | `{"LD_LIBRARY_PATH":"/usr/local/corex/lib64:/usr/local/corex/lib:/usr/local/corex-4.3.0/lib64:/usr/local/corex-4.3.0/lib"}` | `pkg/device` |
| `mappingParser` | `csv-header(index,uuid)` | `pkg/device` |
| `fallbackPolicy` | `skip-none`, `error-on-empty`, `degrade-on-ixsmi-failure` | hook + device 行为 |

### 5.5 `inject`

| 字段 | Iluvatar 当前值 | 来源 |
|---|---|---|
| `containerRoot` | `/usr/local/corex` | `pkg/config` |
| `artifacts.devices` | `/dev/iluvatarN` bind mount | `internal/hook` |
| `artifacts.libraries` | `["/usr/local/corex/lib64","/usr/local/corex/lib"]` | `pkg/config` |
| `artifacts.binaries` | `["/usr/local/corex/bin"]` | `pkg/config` |
| `libraryFilter.mode` | `so-only` | `pkg/config` |
| `libraryFilter.excludeDirs` | `["python3","cmake","clang"]` | `pkg/config` |
| `linker.configPath` | `/etc/ld.so.conf.d/accelerator-toolkit.conf` | hook 行为 |
| `linker.runLdconfig` | `true` | hook 行为 |
| `extraEnv` | `[]` | 当前代码未注入 OCI env |

## 六、当前文档残留差异

后续进入 `P1` 之前，应继续把下面这些残留描述统一掉：

- 部分文档仍保留旧 env 名 `ILUVATAR_VISIBLE_DEVICES`
- 部分文档仍把 UUID 解析、`ldconfig`、RuntimeClass、自动打标写成“待完成”
- 部分链路说明仍把当前实现描述为“bind mount 整个目录”，而代码已经是“共享库细分处理 + linker 刷新”

因此，后续 profile 设计应以代码与本基线文档为准，不再以旧联调记录中的早期描述为准。

## 七、对下一阶段的直接输入

这份基线文档已经给出 `P1` 所需的最小输入：

- 哪些值是项目级默认值，哪些值是 Iluvatar 厂商事实
- 哪些当前逻辑必须抽成 profile 字段
- 哪些逻辑仍属于通用引擎代码，不适合直接挪进 YAML

下一步可以直接开始：

1. 设计首版 YAML profile schema
2. 定义 profile 加载与校验契约
3. 写出 Iluvatar 首版 profile 样例
