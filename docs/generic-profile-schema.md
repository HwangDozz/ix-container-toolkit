# ix-container-toolkit 首版 Generic Profile Schema

> 更新日期：2026-04-07
> 状态：已落地，本文包含当前实现约束
> 目标：定义首版 YAML profile 的最小结构、加载约束和校验边界，为后续 runtime/hook/installer 通用化改造提供稳定输入。

## 一、设计目标

首版 schema 只解决一件事：

- 用单个 YAML 文件完整表达“一个厂商/产品线”的运行时注入事实

首版不解决这些问题：

- profile 继承
- 动态脚本化
- 多 profile 自动发现
- 一次性建模所有厂商差异

## 二、文件组织

约定：

- profile 文件存放在仓库 `profiles/` 目录
- 每个厂商/产品线一个文件
- 文件由显式路径加载，不做自动发现

示例：

- `profiles/iluvatar-bi-v150.yaml`

## 三、顶层结构

首版 profile 使用以下 5 个顶层分组：

- `metadata`
- `runtime`
- `kubernetes`
- `device`
- `inject`

示例骨架：

```yaml
metadata: {}
runtime: {}
kubernetes: {}
device: {}
inject: {}
```

## 四、字段说明

### 4.1 `metadata`

用途：

- 提供 profile 的识别信息

字段：

- `name`
- `vendor`
- `modelFamily`
- `version`

约束：

- `name` 必填，作为 profile 的稳定标识
- `vendor` 必填
- `version` 必填，表示 schema/profile 自身版本，不是驱动版本

### 4.2 `runtime`

用途：

- 描述 runtime shim 的节点侧执行信息

字段：

- `underlyingRuntime`
- `hookStage`
- `hookBinary`
- `injectMode`

约束：

- `underlyingRuntime` 必填
- `hookStage` 首版只接受 `prestart`
- `hookBinary` 必填
- `injectMode` 可为空；当前仅额外支持 `delegate-only`

当前实现补充约束：

- 集群侧统一只使用一个 `RuntimeClass` / handler：`xpu-runtime`
- profile 不再单独声明 `handlerName` 或 `runtimeClassName`
- `injectMode: delegate-only` 表示 `accelerator-container-runtime` 不修改 OCI spec，不注入本项目 hook/device/artifact，直接委托 `underlyingRuntime`

### 4.3 `kubernetes`

用途：

- 描述 Kubernetes 侧资源名、节点标签和 installer DaemonSet 的节点选择约束

字段：

- `resourceNames`
- `nodeLabels`
- `runtimeClassScheduling.nodeSelector`
- `runtimeClassScheduling.tolerations`

约束：

- `resourceNames` 至少一个
- `nodeLabels` 可为空
- `runtimeClassScheduling` 可为空，但 Iluvatar 现状样例中会显式填写

当前实现说明：

- `runtimeClassScheduling` 当前只用于渲染 installer DaemonSet
- 统一的 `RuntimeClass xpu-runtime` 不再携带 profile-specific scheduling

### 4.4 `device`

用途：

- 描述容器“请求了哪些设备”的识别方式，以及如何将 selector 解析成宿主机设备节点

字段：

- `selectorEnvVars`
- `selectorFormats`
- `deviceGlobs`
- `controlDeviceGlobs`
- `mapping.strategy.primary`
- `mapping.strategy.fallback`
- `mapping.command.pathCandidates`
- `mapping.command.args`
- `mapping.command.env`
- `mapping.parser`
- `fallbackPolicy`

约束：

- `selectorEnvVars` 至少一个
- `deviceGlobs` 至少一个
- `mapping.strategy.primary` 必填
- 若 primary strategy 是命令查询类策略，则 `mapping.command` 与 `mapping.parser` 必填

当前已验证的 selector / mapping 形态：

- `command` / `env`：用于 index、UUID 或厂商命令可解析的 selector
- `env-all`：用于只支持 `all` / `none` selector 的设备族，运行时按 profile 的 device globs 注入全部匹配设备

对 `env-all` profile，如果容器 OCI spec 中没有显式 selector env，runtime 会把第一个 `selectorEnvVars` 注入为 `all`，并以此触发 hook。

### 4.5 `inject`

用途：

- 描述如何把宿主机侧设备、库、工具和 linker 配置注入容器

字段：

