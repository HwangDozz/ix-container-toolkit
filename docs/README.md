# docs 索引

本目录收集了 `ix-container-toolkit` 的背景说明、状态记录、真实环境验证和联调报告。

如果你是第一次接手这个项目，建议按下面顺序阅读。

## 推荐阅读顺序

1. [项目入口说明](/home/huangsy/project/ix-container-toolkit/README.md)  
   仓库级入口，快速了解项目目标、三段式架构和当前边界。

2. [项目状态文档](/home/huangsy/project/ix-container-toolkit/docs/project-status.md)  
   汇总当前实现范围、组件职责、工作流程和已知风险。

3. [全链路机制说明](/home/huangsy/project/ix-container-toolkit/docs/end-to-end-runtime-hook-chain.md)  
   解释 `RuntimeClass -> containerd -> ix-container-runtime -> ix-container-hook -> 容器 rootfs` 是如何逐层生效的，以及每一步的运行时凭据。

4. [RuntimeClass 与 Hook 联调验证报告](/home/huangsy/project/ix-container-toolkit/docs/runtime-hook-validation.md)  
   记录本次真实联调过程、遇到的问题、修复的 bug 和最终验证结果。

5. [物理节点验证报告](/home/huangsy/project/ix-container-toolkit/docs/physical-node-validation.md)  
   记录宿主机 `/dev/iluvatar*`、`/usr/local/corex`、`ixsmi` 等真实环境信息，用于校正代码假设。

6. [真实 Pod 分析](/home/huangsy/project/ix-container-toolkit/docs/pod-analysis.md)  
   记录对线上真实 GPU Pod 的 YAML 和 Device Plugin 行为分析，重点解释为什么要兼容 `ILUVATAR_COREX_VISIBLE_DEVICES` 和 UUID。

## 按用途查找

### 背景与状态

- [项目入口说明](/home/huangsy/project/ix-container-toolkit/README.md)
- [项目状态文档](/home/huangsy/project/ix-container-toolkit/docs/project-status.md)

### 真实环境依据

- [物理节点验证报告](/home/huangsy/project/ix-container-toolkit/docs/physical-node-validation.md)
- [真实 Pod 分析](/home/huangsy/project/ix-container-toolkit/docs/pod-analysis.md)

### 机制与实现

- [全链路机制说明](/home/huangsy/project/ix-container-toolkit/docs/end-to-end-runtime-hook-chain.md)

### 调试与验证记录

- [RuntimeClass 与 Hook 联调验证报告](/home/huangsy/project/ix-container-toolkit/docs/runtime-hook-validation.md)

## 维护建议

后续如果继续新增文档，建议按以下原则放置：

- 背景和长期事实放在 `README.md`、`CLAUDE.md` 或 `project-status.md`
- 一次性的排障过程单独落到 `docs/`，不要混进状态文档
- 真实节点观测结果优先单独成文，避免和推断性结论写在一起
