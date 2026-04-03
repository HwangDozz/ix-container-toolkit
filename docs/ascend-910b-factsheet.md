# Ascend 910B Factsheet

> 状态：完成
> 更新日期：2026-04-03
> 目的：收集 Ascend 910B 的真实运行时注入事实，作为 profile 建模与通用化实现的输入。

## 一、样本环境

当前已确认：

- 节点架构：`aarch64`
- 节点型号：`kunlun-02`
- OS：`openEuler 22.03 (LTS-SP4)`
- Kernel：`5.10.0-303.0.0.206.oe2203sp4.aarch64`
- 驱动安装根目录：`/usr/local/Ascend`
- Driver install info：见 `/etc/ascend_install.info`
- CANN 目录：`/usr/local/Ascend/cann-8.5.1`
- toolkit 入口：`/usr/local/Ascend/ascend-toolkit`
- 采集时间：`2026-04-03`
- 采集来源：
  - 当前节点宿主机文件系统
  - 运行中的 910B workload Pod
  - `kube-system/ascend-device-plugin-daemonset-*`

仍待补：

- containerd 版本：
- Kubernetes 版本：
- 910B Device Plugin 版本的更完整镜像 digest / 构建来源：

## 二、设备节点事实

当前观察：

- 在运行中的 910B workload Pod 内，可见主设备节点：
  - `/dev/davinci0`
  - `/dev/davinci3`
- 可见控制 / 管理类设备节点：
  - `/dev/davinci_manager`
  - `/dev/devmm_svm`
  - `/dev/hisi_hdc`
- 设备节点样例权限与主次设备号：
  - `/dev/davinci0` -> `crw-rw---- 237,0`
  - `/dev/davinci3` -> `crw-rw---- 237,3`
  - `/dev/davinci_manager` -> `crw-rw---- 238,0`
  - `/dev/devmm_svm` -> `crw-rw---- 236,0`
  - `/dev/hisi_hdc` -> `crw-rw---- 235,0`
- 宿主机内核已加载大量 Ascend / devdrv / davinci 相关模块，说明驱动侧与字符设备子系统已激活

已确认的相关内核模块样例：

- `drv_davinci_intf_host`
- `drv_devmm_host`
- `drv_devmng_host`
- `ascend_queue`
- `ascend_event_sched_host`
- `ascend_logdrv`

仍待补：

- 是否还存在当前样本未分配进 Pod 的其他控制节点 / 子设备节点
- 宿主机侧更完整的 `/dev` 清单

## 三、Device Plugin 注入事实

当前已确认：

- Device Plugin DaemonSet：
  - 名称：`kube-system/ascend-device-plugin-daemonset`
  - 镜像：`ascend-k8sdeviceplugin:v7.3.0`
  - 关键参数：`-useAscendDocker=true`
  - 挂载：`/usr/local/Ascend/driver`
- 节点选择：
  - `nodeSelector.accelerator=huawei-Ascend910`
  - toleration key：`huawei.com/Ascend910`
- 资源名：
  - `huawei.com/Ascend910`
- workload Pod 相关 annotation：
  - `huawei.com/Ascend910: Ascend910-0,Ascend910-3`
  - `huawei.com/AscendReal: Ascend910-0,Ascend910-3`
  - `huawei.com/kltDev: Ascend910-0,Ascend910-3`
- workload Pod 资源请求：
  - `limits/requests["huawei.com/Ascend910"]="2"`
- 实际运行时 selector env：
  - `ASCEND_VISIBLE_DEVICES=0,3`
- 当前样本使用的是 index-list，而不是 UUID-list

## 四、驱动与工具目录

当前已确认：

- 驱动根目录：`/usr/local/Ascend`
- driver 库目录：
  - `/usr/local/Ascend/driver/lib64`
  - `/usr/local/Ascend/driver/lib64/common`
  - `/usr/local/Ascend/driver/lib64/driver`
- toolkit / runtime 根目录：`/usr/local/Ascend/cann-8.5.1`
- toolkit 共享库目录：
  - `/usr/local/Ascend/cann-8.5.1/lib64`
  - `/usr/local/Ascend/cann-8.5.1/runtime/lib64`
  - `/usr/local/Ascend/cann-8.5.1/devlib`
- toolkit / runtime / tools 目录：
  - `/usr/local/Ascend/cann-8.5.1/bin`
  - `/usr/local/Ascend/cann-8.5.1/runtime/bin`
  - `/usr/local/Ascend/driver/tools`
- `ascend-toolkit` 为入口目录，`set_env.sh` 指向 `ascend-toolkit/latest/set_env.sh`
- `cann-8.5.1` 下存在大量软链：
  - `bin -> aarch64-linux/bin`
  - `lib64 -> aarch64-linux/lib64`
  - `include -> aarch64-linux/include`
  - `devlib -> aarch64-linux/devlib`
