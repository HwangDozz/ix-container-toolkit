# ix-container-toolkit 通用化阶段剩余工作清单

> 更新日期：2026-04-03
> 依据：`docs/generic-framework-plan.md`、`docs/generic-profile-schema.md`、`docs/generic-framework-p0-baseline.md` 与当前代码实现状态
> 目的：收敛当前“首轮通用化”还未完成的工作，明确哪些属于 `P2`，哪些属于 `P3`，以及还缺哪些验证与文档收口动作。

## 一、阶段结论

当前项目状态可以概括为：

- `P0` 基本完成：现状纠偏、硬编码项清单和 Iluvatar 字段映射已经形成文档基线
- `P1` 基本完成：首版 profile schema、loader、校验、Iluvatar 样例 profile 和 profile 渲染入口已经落地
- `P2` 已完成 `910B` 子目标：`910B` 的事实采集、样本 profile 和验证记录已经形成正式输出；`310P` 当前暂缓
- `P3` 只完成了一部分：runtime / hook / installer 已能消费 profile，但执行链内部仍保留较重的 Iluvatar 兼容层与 legacy 路径

因此，距离“本阶段目标”真正完成，还差的核心不是再设计一版 schema，而是：

- 用第二类厂商样本验证当前 schema 是否足够
- 把当前执行链从“profile 输入 + legacy config 兼容桥”推进到“profile 主模型驱动”
- 补齐验证、测试和文档收口

## 二、`P2` 还未完成的工作

`P2` 的目标是：获取真实 Ascend 运行时事实，验证当前 profile 抽象是否足够。

目前还缺：

- `Ascend 310P factsheet`
- `310P` 候选 profile 草案

当前已完成：

- `Ascend 910B factsheet`
- `910B` 正式 profile
- `910B` 验证记录
- `Iluvatar vs Ascend` 差异矩阵中的 910B 部分

按 `docs/generic-framework-plan.md` 约定，至少还需要采集和沉淀以下事实：

- `/dev` 设备节点命名、控制节点、管理节点形态
- Device Plugin 注入的 env、resource name、annotation
- 驱动目录与版本化软链关系
- 厂商查询工具、命令参数、依赖环境
- 容器内最小可用性验证方式
- 是否需要额外 env 注入
- 是否需要特殊 linker 处理

按当前范围，`910B` 已经证明当前 schema 可以无歧义承载第二个厂商样本。若要完成“更强的跨 Ascend 型号验证”，仍需后续补齐 `310P`。

## 三、`P3` 还未完成的工作

`P3` 的目标是：把当前项目改造成“通用引擎 + YAML profile”，同时保持 Iluvatar 零回归。

当前已经完成的部分：

- installer / runtime / hook 已支持读取 profile
- `pkg/device` 已支持读取 `deviceGlobs`、mapping command、mapping env、mapping parser
- hook 已支持首版 artifact 与 linker 执行路径
- runtime 已支持基于 profile 注入 `extraEnv`

但距离 `P3` 完成，还差下面这些工作。

### 3.1 去掉执行链中的重兼容桥

当前 `pkg/config` 仍是运行链中的重要桥接层，而不是纯遗留兼容层。

还需要做：

- 继续弱化 `profile -> config.json compatibility bridge`
- 明确哪些字段仍保留在项目级默认值中，哪些应彻底由 profile 驱动
- 让 runtime / hook / installer 的核心执行路径尽量直接消费 profile 视图，而不是先降级成 Iluvatar 风格 `Config`

当前表现：

- `pkg/config` 仍保留大量 Iluvatar 默认值
- `runtimeview` 仍有 `ix`、`iluvatar.ai/gpu=present` 等内置 fallback
- 未指定 profile 时仍主要依赖旧兼容模式

### 3.2 把设备解析真正做成通用流水线

当前设备解析已经参数化，但仍然带有明显 Iluvatar 假设。

还需要做：

- 把 selector 解析、index / UUID 判定规则继续抽象
- 让 `fallbackPolicy` 不只是 schema 字段，而是实际驱动执行路径
- 继续扩展 `mapping.parser`，避免只支持单一 CSV 解析格式
- 明确并实现 `controlDeviceGlobs` 的实际消费逻辑
- 评估是否需要支持非 `GPU-` 前缀的 UUID / identifier 模式

当前限制：

- `isUUID()` 仍硬编码 `GPU-` 前缀
- fallback 逻辑仍偏 Iluvatar 语义
- parser 目前实际上只有一类

