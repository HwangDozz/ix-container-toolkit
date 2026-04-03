# 2026-04-03 工作总结

> 目的：汇总今天这轮大重构已经完成的工作，明确当前仓库离“通用的 profile 驱动加速卡注入框架”这个目标还差哪些事，以及下一步建议的推进顺序。

## 一、阶段目标回顾

当前项目的整体目标已经比较明确：

- 不再围绕单一厂商实现专用运行时
- 把节点侧驱动事实收敛到 YAML profile
- 由同一套 runtime / hook / installer 在不同节点上按 profile 工作
- 支持 mixed-arch 集群和 mixed-vendor 节点部署
- 让部署入口、运行时行为、注入逻辑和文档口径都以 profile 为中心

今天这轮工作的重点不是继续加功能，而是把主执行链真正收口到这个目标上。

## 二、今天完成的工作

### 2.1 运行时主链改成 profile 必选

已经完成：

- `accelerator-container-runtime`
- `accelerator-container-hook`
- `accelerator-installer`

这三个组件现在都要求能成功加载 active profile。

当前行为是：

- 默认从 `/etc/accelerator-toolkit/profiles/active.yaml` 读取 profile
- 也支持通过 `ACCELERATOR_PROFILE_FILE` 显式指定 profile 路径
- 如果 profile 缺失、不可读或校验失败，组件直接报错退出
- 不再回退到历史单厂商默认值

这意味着“节点上一份 active profile 是唯一事实源”已经从设计变成了代码现实。

### 2.2 主执行链去掉旧的厂商兼容默认值

已经完成：

- 删除旧的 vendor fallback 主路径
- `runtimeview` 不再承担厂商回退
- `runtime` / `hook` 不再按“有 profile / 无 profile”走双分支
- `device` 层不再依赖 `GPU-` 风格的硬编码判断

结果是：

- 同一套二进制按 profile 工作
- 不再依赖 Iluvatar 风格默认 env、默认路径、默认设备前缀
- 非测试主代码里已经清掉 `Iluvatar`、`ILUVATAR`、`/dev/iluvatar`、`corex`、`GPU-` 这类旧硬编码

### 2.3 hook 注入路径继续通用化

已经完成：

- `controlDeviceGlobs` 已真正接入 hook 设备注入
- `artifact + linker` 已成为 hook 主注入模型
- 910B 所需控制节点和库路径已经能够通过 profile 驱动

这使 910B 不再只是“能被 schema 表达”，而是已经进入可执行的主链。

### 2.4 910B 样本链路完成到可交付状态

已经完成：

- 910B factsheet
- 910B validation 文档
- 910B 正式 profile
- Iluvatar vs Ascend 差异矩阵中的 910B 部分

当前仓库内，`profiles/ascend-910b.yaml` 已不再是占位草案，而是可以直接加载、渲染并参与运行链路的 profile。

### 2.5 多架构构建与分发链已经落地

已经完成：

- `make build` 同时产出平铺二进制和 `bin/<os>-<arch>/`
- Dockerfile / Dockerfile.prebuilt 支持按架构构建
- installer 镜像支持 multi-arch manifest
- DaemonSet 维持单镜像入口，依赖 Kubernetes 自动按节点架构拉取对应变体

这部分已经满足“runtime / hook 将来跑在不同架构节点上”的基础要求。

### 2.6 `config.json` 进一步瘦身

今天又额外做了一轮收口：

- `config.json` 不再承载设备路径、selector env、driver 库路径、library mode 这类 profile 事实
- 它现在只保留进程级配置：
  - `underlyingRuntime`
  - `hookPath`
  - `logLevel`
  - `logFile`
  - `disableRequire`

这一步的意义很直接：

- profile 负责节点事实
- `config.json` 只负责少量进程级覆盖
- 运行时不再同时维护两套事实模型

### 2.7 项目活跃入口已改为中性命名

已经完成：

- Go module 改为 `github.com/accelerator-toolkit/accelerator-toolkit`
- 命令入口目录改名为：
  - `cmd/accelerator-container-runtime`
  - `cmd/accelerator-container-hook`
  - `cmd/accelerator-installer`
  - `cmd/accelerator-profile-render`
- 默认环境变量改为：
  - `ACCELERATOR_CONFIG_FILE`
  - `ACCELERATOR_PROFILE_FILE`
  - `ACCELERATOR_LOG_LEVEL`
- 默认宿主机目录改为 `/etc/accelerator-toolkit`
- 默认 linker 配置文件改为 `/etc/ld.so.conf.d/accelerator-toolkit.conf`
- 渲染器生成的 DaemonSet / RBAC 资源名改为 `accelerator-toolkit`

