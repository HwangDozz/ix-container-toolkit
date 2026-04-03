# Ascend 910B Validation Record

> 状态：完成
> 更新日期：2026-04-03
> 目的：记录 `profiles/ascend-910b.yaml` 的事实来源、节点级验证结论和当前边界，作为 910B profile 收口依据。

## 一、验证范围

本轮只验证 `Ascend 910B`，不等待 `310P`。

验证目标：

- 确认 910B 的设备节点、selector env、resource name 和节点标签
- 确认 910B profile 中声明的关键 library / binary / env 路径在真实节点或真实 workload Pod 中可见
- 确认当前 schema 足以表达 910B 主链路，不需要再为 910B 单独扩字段

## 二、验证样本

宿主机：

- 节点：`kunlun-02`
- 架构：`aarch64`
- OS：`openEuler 22.03 (LTS-SP4)`
- Kernel：`5.10.0-303.0.0.206.oe2203sp4.aarch64`

workload Pod：

- 命名空间：`crater-workspace`
- Pod：`jpt-zhouxiao25-260327-56bd4-default0-0`
- 节点：`kunlun-02`

device plugin：

- 命名空间：`kube-system`
- Pod：`ascend-device-plugin-daemonset-tct2w`
- 镜像：`ascend-k8sdeviceplugin:v7.3.0`

## 三、已确认事实

### 3.1 Kubernetes / Device Plugin

- 资源名：`huawei.com/Ascend910`
- 节点标签：`accelerator=huawei-Ascend910`
- toleration key：`huawei.com/Ascend910`
- workload Pod annotation：
  - `huawei.com/Ascend910: Ascend910-0,Ascend910-3`
  - `huawei.com/AscendReal: Ascend910-0,Ascend910-3`
  - `huawei.com/kltDev: Ascend910-0,Ascend910-3`
- selector env：`ASCEND_VISIBLE_DEVICES=0,3`

结论：

- 910B 当前样本走的是 `env-index-list` 主链路
- 当前不需要依赖 UUID 映射命令才能完成设备注入

### 3.2 设备节点

在运行中的 workload Pod 内确认：

- 主设备节点：
  - `/dev/davinci0`
  - `/dev/davinci3`
- 控制 / 管理节点：
  - `/dev/davinci_manager`
  - `/dev/devmm_svm`
  - `/dev/hisi_hdc`

结论：

- `device.deviceGlobs` 可落为 `/dev/davinci*`
- `device.controlDeviceGlobs` 需要真实消费，不能只停留在 schema

### 3.3 Library 路径

宿主机和 workload Pod 已确认可见：

- `/usr/local/Ascend/driver/lib64/common`
- `/usr/local/Ascend/driver/lib64/driver`
- `/usr/local/Ascend/ascend-toolkit/latest/lib64`
- `/usr/local/Ascend/ascend-toolkit/latest/tools/aml/lib64`
- `/usr/local/Ascend/nnal/atb/latest/atb/cxx_abi_0/lib`

运行中的 workload Pod 内，`ldd /usr/local/bin/npu-smi` 直接依赖：

- `libc_sec.so`
- `libdrvdsmi_host.so`
- `libmmpa.so`
- `libascend_hal.so`

结论：

- 当前 910B profile 里的 `shared-libraries` 与 `linker.paths` 已覆盖主运行依赖的基线组合

### 3.4 Tool 路径

已确认：

- `npu-smi` 位于 `/usr/local/bin/npu-smi`
- 宿主机存在 `/usr/local/Ascend/driver/tools`
- 运行中的 workload Pod 内未看到 `/usr/local/Ascend/driver/tools/hccn_tool`

结论：

- `npu-smi` 是当前样本中的稳定诊断工具
- `driver/tools` 可以保留在 host-side artifact 中，但不能把 `hccn_tool` 视为 workload 容器内已知必需路径

### 3.5 环境变量

运行中的 workload Pod 内已确认：

- `ASCEND_VISIBLE_DEVICES`
- `ASCEND_HOME_PATH`
- `ASCEND_TOOLKIT_HOME`
- `ASCEND_OPP_PATH`
- `ASCEND_AICPU_PATH`
- `ASCEND_RUNTIME_OPTIONS`
- `LD_LIBRARY_PATH`
- `PATH`
- `PYTHONPATH`
- `TOOLCHAIN_HOME`
- `CRATER_ASCEND_ENV_INITIALIZED`
- `NPU_COMPUTING_FORECAST_HOME`

额外观察：

- workload env 中曾出现 `ATB` 的 `examples` / `tests` 路径
- 当前宿主机未确认这些目录存在

结论：

- `profiles/ascend-910b.yaml` 只保留了当前宿主机已确认存在的 host-side 路径
- 未把仅出现在 workload env、但宿主机未确认存在的目录当作注入前提

## 四、当前结论

### 4.1 910B profile 已完成收口

当前 [ascend-910b.yaml](/home/huangsy/project/ix-container-toolkit/profiles/ascend-910b.yaml) 已具备：

- 正式命名的 `handlerName` / `runtimeClassName`
- 真实 `resourceNames`
- 真实 `nodeLabels` / `nodeSelector` / `tolerations`
- 真实 `selectorEnvVars`
- 真实主设备和控制设备节点
- 基于节点事实收敛后的 library / binary artifact
- 基于节点事实收敛后的 `extraEnv`

### 4.2 当前 schema 对 910B 足够

对 910B 而言，当前 schema 已经能表达：

- index-list selector
- 多组 device / control device
- 多目录 library artifact
- 目录型 binary artifact
- linker 配置
- 额外 env 注入

本轮没有发现必须为 910B 新增 schema 字段的证据。

### 4.3 剩余的不是 910B 建模问题

当前剩余问题主要属于更通用的执行链收口，而不是 910B facts / profile 缺口：

- legacy `config.json` 兼容桥仍存在
- hook 仍保留 legacy 注入分支
- 如果将来要支持非 index-list 的 Ascend selector，可能还需要新的 parser / fallback 逻辑

## 五、完成判定

本次将“910B 完成”定义为：

- factsheet 完成
- 910B profile 完成
- 差异矩阵完成到足以解释 910B 和 Iluvatar 的关键差异
- 关键路径有真实节点 / workload 验证记录
- 代码中已补上 910B 所需的 `controlDeviceGlobs` 消费和更通用的 identifier 判定

按上述定义，910B 这一条已经完成。
