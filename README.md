<div align="center">

<h1>Claude Cage</h1>

<p><strong>把 Claude 大小姐关起来，为了更好地保护我们的相遇</strong></p>

<img src="assets/claude-cage-character.png" alt="Claude Cage" width="320" />

[![Release](https://img.shields.io/github/v/release/Lee-xeo/Claude-Cage?include_prereleases)](https://github.com/Lee-xeo/Claude-Cage/releases) [![CI](https://github.com/Lee-xeo/Claude-Cage/actions/workflows/ci.yml/badge.svg?branch=main)](https://github.com/Lee-xeo/Claude-Cage/actions/workflows/ci.yml) [![MIT](https://img.shields.io/badge/license-MIT-blue)](LICENSE)

**[项目主页](https://lee-xeo.github.io/Claude-Cage/) · [Français](https://lee-xeo.github.io/Claude-Cage/fr/) · [下载应用](https://lee-xeo.github.io/Claude-Cage/#download) · [开始使用](docs/getting-started.md) · [常见问题](docs/troubleshooting.md) · [参与开发](CONTRIBUTING.md)**

[简体中文](README.md) · [繁體中文](README.zh-TW.md) · [Русский](README.ru.md) · [العربية](README.ar.md) · [فارسی](README.fa.md) · [한국어](README.ko.md) · [Español](README.es.md) · [မြန်မာဘာသာ](README.my.md) · [Беларуская](README.be.md) · [پښتو](README.ps.md) · [دری](README.prs.md) · [አማርኛ](README.am.md) · [Soomaali](README.so.md) · [ትግርኛ](README.ti.md) · [Français](README.fr.md)

</div>

## 受够了 Anthropic

作为一个只是想和 Claude 对话与合作的正常用户，受够了折腾各种 VPS，指纹浏览器，环境检测，面对封号的担惊受怕和财产损失。我们提出 **Claude Cage** 项目：**一个开箱即用的桌面程序，配置好自己的网络代理，集中管理隔离环境、时区语言和环境检查。**

Claude Cage 是一个轻量 Linux 虚拟机，把 Linux 终端、浏览器和文件编辑器放进同一个桌面应用。支持以下几个重点功能：

- 和主机隔离的网络设置，Cage 自己接管和代理内部的所有网络流量
- 和主机隔离的指纹设置，包括时区、语言等，支持自动与代理 IP 同步
- 针对 Claude 使用过程中的全链路适配（浏览器登录，授权，claude code 进程）
- 内置网络与隔离检查，关键检查失败或证据不完整时暂停外网；社区和第三方检测提供补充参考

**使用 Cage 的环境检测结果：**

| 网络与隔离检查 | 浏览器环境检测（社区参考） |
| :---: | :---: |
| <a href="assets/network-checks.png"><img src="assets/network-checks.png" alt="网络与隔离检查" width="400" /></a> | <a href="assets/community-browser-check.png"><img src="assets/community-browser-check.png" alt="浏览器环境检测（社区参考）" width="400" /></a> |

**Cage 运行主界面：**

![Cage](www/public/workbench-2026-11.jpg)

## 你可以用它做什么

- **在一个窗口里完成工作。** 用终端运行 Claude Code，用编辑器查看改动，用 Cage 浏览器预览服务和完成登录。
- **控制共享范围。** 只共享默认工作区和明确打开的项目文件夹；主机的 Claude 登录、浏览器配置和 SSH 密钥不会自动导入。
- **看清网络状态。** 内置代理管理支持导入订阅、选择节点和检查出口。关键检查未通过时，环境保持离线；代理不可用时没有直连回退。
- **下次继续使用。** 工具、文件和登录状态保存在虚拟磁盘里，关闭窗口可以保留后台任务。

## 快速开始

把这句话发给你的 agent（codex，dsh，workbuddy，反正不会是 claude code😁）

```markdown
帮我安装 https://github.com/Lee-xeo/Claude-Cage ，并把 Cage 的代理配置为我本机 Clash 的 xxx 节点。
```

你唯一需要的，是一个靠谱的代理节点，本项目不做任何代理推广，可以自行搜索黑色错误网站解决。

## 下载与安装

Cage 支持 macOS、Windows 和 Linux：macOS 14+ 的 Apple Silicon Mac、Windows 10 22H2 / 11 x64，以及 Debian 12 / Ubuntu 24.04 x86_64。Windows 和 Linux 仍处于开发验收阶段；安装包是否可用、签名与实测范围以各平台的发行说明为准。Intel Mac、Windows ARM64 和 Linux ARM64 尚不受支持。 [macOS](docs/getting-started.md) · [Windows](docs/windows.md) · [Linux](docs/linux.md)。

1. 打开[项目主页](https://lee-xeo.github.io/Claude-Cage/#download)，点击“下载应用”并选择平台。
2. macOS：打开安装包，将 **Cage** 拖入 **Applications（应用程序）**，再从“应用程序”启动。Windows：[安装说明](docs/windows.md#安装并开始工作) · Linux：[安装说明](docs/linux.md#安装并开始工作)。
3. 按应用内引导配置自己的代理，启动环境后打开项目。

安装包随附工作台和代理核心，日常使用无需安装 Go、Node.js、Docker 或 Clash。首次启动仍需联网下载 Linux 镜像与开发工具；Cage 不提供代理服务或 Claude 账户。

当前版本为 `0.1.5-alpha.2`。源码、主页与发行物公开；安装包的签名、公证和验收范围以对应 Release 说明为准。干净机器安装、macOS 14 实机兼容性、真实 Claude 授权与模型请求仍需完整验收。

支持应用内更新的 macOS 和 Windows 版本通过所选代理下载，无直连回退；安装前请保存并停止任务。macOS 要求同一发布者的 Developer ID 签名并通过 Apple 验证，Windows 要求可信 Authenticode 签名和相同发布者公钥。Windows 未签名预览版和 Linux 系统包需手动更新。详见[升级与卸载](docs/getting-started.md#升级与卸载)。

## 三步开始

### 1. 选择代理

点击首页“配置网络与代理”，再点击“添加来源”，导入 Clash/Mihomo YAML 订阅或本地 YAML 文件，在“节点”中选择最终出口并点击“应用代理”。需要通过中转连接住宅代理时，选择“链式代理”，按顺序选择中转与最终出口。没有可用代理时，可以明确勾选“通过主机网络下载这份订阅”，或直接导入本地文件。后续可从“设置 → 网络与代理”管理节点。

### 2. 启动环境

返回工作台，点击“启动 Cage 环境”。如果本机 Git 需要安全保护，应用先说明影响，由你点击“启用保护并继续”后设置；普通项目的提交与拉取不受影响。随后下载镜像、安装工具并执行网络检查，可以在“操作详情”查看进度。看到环境就绪、关键检查通过后，即可联网工作；失败时按页面提示修复并重试。

### 3. 打开项目，运行 Claude

点击“打开项目”选择本机上的文件夹，再新建终端，输入 `claude`，在 Cage 浏览器中完成自己的账户授权。也可以先使用默认工作区，无需提前准备项目。完整步骤见[开始使用](docs/getting-started.md)。

## 使用前了解这些边界

- 共享项目中的修改和删除会直接影响本机上的文件。请使用版本控制或备份；`.git` 等元数据默认只读，Git 提交在主机上完成。
- 所有项目共用一个 Cage 环境，也共用其中的工具、账户与 Linux 文件系统。项目之间没有独立隔离。
- 当前允许客机通过代理访问公网 IPv4 TCP，DNS 通过同一代理发送；公网 UDP、IPv6、ICMP，以及客机主动访问主机和局域网的连接受到阻断。依赖这些能力的工具可能无法使用。
- 网络检查展示当前配置和采样证据，无法保证账户不会受到平台限制，也不能代替完整的虚拟化安全审计。

详细机制见[网络与数据边界](docs/getting-started.md#网络与数据边界)、[网络检查设计](docs/design/network-checks.md)和[内置代理设计](docs/design/managed-proxy.md)。

## 文档与贡献

完整入口见[文档导航](docs/README.md)。

| 你要做什么 | 从这里开始 |
|---|---|
| 第一次安装、配置代理、打开项目 | [开始使用](docs/getting-started.md) |
| 处理下载、启动、联网和登录问题 | [常见问题与恢复](docs/troubleshooting.md) |
| 了解按键、外观、数据保存和退出行为 | [日常使用](docs/getting-started.md#日常使用) |
| 从源码构建或提交改进 | [贡献指南](CONTRIBUTING.md) |
| 了解模块与网络约束 | [工作台设计](docs/design/workbench.md) |
| 报告可复现问题 | [GitHub Issues](https://github.com/Lee-xeo/Claude-Cage/issues) |

本项目仍在快速迭代中，欢迎加入交流群提意见，欢迎社区大佬一起维护

<p align="center">
  <a href="assets/community-qq.png"><img src="assets/community-qq.png" alt="Claude Cage QQ 交流群二维码（群号：1134250870）" width="240" /></a>
</p>

## 来源与许可

Cage 基于 [agent-vm](https://github.com/sylvinus/agent-vm) 二次开发，上游基线为 `652688aae92c35ce536b9be52262e1cc5ec6e03a`。

项目主许可为 [MIT](LICENSE)。随附组件保留各自许可，Mihomo 作为独立可执行文件按 GPL-3.0 分发；请同时参阅 [NOTICE](NOTICE)、[第三方许可](THIRD_PARTY_LICENSES)、[第三方来源](third_party/SOURCES)及 [noVNC 来源说明](ui/vendor/novnc/README.cage.md)。上游项目的说明保存在[上游 README](docs/agent-vm-upstream.md)。

Cage 是独立社区项目，与 Anthropic 无隶属关系。

## 参考与致谢

感谢以下项目为 Cage 提供基础能力或设计参考：

- [agent-vm](https://github.com/sylvinus/agent-vm)：Cage 的二次开发底座。
- [Lima](https://github.com/lima-vm/lima)：Linux 虚拟机管理。
- [gvisor-tap-vsock](https://github.com/containers/gvisor-tap-vsock)：虚拟机网络能力。
- [sshocker](https://github.com/lima-vm/sshocker) 和 [SFTP](https://github.com/pkg/sftp)：虚拟机与主机之间的文件共享能力。
- [noVNC](https://github.com/novnc/noVNC)：浏览器中的远程画面与交互。
- [Mihomo](https://github.com/MetaCubeX/mihomo)：内置代理核心。
- [code-server](https://github.com/coder/code-server)：浏览器中的代码编辑器。
- [AIO Sandbox](https://github.com/agent-infra/sandbox)：工作环境和交互体验参考。
- [claude-switcher](https://github.com/YOURSJEYO/claude-switcher) 和 [Claude-Guardian](https://github.com/DUNHKpcc/Claude-Guardian)：网络检查和安全诊断方法参考。