- 当前节点还存在：
  - `/usr/local/Ascend/nnal/atb/latest/atb/cxx_abi_0/lib`
  - `/usr/local/Ascend/ascend-toolkit/latest/tools/ccec_compiler/bin`
- workload Pod 的运行时 env 中还出现过：
  - `/usr/local/Ascend/nnal/atb/latest/atb/cxx_abi_0/examples`
  - `/usr/local/Ascend/nnal/atb/latest/atb/cxx_abi_0/tests/atbopstest`
  但当前宿主机侧未找到这两个目录，暂不把它们视为必需的 host-side artifact

当前推断：

- 容器最小运行依赖大概率不是单个目录，而是 `driver/lib64* + cann/lib64 + runtime/lib64` 的组合
- 具体哪些目录必须注入，仍需结合最小运行验证确认

## 五、查询工具与映射命令

当前已确认：

- 查询工具路径候选：
  - `/usr/local/bin/npu-smi`
  - `/usr/local/sbin/npu-smi`
- 相关库：
  - `/usr/local/Ascend/driver/lib64/driver/libdrvdsmi_host.so`
  - `/usr/local/Ascend/driver/lib64/driver/libdcmi.so`
  - `/usr/local/dcmi/libdcmi.so`
- `npu-smi` 为 `ELF 64-bit LSB pie executable, ARM aarch64`
- `ldd /usr/local/bin/npu-smi` 显示其直接依赖：
  - `libc_sec.so`
  - `libdrvdsmi_host.so`
  - `libmmpa.so`
  - `libascend_hal.so`
- 在运行中的 910B workload Pod 内，`npu-smi info` 可稳定执行
- 当前样本输出显示：
  - 可见设备索引：`0`、`3`
  - 设备型号：`910B3`
  - `Bus-Id`：`0000:C1:00.0`、`0000:82:00.0`
  - 输出是表格文本，不是 CSV
- 运行中的 workload Pod 内 `npu-smi` 版本：
  - `25.5.1`

当前观察：

- 在当前受限宿主机会话中，`npu-smi info` 无法稳定返回设备信息
- 使用 toolkit 环境后可见错误：`Cannot open netlink socket: Operation not permitted`
- 在真实 workload Pod 中，`npu-smi info` 成功，说明阻塞点来自当前会话权限 / 命名空间，而不是二进制缺失或库路径错误

从二进制字符串中可确认的能力线索：

- 支持 `info`
- 支持 `reset`
- 支持 `list_device`
- 支持 `device health`
- 支持 `hbm info`
- 支持 `chip info`

仍待补：

- 是否存在可用于宿主机外部映射的稳定唯一标识
- 是否需要为 `npu-smi info` 单独实现表格 parser

## 六、容器内最小可用性验证

当前已确认的样例：

- 宿主机当前会话：
  - 未补齐环境或权限时，`npu-smi info` 可能超时
  - 补齐 toolkit 环境后，当前会话报错：`Cannot open netlink socket: Operation not permitted`
- 运行中 workload Pod：
  - `ASCEND_VISIBLE_DEVICES=0,3`
  - `/dev/davinci0`、`/dev/davinci3` 可见
  - `npu-smi info` 成功返回 2 张卡的状态表

仍待补：

- 最小验证镜像
- 最小验证命令
- 容器内最小必需库 / 环境的删减边界

## 七、额外注入需求

当前已确认：

- `set_env.sh` 会修改：
  - `PATH`
  - `LD_LIBRARY_PATH`
  - `PYTHONPATH`
  - `ASCEND_OPP_PATH`
  - `ASCEND_AICPU_PATH`
  - `TOOLCHAIN_HOME`
  - `ASCEND_HOME_PATH`
  - `ASCEND_TOOLKIT_HOME`
  - `CMAKE_PREFIX_PATH`
- `LD_LIBRARY_PATH` 明确包含：
  - `/usr/local/Ascend/ascend-toolkit/latest/lib64`
  - `/usr/local/Ascend/ascend-toolkit/latest/lib64/plugin/opskernel`
  - `/usr/local/Ascend/ascend-toolkit/latest/lib64/plugin/nnengine`
  - `/usr/local/Ascend/ascend-toolkit/latest/tools/aml/lib64`
  - `/usr/local/Ascend/ascend-toolkit/latest/tools/aml/lib64/plugin`
  - `/usr/local/Ascend/driver/lib64`
  - `/usr/local/Ascend/driver/lib64/common`
  - `/usr/local/Ascend/driver/lib64/driver`
- `PYTHONPATH` 明确依赖：
  - `/usr/local/Ascend/ascend-toolkit/latest/python/site-packages`
  - `/usr/local/Ascend/ascend-toolkit/latest/opp/built-in/op_impl/ai_core/tbe`
