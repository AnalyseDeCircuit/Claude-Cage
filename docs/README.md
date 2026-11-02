# Cage 文档入口

首次使用 Cage，从安装与代理配置开始；已有环境遇到问题时，直接查故障恢复。公开安装包与版本验收范围见 [GitHub Releases](https://github.com/Lee-xeo/Claude-Cage/releases)，产品介绍见[项目主页](https://lee-xeo.github.io/Claude-Cage/)。

## 使用与反馈

| 你要做什么 | 从这里开始 |
|---|---|
| 安装、配置代理、打开第一个项目 | [开始使用](getting-started.md) |
| 查看快捷键、文件保存、关闭与退出行为 | [日常使用](getting-started.md#日常使用) |
| 升级应用、更新环境工具或卸载 | [升级与卸载](getting-started.md#升级与卸载) |
| 查看首个内测版本的能力与验收范围 | [0.1.0-alpha.1 发行说明](releases/0.1.0-alpha.1.md) |
| 排查下载、启动、联网与授权问题 | [常见问题与恢复](troubleshooting.md) |
| 报告隔离或凭据安全问题 | [安全报告](../SECURITY.md) |

## 开发与发布

| 你要做什么 | 从这里开始 |
|---|---|
| 从源码构建、修改和验证代码 | [参与开发](../CONTRIBUTING.md) |
| 找到代码、构建入口与上游资料 | [仓库结构与职责](repository.md) |
| 理解工作台、浏览器与后台的职责 | [工作台设计](design/workbench.md) |
| 区分原生界面迁移与宿主跨平台工作 | [平台能力与迁移边界](design/platform-boundaries.md) |
| 使用真实 VM 检查服务的无窗口完整流程 | [无窗口完整验收](headless-acceptance.md) |
| 理解网络检查、暂停与恢复 | [网络检查设计](design/network-checks.md) |
| 理解节点配置、代理核心与凭据归属 | [内置代理设计](design/managed-proxy.md) |
| 构建签名、公证的 macOS 安装包 | [发布 macOS 安装包](releasing.md) |
| 发布 Release、验证公开附件与部署主页 | [公开发行与主页部署](public-releases.md) |

## 上游与许可

Cage 基于 agent-vm 构建。[上游 README](agent-vm-upstream.md)、[上游版本历史](upstream/CHANGELOG.md)、[上游发布流程](upstream/RELEASING.md)和[上游待办](upstream/BACKLOG.md)保留其原有语境，不代表 Cage 的平台支持或产品路线。上游版本历史、发布流程与待办统一放在 `docs/upstream/`。

依赖来源与许可分别见 [NOTICE](../NOTICE)、[第三方许可](../THIRD_PARTY_LICENSES)、[固定上游来源](../third_party/SOURCES)和[补丁说明](../patches/README.md)。
