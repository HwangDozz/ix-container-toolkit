# docs 索引

本目录收集了 `accelerator-container-toolkit` 的背景说明、状态记录、真实环境验证和联调报告。

如果你是第一次接手这个项目，建议按下面顺序阅读。

## 推荐阅读顺序

1. [项目入口说明](/home/huangsy/project/ix-container-toolkit/README.md)  
   仓库级入口，快速了解项目目标、三段式架构和当前边界。

2. [项目状态文档](/home/huangsy/project/ix-container-toolkit/docs/project-status.md)  
   汇总当前实现范围、组件职责、工作流程和已知风险。

3. [2026-04-08 项目介绍（组会版）](/home/huangsy/project/ix-container-toolkit/docs/2026-04-08-project-introduction.md)  
   面向组会汇报整理项目目标、整体设计、工作流程和项目效果，适合快速对外介绍。

4. [2026-04-03 工作总结](/home/huangsy/project/ix-container-toolkit/docs/2026-04-03-work-summary.md)  
   汇总今天这轮大重构的完成项、当前阶段判断和下一步优先级，适合作为最新入口。

5. [2026-04-07 下一步工作说明](/home/huangsy/project/ix-container-toolkit/docs/2026-04-07-next-steps.md)  
   面向执行的最新入口，聚焦整体目标下接下来最应该推进的事项、顺序和完成判定。

6. [2026-04-07 集群测试总结](/home/huangsy/project/ix-container-toolkit/docs/2026-04-07-cluster-test-summary.md)  
   记录今天围绕统一 `xpu-runtime`、BuildKit 修复、多架构镜像构建和 Ascend 910B 集群验证的实际结果，可直接作为明天继续测试的上下文。

7. [通用化改造 P0 基线](/home/huangsy/project/ix-container-toolkit/docs/generic-framework-p0-baseline.md)  
   收敛当前代码真实行为、Iluvatar 硬编码项和首版 profile 字段映射，是后续 schema 设计的直接输入。

8. [首版 Generic Profile Schema](/home/huangsy/project/ix-container-toolkit/docs/generic-profile-schema.md)  
   定义 `metadata/runtime/kubernetes/device/inject` 五段式 YAML 结构、加载约束和校验边界。

9. [通用化改造计划](/home/huangsy/project/ix-container-toolkit/docs/generic-framework-plan.md)  
   描述项目从 Iluvatar 专用实现演进为 YAML profile 驱动通用框架的阶段目标、优先级和后续调研计划。

10. [通用化阶段剩余工作清单](/home/huangsy/project/ix-container-toolkit/docs/generic-framework-remaining-work.md)  
   汇总当前距离“首轮通用化目标完成”还缺哪些工作，重点标出 `P2`、`P3`、验证和文档收口缺口。

11. [通用化阶段执行任务拆解](/home/huangsy/project/ix-container-toolkit/docs/generic-framework-task-breakdown.md)  
   把剩余工作进一步拆成可执行任务、issue 粒度和里程碑建议，便于直接推进。

12. [通用化阶段执行跟踪](/home/huangsy/project/ix-container-toolkit/docs/generic-framework-execution-tracker.md)  
   跟踪 `P2`、`P3`、验证与文档收口的执行状态、阻塞项和下一步优先级。

13. [全链路机制说明](/home/huangsy/project/ix-container-toolkit/docs/end-to-end-runtime-hook-chain.md)  
   解释 `RuntimeClass -> containerd -> accelerator-container-runtime -> accelerator-container-hook -> 容器 rootfs` 是如何逐层生效的，以及每一步的运行时凭据。

14. [RuntimeClass 与 Hook 联调验证报告](/home/huangsy/project/ix-container-toolkit/docs/runtime-hook-validation.md)  
   记录本次真实联调过程、遇到的问题、修复的 bug 和最终验证结果。

