# docs 索引

本目录只保留长期有效的设计、状态、事实和验证结果。日期型工作总结、临时 handoff、调试过程和任务拆解已经合并到下面的稳定文档中。

## 推荐阅读

1. [项目状态](./project-status.md)  
   当前目标、组件边界、已验证能力和剩余风险。

2. [运行时架构](./runtime-architecture.md)  
   `RuntimeClass -> containerd -> accelerator-container-runtime -> accelerator-container-hook` 的执行链路。

3. [Generic Profile Schema](./generic-profile-schema.md)  
   profile 的稳定结构、校验规则和字段语义。

4. [硬件 Profile 事实](./hardware-profile-facts.md)  
   Iluvatar 与 Ascend 910B 的资源名、selector env、设备节点和注入路径差异。

5. [PyTorch Backend 边界](./pytorch-backend-and-portability-summary.md)  
   backend 镜像、用户训练代码和 toolkit 的职责边界。

6. [Ascend 910B Backend](./ascend-910b-backend.md)  
   Ascend 910B CANN/PyTorch backend 的镜像构建方式和 smoke test 结果。

7. [Iluvatar BI-V150 Backend](./iluvatar-bi-v150-backend.md)  
   天数 BI-V150/CoreX PyTorch backend 的本机调研结论和后续验证计划。

8. [可迁移训练代码规范](./portable-training-guidelines.md)  
   训练代码在 CUDA、NPU 和 CPU/debug 后端之间迁移时需要遵守的约束。

9. [Backend 镜像制作注意事项](./backend-image-guidelines.md)  
   后续迁移到其他加速卡时制作 PyTorch backend 镜像的边界和验证要求。

10. [验证结果](./validation-results.md)  
   已完成的节点级、runtime/hook 和 Ascend 910B smoke 验证结论。

## 文档维护原则

- 长期事实进入稳定文档，不再新增日期型过程文档。
- 排障过程只保留最终原因、修复点和验证结果。
- 新硬件接入优先更新 `hardware-profile-facts.md` 和 `profiles/*.yaml`。
- 新 backend 验证优先更新 `ascend-910b-backend.md` 或新增对应硬件 backend 文档。