### 3.3 把 artifact 驱动路径收敛为主实现

当前 hook 已支持 artifact 驱动，但 legacy 注入逻辑仍在并列存在。

还需要做：

- 让 artifact 路径成为默认且唯一的主实现
- 评估 legacy `injectDevices / injectDriverLibraries / injectDriverBinaries` 是否还能继续收缩
- 明确设备、共享库、目录、linker 四类 artifact 的统一语义边界
- 如后续出现 Ascend 特殊路径，再验证当前 artifact 模型是否需要补字段，而不是回退到硬编码分支

### 3.4 明确 runtime 的通用触发模型

当前 runtime 的注入判定仍然只依赖 selector env 命中。

还需要做：

- 明确是否还需要支持 annotation / resource / CDI 等额外触发条件
- 判断 `hookStage` 是否真的只保留 `prestart`，还是需要把它落实为可扩展执行点
- 验证不同厂商 Device Plugin 注入方式是否都能被“env 命中”覆盖

### 3.5 继续收敛 installer / 渲染器中的项目级硬编码

当前 profile 已不再单独决定 runtime handler / RuntimeClass 名称，二者统一为 `xpu-runtime`；但渲染器和 installer 里仍有不少项目级常量。

还需要做：

- 明确哪些常量属于项目级默认，不应进 profile
- 明确哪些内容应该继续参数化
- 至少重新审视下面这些项：
  - DaemonSet / ServiceAccount / ClusterRole 命名
  - namespace
  - pause 镜像
  - host mount 路径
  - installer 默认 env

这里不一定要求“全部进 profile”，但必须形成明确边界，避免后续继续混入厂商语义。

## 四、验证与测试还未完成的工作

除了 `P2` / `P3` 本身，当前还缺少足够的验证闭环。

### 4.1 installer 自动化测试

还缺：

- `copyBinaries` 测试
- `writeConfig` 测试
- `patchContainerd` 测试
- 节点打标逻辑的可控测试

当前这部分主要仍依赖手工节点验证。

### 4.2 非 Iluvatar profile 的执行验证

还缺：

- 至少一类非 Iluvatar profile 的 loader / render / runtime / hook 单元测试
- profile 驱动路径的端到端回归验证
- “不同 profile 不改核心代码即可接入”的实证验证

### 4.3 profile 驱动部署链路回归

还缺：

- `profile -> render bundle -> installer -> runtime -> hook` 的端到端验证
- profile 变更后对宿主机 `/etc/accelerator-toolkit/profiles/active.yaml` 与 `config.json` 的一致性验证
- 多架构节点场景下 installer 镜像分发与宿主机二进制安装验证

## 五、文档还未完成的工作

当前文档已经有较多最新内容，但仍未完全收口。

还需要做：

- 把 `docs/generic-profile-schema.md` 的状态从“`P1` 草案”更新为更准确的落地状态
- 继续清理 `docs/project-status.md` 中残留的旧 env 名、旧流程描述和历史口径
- 明确哪些文档属于历史联调记录，哪些文档是当前事实基线
- 在后续补齐 `P2` 输出后，把 factsheet / profile 草案 / 差异矩阵补进 `docs/README.md`

## 六、建议的收尾顺序

如果要以最小返工完成当前阶段，建议顺序如下：

1. 先做 `P2`
- 优先拿到 `310P` / `910B` 事实与样本 profile
- 用第二个厂商样本验证 schema 是否够用

2. 再补 `P3` 收口
- 收缩 `pkg/config` 兼容桥
- 完成设备解析通用流水线
- 让 artifact 驱动成为主执行路径

3. 最后补验证与文档
- installer 自动化测试
- profile 驱动端到端验证
- 更新阶段状态文档与 docs 索引

## 七、完成判定

可将“本阶段目标完成”定义为同时满足以下条件：

- 至少有 `Iluvatar + 一类 Ascend` 两套样本事实能够无歧义映射到当前 schema
- 新厂商样本接入不需要再往 runtime / hook / installer 核心路径里增加厂商常量
- runtime / hook / installer 的主执行路径已经由 profile 视图驱动，而不是主要依赖 legacy `config.json` 兼容桥
- profile 驱动部署链路具备自动化测试或稳定的端到端验证记录
- 项目文档对 `P0` / `P1` / `P2` / `P3` 的状态表述一致，不再混杂历史口径
