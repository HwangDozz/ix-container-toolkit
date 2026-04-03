# Ascend 310P Factsheet

> 状态：未完成
> 更新日期：2026-04-03
> 目的：收集 Ascend 310P 的真实运行时注入事实，作为 profile 建模与通用化实现的输入。

## 一、样本环境

待补：

- 节点型号：
- OS / Kernel：
- containerd 版本：
- Kubernetes 版本：
- 驱动版本：
- Device Plugin 版本：
- 采集人 / 采集时间：

## 二、设备节点事实

待补：

- 主设备节点命名：
- 是否存在控制节点：
- 是否存在管理节点：
- 是否存在子设备节点：
- `ls -l /dev` 关键样例：

## 三、Device Plugin 注入事实

待补：

- resource name：
- 注入的 env：
- 注入的 annotation：
- 是否使用 UUID / index / 其他 selector：
- 一个真实 Pod 的相关 spec 片段：

## 四、驱动与工具目录

待补：

- 驱动根目录：
- 共享库目录：
- 工具目录：
- 版本目录与软链关系：
- 容器内最小运行必需目录：

## 五、查询工具与映射命令

待补：

- 查询工具路径候选：
- 查询命令：
- 命令输出样例：
- 命令依赖的 env：
- 是否能从输出稳定解析 index / UUID / device id：

## 六、容器内最小可用性验证

待补：

- 最小验证镜像：
- 最小验证命令：
- 成功判据：
- 失败时典型报错：

## 七、额外注入需求

待补：

- 是否需要额外 env 注入：
- 是否需要特殊 linker 策略：
- 是否需要额外目录挂载：
- 是否需要控制设备一并注入：

## 八、对当前 schema 的映射

待补：

- `runtime`：
- `kubernetes`：
- `device`：
- `inject`：

## 九、未解问题

待补：

- [ ] 是否需要多个 selector env 同时兼容
- [ ] 是否存在当前 `mapping.parser` 无法表达的输出格式
- [ ] 是否存在当前 `artifact` 模型无法表达的注入对象
