# ix-container-toolkit 通用化阶段执行任务拆解

> 更新日期：2026-04-03
> 作用：把 `docs/generic-framework-remaining-work.md` 中的剩余工作进一步拆成可执行任务，便于按阶段推进、分配和验收。

## 一、建议执行顺序

推荐按下面顺序推进：

1. `P2-A` Ascend 310P 事实采集
2. `P2-B` Ascend 910B 事实采集
3. `P2-C` Ascend 候选 profile 与差异矩阵
4. `P3-A` 设备解析通用流水线收口
5. `P3-B` hook artifact 主路径收口
6. `P3-C` runtime / installer / runtimeview 去兼容桥
7. `V1` installer 自动化测试
8. `V2` profile 驱动端到端验证
9. `D1` 文档状态统一收口

## 二、`P2` 任务拆解

### `P2-A` Ascend 310P factsheet

目标：

- 形成一份可落到 `docs/` 的 310P 事实文档

任务：

- 确认 `/dev` 设备节点命名规则
- 确认是否存在控制节点、管理节点、子设备节点
- 确认 Device Plugin 注入的 env / annotation / resource name
- 确认驱动目录、版本目录和软链关系
- 确认厂商查询工具、命令参数和依赖环境
- 确认容器内最小可用性验证方法
- 记录是否需要额外 env 注入或 linker 特殊处理

输出物：

- `docs/ascend-310p-factsheet.md`

完成判定：

- 后续实现者不需要再凭经验猜测 310P 的运行时注入事实

### `P2-B` Ascend 910B factsheet

目标：

- 形成一份可落到 `docs/` 的 910B 事实文档

任务：

- 与 `P2-A` 相同维度采集
- 特别确认 910B 是否与 310P 在设备节点、驱动布局、查询工具、最小运行依赖上存在明显差异

输出物：

- `docs/ascend-910b-factsheet.md`

完成判定：

- 可以明确区分“310P / 910B 共性”和“910B 特有差异”

### `P2-C` Ascend 候选 profile 与差异矩阵

目标：

- 用真实样本验证当前 schema 是否足够

任务：

- 基于 310P factsheet 起草 `profiles/ascend-310p.yaml`
- 基于 910B factsheet 起草 `profiles/ascend-910b.yaml`
- 对照 Iluvatar 样本整理差异矩阵
- 记录 schema 不足之处和建议补充字段

输出物：

- `profiles/ascend-310p.yaml`
- `profiles/ascend-910b.yaml`
- `docs/ascend-vs-iluvatar-diff-matrix.md`

完成判定：

- 至少一个 Ascend 样本能完整映射到现有 schema
- 如果不能完整映射，缺口已被清晰记录，而不是散落在口头讨论中

## 三、`P3` 任务拆解

### `P3-A` 设备解析通用流水线收口

目标：

- 把当前“参数化后的 Iluvatar 逻辑”推进为更通用的设备解析模型

任务：

- 收敛 selector 格式判定逻辑
- 移除或弱化 `GPU-` 前缀硬编码
- 把 `fallbackPolicy` 落到执行逻辑
- 为 `mapping.parser` 增加可扩展实现点
- 明确并实现 `controlDeviceGlobs` 的消费逻辑
- 为非 Iluvatar 样本补设备解析测试

涉及文件：

- `pkg/device/device.go`
- `pkg/device/device_test.go`
- 可能还会涉及 `pkg/profile/profile.go`

完成判定：

- 新厂商接入不需要再改 `pkg/device` 里的厂商常量

### `P3-B` hook artifact 主路径收口

目标：

- 让 profile artifact 成为 hook 的主执行路径

任务：

- 明确 legacy 注入逻辑与 artifact 注入逻辑的保留边界
- 把设备 / 共享库 / 目录 / linker 四类注入统一到 artifact 模型
- 审视 `linker.strategy` 是否只保留 `ldconfig` 或需要扩展
- 评估是否继续保留旧的 `injectDriverLibraries` / `injectDriverBinaries` 专用路径
- 为 artifact 路径补更多覆盖测试

涉及文件：

- `internal/hook/hook.go`
- `internal/hook/hook_test.go`

完成判定：

