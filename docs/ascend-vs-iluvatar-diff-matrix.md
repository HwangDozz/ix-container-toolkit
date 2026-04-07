# Iluvatar vs Ascend 差异矩阵

> 状态：910B 部分完成，310P 暂缓
> 更新日期：2026-04-03
> 目的：对比 Iluvatar、Ascend 310P、Ascend 910B 的运行时注入差异，验证当前 schema 是否足够。

| 维度 | Iluvatar BI-V150 | Ascend 310P | Ascend 910B | 备注 |
|---|---|---|---|---|
| 资源名 | `iluvatar.ai/gpu` | 待补 | `huawei.com/Ascend910` | 910B 样本已从 workload Pod 和 device plugin 确认 |
| selector env | `ILUVATAR_COREX_VISIBLE_DEVICES` | 待补 | `ASCEND_VISIBLE_DEVICES` | |
| selector 形式 | `all/none/index-list/uuid-list` | 待补 | `index-list` | 当前样本值为 `0,3` |
| 设备节点 glob | `/dev/iluvatar*` | 待补 | `/dev/davinci*` | |
| 控制设备 glob | `[]` | 待补 | `/dev/davinci_manager`, `/dev/devmm_svm`, `/dev/hisi_hdc` | 910B 明确存在控制节点，hook 需额外注入 |
| 映射策略 | `command-csv-index-uuid` | 待补 | `env-index-list` | 910B 当前样本不需要 UUID->index 映射 |
| 查询命令 | `ixsmi --query-gpu=index,uuid --format=csv` | 待补 | `npu-smi info` | 输出为表格文本，可用于诊断，但当前样本主链路不依赖它做映射 |
| 查询 env | `LD_LIBRARY_PATH=...corex...` | 待补 | `ASCEND_* + LD_LIBRARY_PATH + PYTHONPATH + PATH` | 910B env 复杂度明显更高 |
| parser | `csv-header-index-uuid` | 待补 | 当前主链路可不需要；若保留诊断命令，则需要新增 `npu-smi` 表格 parser | |
| 容器驱动根目录 | `/usr/local/corex` | 待补 | `/usr/local/Ascend` | |
| 共享库注入 | `so-only` | 待补 | `driver/lib64/common + driver/lib64/driver + nnal/atb/.../lib + ascend-toolkit/latest/lib64 + tools/aml/lib64` | 910B 是多目录组合，不是单根目录 |
| 工具目录注入 | `/usr/local/corex/bin` | 待补 | `driver/tools + ascend-toolkit/latest/bin + tools/ccec_compiler/bin` | 当前样本至少确认这些路径 |
| linker 策略 | `ldconfig` | 待补 | `ldconfig` | 当前 schema 足够表达 910B 的 linker 基线 |
| extraEnv | `{}` | 待补 | 需要重点注入 | 当前样本实际依赖 `ASCEND_*`、`PATH`、`LD_LIBRARY_PATH`、`PYTHONPATH` |
| RuntimeClass / handler | `xpu-runtime` / `xpu-runtime` | `xpu-runtime` / `xpu-runtime` | `xpu-runtime` / `xpu-runtime` | 运行时入口已统一，差异只保留在 profile 与资源调度层 |
| 节点标签 | `iluvatar.ai/gpu=present` | 待补 | `accelerator=huawei-Ascend910` | 当前样本来自 device plugin DaemonSet 的调度条件 |

## 结论

当前判断：

- 当前 schema 已足够表达 910B 的一条可执行主链路：
  - selector env
  - 主设备节点
  - 控制设备节点
  - 多目录 library / binary artifact
  - linker 配置
  - 额外 env 注入
- 当前主要缺口不在 schema，而在执行链：
  - `controlDeviceGlobs` 需要真实消费
  - `pkg/device` 不能继续假设只有 `GPU-` 风格标识
  - 若后续要用 `npu-smi info` 做映射，则还需新增表格 parser
- 暂时不需要为 910B 再加新字段：
  - 当前 profile 已可采用 `env-index-list`
  - 映射命令可留作诊断，而不是主执行依赖
