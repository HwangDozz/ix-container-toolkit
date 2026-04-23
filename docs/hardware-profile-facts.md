# 硬件 Profile 事实

> 更新日期：2026-04-23

## 当前样本

| 硬件 | Profile | 状态 |
|---|---|---|
| Iluvatar BI-V150 | `profiles/iluvatar-bi-v150.yaml` | 早期验证样本，作为通用化起点 |
| Ascend 910B | `profiles/ascend-910b.yaml` | 已完成 runtime/backend L3 smoke 验证 |
| Ascend 310P | `profiles/ascend-310p.yaml` | 暂缓，不阻塞当前 910B 闭环 |

## 关键差异

| 维度 | Iluvatar BI-V150 | Ascend 910B |
|---|---|---|
| Kubernetes resource | `iluvatar.ai/gpu` | `huawei.com/Ascend910` |
| selector env | `ILUVATAR_COREX_VISIBLE_DEVICES` | `ASCEND_VISIBLE_DEVICES` |
| selector 格式 | index / UUID | index-list |
| 设备节点 | `/dev/iluvatar*` | `/dev/davinci*` |
| 控制设备 | 无单独样本 | `/dev/davinci_manager`, `/dev/devmm_svm`, `/dev/hisi_hdc` |
| 驱动根目录 | `/usr/local/corex` | `/usr/local/Ascend` |
| 映射策略 | command/env + parser | env index-list |

## Ascend 910B 节点事实

已验证节点：

```text
kunlun-02
```

关键事实：

- 架构：`arm64`
- OS：`openEuler 22.03 (LTS-SP4)`
- Kernel：`5.10.0-303.0.0.206.oe2203sp4.aarch64`
- containerd：`1.7.28`
- 节点标签：`accelerator=huawei-Ascend910`
- 芯片标签：`node.kubernetes.io/npu.chip.name=910B3`
- 资源名：`huawei.com/Ascend910`
- Capacity：`8`
- Allocatable：`7`
- CANN：`/usr/local/Ascend/cann-8.5.1`
- toolkit 入口：`/usr/local/Ascend/ascend-toolkit/latest`

## Ascend 910B Profile 要点

`profiles/ascend-910b.yaml` 当前覆盖：

- `/dev/davinci*`
- `/dev/davinci_manager`
- `/dev/devmm_svm`
- `/dev/hisi_hdc`
- Ascend driver libraries
- CANN/toolkit runtime libraries
- CANN toolkit bin / ccec compiler bin
- `ASCEND_*`、`LD_LIBRARY_PATH`、`PYTHONPATH`、`PATH`

对于基于官方 CANN 镜像的 backend，CANN 用户态由镜像提供；profile 中相关路径仍可作为兼容补充，后续可以进一步裁剪。