- hook 的主逻辑不再依赖 Iluvatar 专用分支

### `P3-C` runtime / installer / runtimeview 去兼容桥

目标：

- 降低 `pkg/config` 在通用化主链路中的核心地位

任务：

- 继续收缩 `DefaultsFromProfile()` 的桥接角色
- 明确 runtimeview 中哪些 fallback 仍然保留、哪些应该移除
- 审视 installer 是否仍需要写完整 legacy `config.json`
- 明确 runtime 的“请求设备判定”是否只保留 env 命中
- 梳理渲染器与 installer 中仍保留的项目级硬编码

涉及文件：

- `pkg/config/config.go`
- `pkg/runtimeview/view.go`
- `internal/runtime/runtime.go`
- `cmd/accelerator-installer/main.go`
- `pkg/profile/render.go`

完成判定：

- profile 成为主模型
- legacy config 只承担兼容职责，而不是主执行依赖

## 四、验证任务拆解

### `V1` installer 自动化测试

目标：

- 为 installer 的关键副作用补最小自动化保障

任务：

- 为 `copyBinaries` 增加临时目录测试
- 为 `writeConfig` 增加 profile + config 输出测试
- 为 `patchContainerd` 增加幂等测试
- 为节点打标逻辑增加可控 mock 测试

涉及文件：

- `cmd/accelerator-installer/main.go`
- 新增 `cmd/accelerator-installer/main_test.go` 或拆分辅助包

完成判定：

- installer 的关键文件写入与 patch 逻辑不再只能靠手工节点验证

### `V2` profile 驱动端到端验证

目标：

- 证明 profile 驱动链路在部署和运行时都能闭环

任务：

- 验证 `profile -> render bundle -> apply -> installer -> host profile/config`
- 验证 runtime 注入 hook 的 profile 路径
- 验证 hook 消费 artifact / linker / extraEnv 的路径
- 验证至少一个非 Iluvatar profile 在 loader / render / runtime / hook 上不会因 schema 不足而失败
- 验证 mixed-arch 环境中的 installer 镜像分发与宿主机二进制安装

完成判定：

- 当前阶段的“通用化骨架”不只是代码可编译，而是具备实际运行闭环

## 五、文档任务拆解

### `D1` 文档状态统一收口

目标：

- 让项目文档对阶段状态的描述一致

任务：

- 更新 `docs/generic-profile-schema.md` 的阶段状态
- 继续清理 `docs/project-status.md` 中的旧 env 名和旧链路描述
- 明确哪些文档是当前事实基线，哪些是历史记录
- 在 `P2` 完成后把 Ascend 相关文档加入 `docs/README.md`

完成判定：

- 文档阅读者不会再因为旧口径和新实现并存而误判阶段进度

## 六、建议的 issue 粒度

如果需要进一步拆成 issue，建议最小拆分如下：

- `Issue 1`: 产出 310P factsheet
- `Issue 2`: 产出 910B factsheet
- `Issue 3`: 编写 Ascend 候选 profile
- `Issue 4`: 编写 Iluvatar vs Ascend 差异矩阵
- `Issue 5`: 去掉 `GPU-` 前缀硬编码并收口 parser / fallback
- `Issue 6`: 完成 `controlDeviceGlobs` 执行路径
- `Issue 7`: 让 artifact 成为 hook 主路径
- `Issue 8`: 收缩 `pkg/config` 兼容桥
- `Issue 9`: 为 installer 增加自动化测试
- `Issue 10`: 增加 profile 驱动端到端验证
- `Issue 11`: 文档状态统一收口

## 七、里程碑建议

可按下面三个里程碑验收：

### 里程碑 M1：Schema 被第二厂商验证

判定：

- Ascend factsheet 完成
- 至少一份 Ascend 候选 profile 完成
- 差异矩阵完成

### 里程碑 M2：执行链去厂商化基本完成

判定：

- 设备解析不再依赖 Iluvatar 特定常量
- hook 主路径已收敛到 artifact 驱动
- runtime / installer 主路径主要由 profile 视图驱动

### 里程碑 M3：阶段收尾

判定：

- installer 自动化测试具备最小覆盖
- profile 驱动闭环验证完成
- 文档状态统一