15. [物理节点验证报告](/home/huangsy/project/ix-container-toolkit/docs/physical-node-validation.md)  
   记录宿主机 `/dev/iluvatar*`、`/usr/local/corex`、`ixsmi` 等真实环境信息，用于校正代码假设。

16. [真实 Pod 分析](/home/huangsy/project/ix-container-toolkit/docs/pod-analysis.md)  
   记录对线上真实 GPU Pod 的 YAML 和 Device Plugin 行为分析，重点解释为什么要兼容 `ILUVATAR_COREX_VISIBLE_DEVICES` 和 UUID。

## 按用途查找

### 背景与状态

- [项目入口说明](/home/huangsy/project/ix-container-toolkit/README.md)
- [项目状态文档](/home/huangsy/project/ix-container-toolkit/docs/project-status.md)
- [2026-04-08 项目介绍（组会版）](/home/huangsy/project/ix-container-toolkit/docs/2026-04-08-project-introduction.md)
- [2026-04-03 工作总结](/home/huangsy/project/ix-container-toolkit/docs/2026-04-03-work-summary.md)
- [2026-04-07 下一步工作说明](/home/huangsy/project/ix-container-toolkit/docs/2026-04-07-next-steps.md)
- [2026-04-07 集群测试总结](/home/huangsy/project/ix-container-toolkit/docs/2026-04-07-cluster-test-summary.md)
- [通用化改造 P0 基线](/home/huangsy/project/ix-container-toolkit/docs/generic-framework-p0-baseline.md)
- [首版 Generic Profile Schema](/home/huangsy/project/ix-container-toolkit/docs/generic-profile-schema.md)
- [通用化改造计划](/home/huangsy/project/ix-container-toolkit/docs/generic-framework-plan.md)
- [通用化阶段剩余工作清单](/home/huangsy/project/ix-container-toolkit/docs/generic-framework-remaining-work.md)
- [通用化阶段执行任务拆解](/home/huangsy/project/ix-container-toolkit/docs/generic-framework-task-breakdown.md)
- [通用化阶段执行跟踪](/home/huangsy/project/ix-container-toolkit/docs/generic-framework-execution-tracker.md)

### 真实环境依据

- [物理节点验证报告](/home/huangsy/project/ix-container-toolkit/docs/physical-node-validation.md)
- [真实 Pod 分析](/home/huangsy/project/ix-container-toolkit/docs/pod-analysis.md)
- [Ascend 310P Factsheet](/home/huangsy/project/ix-container-toolkit/docs/ascend-310p-factsheet.md)
- [Ascend 910B Factsheet](/home/huangsy/project/ix-container-toolkit/docs/ascend-910b-factsheet.md)
- [Ascend 910B Validation Record](/home/huangsy/project/ix-container-toolkit/docs/ascend-910b-validation.md)
- [Iluvatar vs Ascend 差异矩阵](/home/huangsy/project/ix-container-toolkit/docs/ascend-vs-iluvatar-diff-matrix.md)

### 机制与实现

- [全链路机制说明](/home/huangsy/project/ix-container-toolkit/docs/end-to-end-runtime-hook-chain.md)

### Profile 驱动部署

- [首版 Generic Profile Schema](/home/huangsy/project/ix-container-toolkit/docs/generic-profile-schema.md)
- `profiles/iluvatar-bi-v150.yaml`
- `profiles/ascend-310p.yaml`（模板，待填）
- `profiles/ascend-910b.yaml`（已完成）
- `make render-runtimeclass`
- `make render-daemonset`
- `make deploy`

### 调试与验证记录

- [RuntimeClass 与 Hook 联调验证报告](/home/huangsy/project/ix-container-toolkit/docs/runtime-hook-validation.md)

## 维护建议

后续如果继续新增文档，建议按以下原则放置：

- 背景和长期事实放在 `README.md`、`CLAUDE.md` 或 `project-status.md`
- 一次性的排障过程单独落到 `docs/`，不要混进状态文档
- 真实节点观测结果优先单独成文，避免和推断性结论写在一起