- 运行中的 workload Pod 还带有：
  - `CRATER_ASCEND_ENV_INITIALIZED=1`
  - `ASCEND_RUNTIME_OPTIONS=`
  - `NPU_COMPUTING_FORECAST_HOME=/opt/npu_computing_forecast`
- 当前样本 workload Pod 的 `PATH` 包含：
  - `/usr/local/Ascend/ascend-toolkit/latest/bin`
  - `/usr/local/Ascend/ascend-toolkit/latest/tools/ccec_compiler/bin`
- `/usr/local/Ascend/nnal/atb/latest/atb/cxx_abi_0/lib` 已在宿主机确认存在
- `ATB` 的 `examples` / `tests` 目录在 workload env 中出现，但当前宿主机未确认存在

当前推断：

- Ascend 很可能需要比 Iluvatar 更重的 env 注入
- linker 可能仍可用 `ldconfig`，但仅依赖 `ld.so.conf.d` 可能不足以替代完整 env 初始化

仍待补：

- 是否必须注入完整 `set_env.sh` 等价环境
- 是否需要额外控制设备一并注入

## 八、对当前 schema 的映射

当前可映射的已知项：

- `runtime.underlyingRuntime`: `runc`
- `runtime.hookStage`: `prestart`
- `runtime.hookBinary`: `/usr/local/bin/accelerator-container-hook`
- `runtime.handlerName`: `ascend-910b`
- `runtime.runtimeClassName`: `ascend-910b`
- `kubernetes.resourceNames`: `["huawei.com/Ascend910"]`
- `kubernetes.nodeLabels`: `{"accelerator":"huawei-Ascend910"}`
- `kubernetes.runtimeClassScheduling.nodeSelector`: `{"accelerator":"huawei-Ascend910"}`
- `kubernetes.runtimeClassScheduling.tolerations`:
  - `key=huawei.com/Ascend910`
  - `operator=Exists`
  - `effect=NoSchedule`
- `device.selectorEnvVars`: `["ASCEND_VISIBLE_DEVICES"]`
- `device.selectorFormats`: `["index-list"]`
- `device.deviceGlobs`: `["/dev/davinci*"]`
- `device.controlDeviceGlobs`:
  - `/dev/davinci_manager`
  - `/dev/devmm_svm`
  - `/dev/hisi_hdc`
- `device.mapping.strategy.primary`: 可先落为 `env-index-list`
- `inject.containerRoot`: `/usr/local/Ascend`
- `inject.artifacts.shared-libraries`: 至少覆盖
  - `/usr/local/Ascend/driver/lib64/common`
  - `/usr/local/Ascend/driver/lib64/driver`
  - `/usr/local/Ascend/nnal/atb/latest/atb/cxx_abi_0/lib`
  - `/usr/local/Ascend/ascend-toolkit/latest/lib64`
  - `/usr/local/Ascend/ascend-toolkit/latest/tools/aml/lib64`
- `inject.artifacts.directory`: 至少覆盖
  - `/usr/local/Ascend/driver/tools`
  - `/usr/local/Ascend/ascend-toolkit/latest/bin`
  - `/usr/local/Ascend/ascend-toolkit/latest/tools/ccec_compiler/bin`
- `inject.extraEnv`: 需要吸收 `ASCEND_*`、`PATH`、`LD_LIBRARY_PATH`、`PYTHONPATH`

仍待补：

- `mapping.command` 是否保留为调试 / 诊断用途
- `mapping.parser` 是否需要新增 `npu-smi` 表格解析器
- 是否需要把 `ATB` 相关路径纳入 host-side artifact

## 九、与 310P 的差异

当前已确认：

- 驱动目录差异：
  - Iluvatar 主要集中在单个 `/usr/local/corex`
  - 910B 明显分成 `driver`、`cann`、`runtime`、`devlib`、`opp`、`tools`
- 工具差异：
  - Iluvatar 使用 `ixsmi`
  - 910B 使用 `npu-smi` / `hccn_tool` / `msnpureport`
- env 差异：
  - Iluvatar 当前几乎不依赖运行时额外 env
  - 910B 的 `set_env.sh` 明确需要大量 `PATH` / `LD_LIBRARY_PATH` / `PYTHONPATH` / `ASCEND_*` 变量

仍待补：

- 设备节点差异
- 真实 selector 差异
- 最小容器依赖差异

## 十、未解问题

- [x] 已确认真实 `/dev` 节点命名样例
- [x] 已确认 Device Plugin 的 resource name / annotation / selector env
- [ ] 需要确认 `npu-smi` 是否需要单独表格 parser，还是当前 index-list 已足够
- [ ] 需要确认当前 `artifact + extraEnv + linker` 模型是否足以覆盖 `set_env.sh`
- [ ] 需要确认 `ATB` / `NNAL` 相关路径是否必须由 host 注入
