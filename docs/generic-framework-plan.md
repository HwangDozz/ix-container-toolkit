# ix-container-toolkit 通用化改造计划

> 更新日期：2026-04-03
> 目标：在不破坏当前 Iluvatar 链路的前提下，把项目演进为可通过 YAML profile 适配多厂商加速卡注入的通用框架。

## 一、背景

当前项目已经具备可工作的三段式链路：

- `ix-installer` 安装二进制、写配置、patch `containerd`
- `ix-container-runtime` 在 `create` 阶段向 OCI spec 注入 `prestart hook`
- `ix-container-hook` 在容器启动前把设备、驱动库、驱动工具和 linker 配置写入容器 rootfs

但实现仍然强绑定 Iluvatar：

- 设备节点前缀、资源名、节点标签、runtime handler、RuntimeClass 名称都带有 Iluvatar 假设
- UUID 查询命令和查询环境变量写死为 `ixsmi` / `corex`
- 当前配置模型只能表达少量路径和日志参数，不能完整表达厂商差异
- 文档中还存在部分与代码实现不一致的描述

因此，首轮目标不是直接做多厂商运行，而是先把现有 Iluvatar 方案抽象成通用 profile 驱动的框架骨架，再以昇腾 310P / 910B 作为首批外部样本验证抽象。

## 二、当前事实与主要缺口

### 2.1 当前代码的真实行为

- runtime 真正改写的是 OCI bundle 中的 `hooks.prestart`
- hook 读取 OCI state stdin 与 bundle 内的 `config.json`
- hook 通过设备选择环境变量判断是否进行 GPU 注入
- hook 当前会注入：
  - `/dev/iluvatarN`
  - 驱动库目录内容
  - 驱动工具目录内容
  - `/etc/ld.so.conf.d/ix-toolkit.conf`
  - `ldconfig`

### 2.2 当前缺口

- 文档中多处写到会注入 `PATH` / `LD_LIBRARY_PATH` / `PYTHONPATH`，但代码实际上没有修改 OCI env
- 当前硬编码项过多，无法仅靠配置切换到其他厂商
- UUID 到设备节点的查询逻辑没有被抽象成通用策略
- 文件注入模式没有被建模成声明式 artifact 列表
- installer 仍然写死 `ix` handler 和 `iluvatar.ai/gpu=present`

### 2.3 首轮通用化边界

首轮纳入范围：

- Iluvatar 现有能力抽象
- YAML profile 设计
- Ascend 310P / 910B 调研与样本 profile
- 为后续多厂商接入做代码骨架改造计划

首轮不纳入范围：

- 寒武纪代码接入
- profile 继承机制
- 动态脚本化 profile
- 一次性完成所有厂商支持

## 三、优先级与阶段目标

### P0：现状纠偏与事实基线

目标：

- 把当前实现与文档描述之间的偏差清理干净
- 产出后续抽象的唯一事实输入

任务：

- 梳理当前代码真实行为与历史文档的差异
- 归类所有 Iluvatar 相关硬编码点
- 形成 Iluvatar 现状 profile 字段清单
- 明确哪些能力已经存在，哪些只是文档目标但代码未实现

输出物：

- 一份事实基线文档
- 一份硬编码项清单
- 一份 Iluvatar profile 草稿字段表

完成判定：

- 后续 profile 设计不再依赖模糊描述
- 实现者能明确区分“现有能力”和“待补能力”

### P1：最小 YAML Profile Schema 设计

目标：

- 设计首版通用 profile 模型，作为后续调研和代码改造的约束

原则：

- 使用 YAML
- 单厂商单文件
- 默认兼容现有 `ix`
- 允许声明新的 runtime handler / RuntimeClass 命名
- schema 和调研并行推进，但以 schema 为主线约束调研字段

建议结构：

- `metadata`
- `runtime`
- `kubernetes`
- `device`
- `inject`

必须覆盖的能力：

- 设备选择环境变量
- 设备节点枚举规则
- index / UUID 解析策略
- 厂商查询命令、命令参数、命令环境和输出解析
- 文件注入模式
- linker 配置策略
- 可选 env 注入策略

输出物：

- YAML schema 设计说明
- Iluvatar 对应的首版 YAML profile 样例
- profile 加载与校验契约

完成判定：

- Iluvatar 的现有事实可以完整映射到 schema
- schema 可以无歧义容纳 Ascend 样本事实

### P2：昇腾 310P / 910B 调研与样本 profile

目标：

- 获取真实 Ascend 运行时事实，验证 profile 抽象是否足够

原则：