这一步完成后，项目的活跃代码和部署入口不再继续暴露历史 `ix-*` 命名。

## 三、当前阶段结论

如果只看今天之前的状态，项目是“已经有 generic profile 骨架，但执行链仍保留较重历史包袱”。  
经过今天这轮改造之后，当前状态可以更新为：

- generic profile 驱动主链已经成立
- active profile 已成为运行时唯一事实源
- 910B 已完成首个非 Iluvatar 样本闭环到“可加载、可渲染、可执行主链”
- mixed-arch 分发链已经具备
- 活跃代码和入口命名已经切到更通用的项目表达

更直白地说：

- “框架骨架”这件事已经不是设计草案
- 现在缺的主要不是继续去默认值
- 而是验证收口、文档清理和再补一个厂商样本

## 四、当前还没完成的事

### 4.1 还缺一次 910B 自部署闭环验证

虽然 910B facts 和 profile 已经齐了，但还差一份更强的运行验证：

- 使用 `profiles/ascend-910b.yaml`
- 用当前命名后的 installer/runtime/hook 重新部署
- 在真实 910B 节点上确认：
  - RuntimeClass 生效
  - runtime 注入 hook 成功
  - hook 注入 device / artifacts / linker 成功
  - 业务容器实际可运行

这是当前最值得优先补的一步，因为它能证明“通用框架 + profile 主链”在第二厂商上真正闭环。

### 4.2 旧文档还没有统一跟上当前实现

现在仓库里仍有不少历史文档保留旧表述：

- 旧项目名
- 旧 `ix-*` 命名
- 旧 `/etc/ix-toolkit`
- 旧 `IX_*` 环境变量
- 旧 `ix` handler / runtimeClass

这些文档不会影响代码运行，但会明显干扰后续维护判断。  
这部分应该做一次系统性文档收口。

### 4.3 `config.json` 是否还能继续缩薄，需要一次明确决策

今天已经把 `config.json` 砍到很小，但还保留了：

- `underlyingRuntime`
- `hookPath`
- 日志项
- `disableRequire`

接下来要明确：

- 这些字段是否仍值得单独落盘
- 还是应该继续收进 profile 或 installer 渲染逻辑

这一步不一定必须立刻做，但需要明确方向，避免以后又重新把节点事实塞回 JSON。

### 4.4 310P 样本仍未补齐

按当前决策，310P 可以暂时后置。  
但从“框架泛化已经被第二、第三类样本验证”这个角度看，后面仍建议补上：

- 310P factsheet
- 310P profile
- 与 910B / Iluvatar 的差异确认

## 五、下一步建议顺序

建议按下面顺序推进。

### P1. 先做 910B 自部署闭环验证

目标：

- 用当前 `accelerator-*` 命名后的构建产物和 profile 真跑一遍 910B
- 形成一份新的、跟当前代码一致的验证记录

完成判定：

- 节点成功安装 active profile
- containerd runtime handler 正常注册
- RuntimeClass 能调度到 910B 节点
- 容器里能看到所需设备和驱动路径
- 最小 workload 成功执行

### P2. 再统一历史文档口径

目标：

- 更新 README、docs 索引和关键状态文档
- 明确现在的活跃入口已经是：
  - `accelerator-*` 命令
  - `/etc/accelerator-toolkit`
  - `ACCELERATOR_*` 环境变量
  - profile 驱动部署

完成判定：

- 新接手的人不再会因为文档误导去找旧入口

### P3. 最后决定 `config.json` 的长期去留

目标：

- 明确它是继续保留为轻量进程配置
- 还是进一步并入 profile / 渲染输出

建议标准：

- 如果某字段属于节点事实，就不应保留在 JSON
- 如果某字段只是运行时覆盖项，可以继续保留

### P4. 310P 后置补样本

目标：

- 把“一个历史样本 + 一个已完成 Ascend 样本”推进到“至少两类非同构样本都可表达”

## 六、建议的阶段完成判定

如果要判断“这一阶段什么时候算完成”，建议按下面口径：

1. `accelerator-*` 命名后的主代码和部署入口稳定
2. 910B profile 完成真实自部署闭环验证
3. 旧文档完成一轮口径更新
4. `config.json` 的长期定位明确

满足这四条之后，可以认为“首轮通用 profile 驱动框架收口完成”。

## 七、附记

今天这轮工作的核心价值，不是单点功能增加，而是把项目从“带强历史兼容痕迹的通用化过渡态”推进到了“主执行链已经以 profile 为中心”的状态。  
这会直接降低后续继续支持 Ascend、补更多样本、或者替换部署入口时的复杂度。
