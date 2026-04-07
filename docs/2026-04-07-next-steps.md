# 2026-04-07 下一步工作说明

> 目的：基于当前仓库状态，收敛项目接下来应该优先推进的工作，确保后续动作继续围绕“profile 驱动的通用加速卡容器注入框架”这个总目标展开。

## 一、当前阶段判断

结合现有代码、文档和最近提交记录，当前项目已经完成了首轮骨架收口：

- `runtime / hook / installer` 主链已经切到 active profile 驱动
- `ascend-910b` 已具备可加载、可渲染、可进入主执行链的样本 profile
- 多架构构建和分发入口已经具备
- 历史 `ix / iluvatar` 默认值已明显收缩，不再是主执行路径

当前更大的缺口已经不是“继续补一层抽象”，而是下面三类事情：

- 用真实部署闭环证明第二厂商 profile 能跑通
- 继续收口仍然残留的兼容桥和 legacy 执行路径
- 把文档、状态表述和实际实现统一到同一口径

## 二、接下来优先要做的事

### 1. 优先完成 `ascend-910b` 自部署闭环验证

这是当前最应该先做的事，因为它最直接决定“通用框架是否真的成立”。

需要完成：

- 用 `profiles/ascend-910b.yaml` 渲染统一 `RuntimeClass xpu-runtime` 和对应 DaemonSet
- 在真实 `910B` 节点重新安装当前 `accelerator-*` 产物
- 验证 installer 是否正确落盘 active profile、二进制和 containerd handler
- 验证 runtime 是否成功注入 hook
- 验证 hook 是否成功注入设备、artifact 和 linker 配置
- 验证最小业务容器是否能实际使用驱动与设备

完成判定：

- 宿主机存在 `/etc/accelerator-toolkit/profiles/active.yaml`
- `RuntimeClass xpu-runtime` 能被节点正确识别，且业务 Pod 可在目标节点上成功走 toolkit 链路
- 容器内能看到 profile 声明的设备和驱动路径
- 最小 workload 可运行，并形成一份新的验证记录文档

### 2. 收口执行链里剩余的 legacy 兼容桥

当前主链虽然已经由 profile 驱动，但仍有几处需要继续收缩。

优先关注：

- `pkg/config` 是否还能继续弱化，只保留进程级覆盖项
- `pkg/device` 中剩余的 selector / identifier 假设是否已经足够通用
- `internal/hook` 中 legacy 注入路径是否还能继续并到 artifact 主路径

完成判定：

- 新增厂商样本时，不需要继续往 runtime / hook / installer 主链里塞厂商常量
- profile 事实不再回流到 `config.json`
- 设备解析和注入路径的主实现只有一套

### 3. 统一 README 和关键状态文档口径

这项工作不影响运行，但会直接影响后续维护效率。

需要收口：

- 旧项目名、旧 `ix-*` 命名
- 旧 `/etc/ix-toolkit`、旧 `IX_*` 环境变量
- 旧 handler / RuntimeClass / 静态 YAML 入口描述
- 哪些文档是历史联调记录，哪些文档是当前事实基线

建议优先更新：

- `README.md`
- `docs/README.md`
- `docs/project-status.md`
- `docs/generic-framework-execution-tracker.md`

完成判定：

- 新接手的人从仓库入口进入，不会再被旧命名和旧流程误导

### 4. 后置补齐 `310P` 样本

这项工作可以排在上面三项之后。

它的意义不是再加一类设备支持，而是进一步验证当前 schema 和执行链是否足够稳。

需要完成：

- `310P factsheet`
- `310P` 正式 profile
- 与 `Iluvatar / 910B` 的差异确认
- 至少一轮 loader / render / 执行链验证

完成判定：

- 当前 schema 能稳定承载第三类样本，而不需要新增明显的厂商分支

## 三、建议推进顺序

建议按下面顺序推进：

1. `910B` 自部署闭环验证
2. 收口 `pkg/config`、`pkg/device`、`internal/hook` 的 legacy 路径
3. 清理 README 和状态文档口径
4. 补 `310P` 样本与验证

这个顺序的原因很直接：

- 第一步验证框架是否真的在第二厂商闭环
- 第二步减少后续继续改动时的结构性返工
- 第三步保证文档与事实一致
- 第四步再继续扩大样本覆盖面

## 四、建议本周交付物

如果按“短周期收口”来推进，建议本周至少产出：

- 一份 `ascend-910b` 当前命名体系下的真实部署验证记录
- 一轮对 `pkg/config` / `pkg/device` / `internal/hook` 的收口结果
- 更新后的状态文档与索引入口

这样做完之后，项目就能更清楚地进入下一个阶段：

- 从“框架骨架已经建立”
- 进入“验证闭环、减少兼容层、扩展样本覆盖”
