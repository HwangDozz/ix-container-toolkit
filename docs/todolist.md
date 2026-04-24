仓库状态

  - 仓库：/Users/huangshiyi/Documents/project/ix-container-toolkit
  - 分支：abstract
  - 远端：git@github.com:HwangDozz/ix-container-toolkit.git
  - 已先把之前本地领先的 2 个提交推到远端：
      - 0cbd427 Document NVIDIA xpu-runtime integration research
      - 9ec0b3a Add NVIDIA A100 delegate runtime profile
  - 310P 卡有人在用，本轮明确决定：暂时不碰 profiles/ascend-310p.yaml，不做现场验证，不操作集群。

  本轮新做的工作

  目标：不依赖硬件，推进 profile -> CDI spec 生成后端原型。

  新增/修改文件：

  - pkg/profile/cdi.go
      - 新增 RenderCDISpecYAML(deviceName string)
      - 生成 CDI YAML 原型：
          - cdiVersion: 1.1.0
          - kind: <kubernetes.resourceNames[0]>
          - 默认 device name 为 all
          - deviceNodes 来自 device-nodes artifact
          - mounts 来自非 device artifact
          - env 来自 inject.extraEnv
          - profile 支持 all selector 时额外注入第一个 selector env，例如 METAX_VISIBLE_DEVICES=all
  - cmd/accelerator-profile-render/main.go
      - 新增 CLI 子命令：
          - accelerator-profile-render cdi --profile <profile>
          - 可选：--device-name <name>，默认 all
  - Makefile
      - 新增 target：
          - make render-cdi PROFILE=profiles/iluvatar-bi-v150.yaml
  - pkg/profile/profile_test.go
      - 新增 TestRenderCDISpecYAML
      - 使用 profiles/metax-c500.yaml 验证 CDI 输出包含 version、kind、device name、selector env、extra env、device node、mount options
  - 文档更新：
      - docs/generic-profile-schema.md
      - docs/runtime-architecture.md
      - docs/project-status.md

  已验证

  命令：

  env GOCACHE=/tmp/ix-go-build-cache go test ./...

  结果：通过。

  命令：

  env GOCACHE=/tmp/ix-go-build-cache go run ./cmd/accelerator-profile-render cdi --profile profiles/metax-c500.yaml

  结果：能输出 CDI YAML。

  命令：

  env GOCACHE=/tmp/ix-go-build-cache make render-cdi PROFILE=profiles/iluvatar-bi-v150.yaml

  结果：能输出 CDI YAML。

  当前 CDI 原型限制

  已写入文档：

  - 不访问宿主机
  - 不展开 glob，例如 /dev/dri/card*
  - 不按单卡生成多个 CDI device
  - 不执行 ldconfig
  - 不写 linker 配置文件
  - 不能完整表达 hook backend 里的 so-only 文件过滤语义

  参考 spec

  CDI spec 参考：

  https://github.com/cncf-tags/container-device-interface/blob/main/SPEC.md

  本轮使用结构：

  cdiVersion: 1.1.0
  kind: vendor.com/device
  devices:
    - name: all
      containerEdits:
        env: []
        deviceNodes: []
        mounts: []

  下一步建议

  最自然的下一步是：

  Add node-local CDI generator

  目标：

  - 在节点上读取 active profile
  - 展开 profile 里的 device globs，例如 /dev/dri/card*
  - 展开或筛选 driver artifact 路径
  - 生成真正可被 runtime 消费的具体 CDI spec 文件
  - 输出路径可以考虑 /etc/cdi/<profile-name>.yaml
  - 先用 Metax 或 Iluvatar profile 做本地单元测试/fixture，不碰 310P

  可拆成：

  1. 增加 CDI spec 写文件能力，支持 --output
  2. 增加 host path glob 展开逻辑
  3. 对 device-nodes 生成具体 device nodes
  4. 暂时把 shared-library/directory artifact 作为只读 bind mount
  5. 后续再决定 so-only 是否需要 profile 层新增更精细 IR，或者继续保留 hook backend 处理