- 310P 与 910B 分开采集，不预设完全兼容
- 调研范围按“尽量全量”执行
- 进入 schema 的字段优先服务运行时注入能力

采集重点：

- `/dev` 设备节点形态
- Device Plugin 注入的 env、resource name、annotation
- 驱动目录与版本化软链关系
- 厂商查询工具、命令参数、依赖环境
- 容器内最小可用性验证方式
- 是否需要额外 env 注入或特殊 linker 处理
- 是否存在控制节点、管理节点或多个设备命名空间

输出物：

- `310P` factsheet
- `910B` factsheet
- 每类卡一份候选 YAML profile 草案
- 与 Iluvatar 抽象差异矩阵

完成判定：

- 对每类卡都能明确列出运行时注入所需的最小字段
- 不再依赖拍脑袋推断 Ascend 行为

### P3：代码改造与兼容接入

目标：

- 把当前项目改造成“通用引擎 + YAML profile”，同时保持 Iluvatar 零回归

改造方向：

- 将当前 Iluvatar 默认值迁入默认 profile 或内置 profile
- 为 runtime / hook / installer 增加 profile 加载层
- 将设备解析拆为通用流水线：
  - 设备枚举
  - selector 解析
  - UUID / index 映射
  - fallback 策略
- 将注入行为改为 artifact 驱动，而不是路径类别硬编码
- 明确建模 env 注入和 linker 注入
- 让 installer 支持根据 profile 生成 runtime handler、RuntimeClass 和节点标签

兼容策略：

- 默认仍支持现有 `ix`
- 同时允许通过 profile 声明中性命名
- 未指定 profile 时回退到 Iluvatar 兼容模式

输出物：

- profile 驱动的配置加载与校验层
- 通用设备解析层
- 通用 artifact 注入层
- 兼容 Iluvatar 的默认 profile

完成判定：

- Iluvatar 行为零回归
- 新厂商接入不需要继续往核心代码里新增厂商常量

## 四、首版 Profile 设计要求

### 4.1 文件组织

- 每个 vendor / 产品线一个 YAML 文件
- 首版不做继承与组合
- profile 文件优先作为显式输入，不做自动发现

### 4.2 首版推荐字段分组

`metadata`

- `name`
- `vendor`
- `modelFamily`
- `version`

`runtime`

- `handlerName`
- `runtimeClassName`
- `underlyingRuntime`
- `hookStage`

`kubernetes`

- `resourceNames`
- `nodeLabels`
- `tolerations`

`device`

- `selectorEnvVars`
- `selectorFormats`
- `deviceGlob`
- `controlDeviceGlobs`
- `mappingStrategy`
- `mappingCommand`
- `mappingEnv`
- `mappingParser`
- `fallbackPolicy`

`inject`

- `containerRoot`
- `artifacts`
- `linker`
- `extraEnv`

### 4.3 首版不做的复杂特性

- profile 继承
- 条件表达式语言
- 嵌入 shell 脚本
- 一个文件定义多个 profile 并做复杂选择

## 五、实施原则

- 先纠偏，再抽象，再调研，再改代码
- schema 与调研并行推进，但不允许脱离 schema 做散点调研
- 任何新厂商差异都先写进 factsheet，再决定是否进入通用 schema
- 代码改造必须把“默认兼容 Iluvatar”作为硬约束
- 不把厂商调研文档和实施计划文档混写

## 六、风险与注意事项

- 若先写代码再反推 schema，容易把 Iluvatar 假设继续固化
- 若不先纠正文档偏差，后续实现者会误以为已有 env 注入能力
- 若把 310P 和 910B 合并建模，可能掩盖设备节点、工具命令或驱动目录差异
- 若继续把 fallback 策略写死在核心代码，通用化后会难以维护
- 若 installer 不 profile 化，runtime/hook 即使抽象完成也无法完整落地

## 七、验收标准

- Iluvatar 当前链路行为零回归
- YAML profile 能完整表达 Iluvatar 的已知运行时事实
- YAML profile 能无歧义表达 Ascend 310P / 910B 的已采集事实
- 新厂商接入以新增 profile 为主，而不是继续添加厂商硬编码
- 文档、实现、测试三者对“当前能力边界”描述一致

## 八、后续文档规划

本计划文档用于描述总体路线，后续建议新增：

- `docs/ascend-310p-research.md`
- `docs/ascend-910b-research.md`
- `docs/profile-schema-design.md`
- `docs/profile-migration-plan.md`

这样可以把“计划”“事实”“设计”“迁移”四类信息分开维护。
