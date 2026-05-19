# CDI 工作流与多 Pod 设备注入机制

> 本文档详细解释 DRA driver 如何通过 CDI spec 文件向多个 Pod 注入不同设备，以及合并策略为什么能正确工作。

## 核心概念

CDI（Container Device Interface）体系中有两个关键对象，它们的职责完全不同：

| 对象 | 类比 | 职责 | 生命周期 |
|------|------|------|----------|
| **CDI spec 文件**（`/etc/cdi/<vendor>.json`） | 电话簿 | 描述节点上所有可注入设备的完整信息 | 节点级，随 Prepare/Unprepare 动态更新 |
| **CDI device ID**（`iluvatar.com/gpu=GPU-aaaa`） | 电话号码 | 告诉 containerd 注入**哪一个**设备 | 容器级，由 kubelet 按 Pod 传递 |

**CDI spec 文件是共享的设备注册表，CDI device ID 是指向注册表中特定条目的指针。**

## 完整工作流

### 阶段 1：DRA driver 启动，上报设备

```
DRA driver 启动
  → 扫描 /dev/iluvatar* 发现 8 块 GPU
  → 向 API Server 发布 ResourceSlice
  → 此时 /etc/cdi/ 下没有文件（还没有 Pod 申请设备）
```

### 阶段 2：Pod A 申请 GPU-aaaa

```
1. 用户提交 Pod A，ResourceClaimTemplate 声明需要 1 块 Iluvatar GPU
2. DRA allocator 匹配到 GPU-aaaa，写入 ResourceClaim.status.allocation
3. kubelet 调用 DRA driver: PrepareResourceClaims(claim-A)
4. DRA driver:
   a. 从 claim-A 的 allocation 中找到分配的设备名 "GPU-aaaa"
   b. 匹配到本地设备 /dev/iluvatar0
   c. 生成 CDI spec（含设备节点、驱动库挂载、环境变量、ldconfig hook）
   d. 读取 /etc/cdi/iluvatar.json → 不存在，返回 nil
   e. MergeDevices(nil, [GPU-aaaa条目]) → 新建 spec
   f. 写入 /etc/cdi/iluvatar.json
5. 返回 CDI device ID: ["iluvatar.com/gpu=GPU-aaaa"]
6. kubelet 将 CDI device ID 传递给 containerd
7. containerd 读取 /etc/cdi/iluvatar.json
8. 在 spec 中找到 name=="GPU-aaaa" 的条目
9. 按该条目的 containerEdits 注入设备节点、库文件、环境变量
```

此时 `/etc/cdi/iluvatar.json` 内容：

```yaml
cdiVersion: 1.1.0
kind: iluvatar.com/gpu
devices:
  - name: GPU-aaaa                    # ← Pod A 的设备
    containerEdits:
      env: [ILUVATAR_COREX_VISIBLE_DEVICES=all]
      deviceNodes:
        - hostPath: /dev/iluvatar0
          path: /dev/iluvatar0
          permissions: rwm
      mounts:
        - hostPath: /usr/local/corex/lib64/libcuda.so
          containerPath: /usr/local/corex/lib64/libcuda.so
          options: [rbind, ro]
        # ... 其他驱动库
      hooks:
        prestart:
          - hookName: accelerator-toolkit-ldconfig
            path: /sbin/ldconfig
```

### 阶段 3：Pod B 申请 GPU-bbbb

```
1. Pod B 的 ResourceClaim 被 DRA allocator 分配到 GPU-bbbb
2. kubelet 调用 DRA driver: PrepareResourceClaims(claim-B)
3. DRA driver:
   a. 从 claim-B 的 allocation 中找到 "GPU-bbbb"
   b. 匹配到本地设备 /dev/iluvatar1
   c. 生成 CDI spec（GPU-bbbb 的条目）
   d. 读取 /etc/cdi/iluvatar.json → 存在，返回现有 spec（含 GPU-aaaa）
   e. MergeDevices(existing, [GPU-bbbb条目]) → 合并
   f. 写入 /etc/cdi/iluvatar.json（现在包含两个条目）
4. 返回 CDI device ID: ["iluvatar.com/gpu=GPU-bbbb"]
5. kubelet 将 "iluvatar.com/gpu=GPU-bbbb" 传递给 containerd
6. containerd 读取 /etc/cdi/iluvatar.json
7. 在 spec 中找到 name=="GPU-bbbb" 的条目（忽略 GPU-aaaa）
8. 按 GPU-bbbb 的 containerEdits 注入 /dev/iluvatar1
```

此时 `/etc/cdi/iluvatar.json` 内容：

```yaml
cdiVersion: 1.1.0
kind: iluvatar.com/gpu
devices:
  - name: GPU-aaaa                    # ← Pod A 的设备（保留）
    containerEdits:
      env: [ILUVATAR_COREX_VISIBLE_DEVICES=all]
      deviceNodes: [{hostPath: /dev/iluvatar0, ...}]
      mounts: [...]
      hooks: [...]
  - name: GPU-bbbb                    # ← Pod B 的设备（新增）
    containerEdits:
      env: [ILUVATAR_COREX_VISIBLE_DEVICES=all]
      deviceNodes: [{hostPath: /dev/iluvatar1, ...}]
      mounts: [...]
      hooks: [...]
```

### 阶段 4：Pod A 退出，Unprepare

```
1. Pod A 被删除
2. kubelet 调用 DRA driver: UnprepareResourceClaims(claim-A)
3. DRA driver:
   a. 从 preparedClaims 中查找 claim-A 的 CDI device IDs: ["iluvatar.com/gpu=GPU-aaaa"]
   b. 提取设备名: ["GPU-aaaa"]
   c. 读取 /etc/cdi/iluvatar.json（含 GPU-aaaa 和 GPU-bbbb）
   d. RemoveDevices(spec, ["GPU-aaaa"]) → 移除 GPU-aaaa
   e. 剩余 [GPU-bbbb]，写回 /etc/cdi/iluvatar.json
4. 清理 preparedClaims 中的 claim-A 记录
```

