# ix-container-toolkit 通用化阶段执行跟踪

> 更新日期：2026-04-03
> 作用：跟踪剩余工作的执行状态、阻塞项和最新结论。

## 一、阶段状态

| 项目 | 状态 | 备注 |
|---|---|---|
| 范围说明 | 已调整 | 当前优先完成 `910B`，`310P` 暂缓 |
| `P0` 事实基线 | 完成 | 见 `docs/generic-framework-p0-baseline.md` |
| `P1` schema 与 Iluvatar 样例 | 基本完成 | loader / render / profile 已落地 |
| `P2-A` 310P factsheet | 暂缓 | 按当前范围先不阻塞 910B 收口 |
| `P2-B` 910B factsheet | 完成 | factsheet、验证记录与关键节点事实已收口 |
| `P2-C` Ascend profile / diff matrix | 910B 完成 | `ascend-910b.yaml` 已正式化，310P 暂缓 |
| `P3-A` 设备解析通用化 | 进行中 | 已参数化，仍有 Iluvatar 假设 |
| `P3-B` hook artifact 主路径 | 进行中 | legacy 路径仍存在 |
| `P3-C` 去兼容桥 | 进行中 | `pkg/config` 仍在主链路中 |
| `V1` installer 自动化测试 | 基本完成 | 已覆盖 `copyBinaries`、`writeConfig`、`patchContainerd` 幂等与 `labelNode` mock |
| `V2` profile 驱动端到端验证 | 910B 基本完成 | 已有事实验证、loader/render 验证与节点级路径校验；仍缺完整自部署 runtime/hook 闭环记录 |
| `D1` 文档状态统一收口 | 进行中 | 新增了剩余工作与任务拆解文档 |

## 二、当前阻塞

- 缺少第二厂商样本来验证 schema 充分性
- profile 执行链仍依赖 legacy `config.json` 兼容桥
- 910B 已完成当前范围收口；剩余问题转入通用执行链收口

## 三、下一步优先项

- [ ] 收口 `pkg/device` 中剩余的 Iluvatar 假设
- [ ] 收口 `internal/hook` 中的 legacy 注入路径
- [ ] 继续收缩 `pkg/config` 兼容桥
- [ ] 补一份“使用 `ascend-910b` profile 的完整自部署验证记录”

## 四、更新规则

后续每次推进时，建议同步更新：

- 本文档中的阶段状态表
- 对应 factsheet / profile / diff matrix 文档
- `docs/README.md` 的入口索引