- `containerRoot`
- `artifacts[]`
- `linker`
- `extraEnv`

其中 `artifacts[]` 为声明式 artifact 列表，每个 artifact 包含：

- `name`
- `kind`
- `hostPaths`
- `containerPath`
- `mode`
- `excludeDirs`
- `optional`

首版支持的 `kind`：

- `device-nodes`
- `shared-libraries`
- `directory`

首版支持的 `mode`：

- `bind`
- `copy`
- `so-only`

`linker` 包含：

- `strategy`
- `configPath`
- `paths`
- `runLdconfig`

约束：

- `containerRoot` 必填
- `artifacts` 至少一个
- 每个 artifact 的 `name`、`kind`、`hostPaths`、`containerPath`、`mode` 必填
- `linker.configPath` 必填

## 五、加载与校验契约

首版 profile 加载契约如下：

1. 由显式文件路径加载 YAML
2. 文件不存在时返回错误，不做默认 profile 回退
3. YAML 语法错误时返回错误
4. 未知字段时报错，避免 YAML key 拼写错误被静默忽略
5. 结构校验失败时返回错误
6. loader 只负责“可解析 + 基本合法”，不在这一层验证宿主机文件是否真实存在

首版校验范围：

- 必填字段完整性
- 枚举字段合法性
- 数组至少一个元素
- artifact 结构合法性

不在首版 loader 中校验：

- `/dev` 节点是否真实存在
- `ixsmi` 是否真实可执行
- `containerd`/`RuntimeClass` 是否已部署

## 六、Iluvatar 映射原则

首版 schema 应能无歧义承载当前 Iluvatar 事实：

- `resourceNames=["iluvatar.com/gpu"]`
- `nodeLabels={"iluvatar.ai/gpu":"present"}`
- `selectorEnvVars=["ILUVATAR_COREX_VISIBLE_DEVICES"]`
- `deviceGlobs=["/dev/iluvatar*"]`
- `mapping.command=*ixsmi*`
- `inject.artifacts` 覆盖设备、驱动库、驱动工具
- `linker` 明确声明 `ld.so.conf.d` 与 `ldconfig`

## 七、与后续代码改造的衔接

后续代码改造建议按下面方式消费 profile：

1. installer 读取 `runtime` 与 `kubernetes`，在节点侧注册统一 `xpu-runtime` handler，并处理节点标签
2. runtime 读取 `runtime.hookStage`、`device.selectorEnvVars` 决定何时注入 hook
3. hook 读取 `device` 与 `inject`，执行设备解析和 artifact 注入

因此，首版 schema 的作用不是替换所有逻辑，而是先把厂商事实从核心代码中剥离出来。

## 八、当前落地状态

截至当前仓库状态，下面这些 profile 消费路径已经落地：

- installer 可读取 profile 并生成兼容 `config.json`
- `pkg/config` 当前角色是旧 JSON 兼容层，不再是通用化后的主配置模型
- installer 会根据 profile 在节点侧注册统一 `xpu-runtime` handler，并处理节点标签行为
- `accelerator-profile-render runtimeclass --profile <path>` 可从 profile 渲染 RuntimeClass
- `accelerator-profile-render daemonset --profile <path> --image <image>` 可从 profile 渲染 installer DaemonSet
- `accelerator-profile-render bundle --profile <path> --image <image>` 可渲染整包部署清单：RBAC + RuntimeClass + DaemonSet
- 渲染得到的 `RuntimeClass` 名称固定为 `xpu-runtime`
- `make deploy` / `make undeploy` 已改为基于 profile 渲染 RuntimeClass，而不是依赖静态 `runtimeclass.yaml`
- `make deploy` / `make undeploy` 已改为基于 profile 渲染 DaemonSet，不再依赖静态 Iluvatar 节点标签与 driver 路径 env
- `make deploy` / `make undeploy` 现在基于 profile 整包渲染并 apply/delete
- runtime / hook 已支持消费 `device.selectorEnvVars`
- `pkg/device` 已支持消费 `device.deviceGlobs`、`mapping.command`、`mapping.env`、`mapping.parser`
- hook 已支持消费 `inject.artifacts` 与 `inject.linker` 的首版执行路径
- runtime 已支持消费 `inject.extraEnv`，并在不覆盖已有 env 的前提下写入 OCI spec