此时 `/etc/cdi/iluvatar.json` 内容：

```yaml
cdiVersion: 1.1.0
kind: iluvatar.com/gpu
devices:
  - name: GPU-bbbb                    # ← 只剩 Pod B 的设备
    containerEdits: [...]
```

### 阶段 5：Pod B 也退出

```
1. Pod B 被删除
2. UnprepareResourceClaims(claim-B)
3. RemoveDevices 后无设备剩余
4. DeleteSpecFile → 删除 /etc/cdi/iluvatar.json
```

节点恢复到无 CDI spec 文件的状态。

## 为什么合并后能正确工作

### containerd 的 CDI 消费逻辑

containerd 的 CDI 处理流程：

```
kubelet 调用 containerd CreateContainer
  → containerd 收到 CDIDeviceIDs: ["iluvatar.com/gpu=GPU-bbbb"]
  → containerd 扫描 /etc/cdi/*.json
  → 解析每个 JSON 文件，按 kind 索引设备条目
  → 在 iluvatar.com/gpu 的设备列表中查找 name=="GPU-bbbb"
  → 找到后，执行该条目的 containerEdits:
      - bind-mount /dev/iluvatar1
      - bind-mount 驱动库
      - 设置环境变量
      - 执行 prestart hooks
  → 完成设备注入
```

**关键点：containerd 只注入 CDI device ID 指定的那个设备条目，不会注入 spec 文件中的所有设备。**

这意味着：
- spec 文件中的其他设备条目（如 GPU-aaaa）对当前容器完全无影响
- 每个容器只看到自己被分配的设备
- 多个容器共享同一个 spec 文件是安全的

### 类比

```
CDI spec 文件 = 电话簿（列出节点上所有设备的"联系方式"）
CDI device ID = 电话号码（告诉 containerd 查找哪一个条目）

Pod A 拿到号码 "GPU-aaaa" → 在电话簿中查到 /dev/iluvatar0 的信息
Pod B 拿到号码 "GPU-bbbb" → 在电话簿中查到 /dev/iluvatar1 的信息

Pod A 不会看到 Pod B 的设备，反之亦然。
电话簿里有多个条目不影响查找的精确性。
```

## 合并策略详解

### MergeDevices 逻辑

```go
func MergeDevices(existing *Spec, newDevices []Device, kind string) *Spec {
    // 1. 如果没有已有 spec，直接用新设备创建
    if existing == nil {
        return &Spec{Kind: kind, Devices: newDevices}
    }
    // 2. 按设备名建立索引
    byName := index existing devices by name
    // 3. 遍历新设备：同名替换，不同名追加
    for each newDevice:
        if name exists in byName → replace at that position
        else → append to end
}
```

### 为什么同名替换是安全的

如果同一个设备被重新 Prepare（例如容器重启后 kubelet 重新调用），新生成的条目会替换旧条目。由于设备的注入信息来自 profile（稳定不变），替换前后内容相同，不会有副作用。但保留替换逻辑是为了应对 profile 更新后的重新 Prepare 场景。

### RemoveDevices 逻辑

```go
func RemoveDevices(spec *Spec, deviceNames []string) *Spec {
    // 1. 建立待删除名称集合
    remove = set(deviceNames)
    // 2. 保留不在删除集合中的设备
    kept = filter spec.Devices where name not in remove
    // 3. 全部删除则返回 nil（触发文件删除）
    if len(kept) == 0 → return nil
    // 4. 否则返回裁剪后的 spec
    return spec with kept devices
}
```

## Prepare/Unprepare 与 CDI spec 的对应关系

```
时间线          preparedClaims (内存)       /etc/cdi/iluvatar.json (磁盘)
─────────────────────────────────────────────────────────────────────
启动            {}                          (不存在)
                ↓
Prepare A       {A: [GPU-aaaa]}             [GPU-aaaa]
                ↓
Prepare B       {A: [GPU-aaaa],             [GPU-aaaa, GPU-bbbb]
                 B: [GPU-bbbb]}
                ↓
Unprepare A     {B: [GPU-bbbb]}             [GPU-bbbb]
                ↓
Unprepare B     {}                          (删除)
```

`preparedClaims` 内存 map 是 CDI spec 文件的索引——它记录每个 claim 对应哪些 CDI device ID，使得 Unprepare 时能精确知道要从 spec 文件中移除哪些条目。

## FAQ

### Q: 两个 Pod 申请同一个设备会怎样？

DRA allocator 不会将同一设备分配给两个 Pod。如果设备已被分配，allocator 会选择其他可用设备或等待。

### Q: Pod A 的容器重启后，CDI spec 还能找到设备吗？

能。除非 Pod A 被 Unprepare（即 Pod 被删除），否则其设备条目始终在 spec 文件中。容器重启时 containerd 会重新读取 spec 并注入设备。

### Q: 为什么不用每个 claim 一个 spec 文件？

CDI 规范中，一个 spec 文件对应一个 kind（如 `iluvatar.com/gpu`）。如果每个 claim 写一个文件，containerd 会尝试解析所有文件中的同 kind 设备，可能导致设备名冲突。单一 spec 文件 + 合并是 CDI 规范推荐的做法。

### Q: ldconfig hook 会被执行多次吗？

是的，如果 spec 中有多个设备条目，每个条目都有 ldconfig hook，containerd 会对每个条目执行一次。但 ldconfig 是幂等的（多次执行结果相同），所以不会有副作用。
