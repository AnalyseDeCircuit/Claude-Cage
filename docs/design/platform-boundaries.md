# 平台能力与原生界面迁移边界

迁移原生 Go 界面时，保留现有控制服务及其业务规则，替换窗口、绘制和系统交互。当前 Cage 桌面交付面向 Apple Silicon macOS 14+；Linux、Windows 的完整 Cage 应用尚未实现。仓库中的上游驱动和多平台工具不等于 Cage 已支持对应平台。

平台能力分为两处：**桌面宿主**负责用户与操作系统的交互，**服务运行环境**负责 VM、进程、终端和文件权限。换掉 Swift 与 React，只解决前一部分及界面绘制，不会自动替换后者。

```text
桌面宿主：窗口、目录选择、剪贴板、菜单、更新安装
    │ 用户操作、服务入口与状态
    ▼
Go 控制服务：环境与项目流程、作业、会话、设置、网络授权
    │
    ├── VM / SSH / PTY / 项目共享
    └── 后台进程 / 文件锁 / 私有文件 / 代理核心
```

## 桌面能力不进入环境服务

当前实现集中在 [Cage.swift](../../desktop/Cage.swift)。[desktop.js](../../ui/src/desktop.js) 是工作台调用当前 WebKit 桥接的入口；[api.js](../../ui/src/api.js) 管理认证后的 Go 服务请求，仅在会话续期时调用桌面宿主。原有桌面函数导出保留，现有界面调用改为直接引用 `desktop.js`。

| 能力 | 当前实现与调用 | 迁移时保留的约定 |
|---|---|---|
| 选择项目目录 | `chooseFolder` → `NSOpenPanel`；取消返回空字符串 | 选择器只返回路径，不授予共享权限；仍由 Go 服务校验并记录项目授权 |
| 本机剪贴板 | `clipboardRead` / `clipboardWrite` → `NSPasteboard`；普通浏览器沿用 `navigator.clipboard` | 保留文字与用户手势约束；客机的后台剪贴板通知不能自行覆盖主机剪贴板 |
| 窗口与外观 | `NSWindow`、`NSVisualEffectView`、菜单和 `appearance`；窗口偏好存入 `UserDefaults` | 主题值继续由现有外观配置提供；窗口尺寸、透明效果和菜单由新宿主实现，不加入环境状态 |
| 显示可见性 | 宿主发布 `cage-window-visibility`；前端结合页面可见性决定是否接收画面 | 隐藏、最小化和关闭窗口暂停显示，不结束 VM、终端或作业；重新打开连接原服务 |
| 菜单与编辑操作 | `cageNativeAction`、`cageEditAction` 将菜单动作交给当前视图，必要时使用系统编辑行为 | 区分本机表单、终端与客机网页的输入目标；原生界面不必保留这些 JavaScript 回调名称 |
| 启动与会话恢复 | Swift 调用 `cage desktop --json`；`refreshSession` / `reconnect` 取得或重新加载认证入口 | 复用控制服务身份核对与一次性入口，不从页面接收任意可执行文件或控制地址 |
| 退出应用 | 现有确认与停止流程完成后调用 `quitNow` | 关闭窗口、停止环境、退出控制服务仍是三种操作；桌面退出不能任意杀死服务进程 |
| 应用更新 | `appUpdate`、`workbenchReady` 与 `cage-app-update` | 下载、验证、安装和启动确认的职责分开，见下节；新窗口必须在服务与界面初始化成功后确认启动 |

WebKit 桥接只接受预期本机端口的主框架请求，主窗口只加载该本机控制地址。客机网页不直接获得主机目录、剪贴板或更新安装权限。改用原生 Go 界面后，系统能力由新宿主直接调用，无需为了复用业务保留 WebKit。

### 更新安装是独立的平台能力

- [internal/appupdate/update.go](../../internal/appupdate/update.go) 负责通过受管出口发现、下载并校验发行附件摘要。目前明确选择 `Cage-<版本>-macos-arm64.dmg`，保存为私有目录中的 `Cage.dmg`，还不是多平台更新选择器。
- [Updates.swift](../../desktop/Updates.swift) 负责 Apple 签名、发布者身份、架构与 Gatekeeper 检查，以及磁盘映像挂载和应用准备；使用 Security、`lipo`、`spctl`、`hdiutil` 和 `ditto`。
- [UpdateInstaller.swift](../../desktop/UpdateInstaller.swift) 负责应用替换授权、安装、重新启动、启动确认与失败恢复。停止与后台接管仍复用 `cage prepare-update --json`，不在安装器中重写环境停止流程。

先迁移 macOS 窗口时可以保留现有安装助手，但需要处理好它与新 App 的打包及启动确认。面向另一平台时，再实现该平台的发行附件、身份校验、安装和恢复；不能仅更换文件后缀或跳过验证。

## Go 服务的实际平台依赖

服务命令不依赖 React、Swift 或 WebKit。以下运行能力仍有明确的平台条件，继续由各自模块持有，不集中套上一个通用“平台接口”。

| 能力与所属模块 | 当前依赖 | 后续适配范围 |
|---|---|---|
| VM 后端：`internal/vm/`、`internal/limaembed/` | 已有 `vm.Backend` 与 Lima 实现；仓库注册 Darwin VZ、QEMU，以及 Windows WSL2 / HCS 驱动 | 复用已有后端边界，不新增同义接口。目标后端必须实际满足启动、共享与网络隔离要求，驱动存在不代表整套 Cage 可运行 |
| 环境连接：`internal/environment/connection.go` | `/usr/bin/ssh`、固定 Unix 搜索路径、受限 SSH 配置和 Linux 客机命令 | SSH 路径与进程启动方式需要对应宿主实现；保留禁止本机命令、代理转发与额外端口转发等限制 |
| 主机终端：`internal/terminal/` | `github.com/creack/pty` 的主机 PTY、尺寸控制和子进程回收 | 当前依赖在 Windows 的 `StartWithSize` 返回 `ErrUnsupported`。Windows 需要实际终端实现；更换 xterm.js 绘制不会解决主机 PTY 问题 |
| 控制进程：`workbench/desktop.go`、`lifecycle.go`、`server.go` | `Setsid` 脱离启动器，`SIGTERM` / `SIGINT` 退出，`Flock` 独占配置与等待接管 | 后台存活、取消、进程回收和独占锁的合同保留；Windows 需要对应进程与锁实现 |
| 进程存活与旧后台身份 | `environment/identity.go`、`cage/profile.go` 使用信号 0；`control_lifecycle.go` 使用 `Kill(pid, 0)`；`control_legacy.go` 仅在 macOS 核对 `ps` / `lsof` 身份 | PID 记录不是终止进程的授权。保留身份核对、空闲接管与锁释放条件，不用强制杀进程代替正常退出 |
| 代理后台：`internal/managedproxy/` | `service_darwin.go` 使用用户 `launchd` 域、私有 plist 与日志；`service_other.go` 保留原有不支持提示；核心下载脚本只提供 Darwin 文件 | 更换宿主的代理进程管理与核心分发；不改变节点、暂停意图、授权和续租状态机。未安装服务时的停止仍为空操作 |
| 物理出口识别：`managedproxy/interface.go` | 明确只支持 macOS，使用 `networksetup`、`route`、`scutil` | 目标平台需要真实物理网卡识别；不能回退到未绑定出口来获得表面可用性 |
| 本机控制通道：`managedproxy/core.go`、`cage/gate_control.go` | Unix socket；网络门槛控制还在 `/tmp` 创建私有目录 | 地址、权限与传输方式需随宿主验证，继续防止客机访问主机控制通道 |
| 系统打开工作台：`cmd/cage/main.go` | macOS 使用 `open`，其他系统当前走 `xdg-open` | 这是现有命令选择，不是完整 Windows 支持；新宿主可直接使用已返回的认证入口 |

[service_darwin.go](../../internal/managedproxy/service_darwin.go) 只包含现有 macOS 进程实现；[service_other.go](../../internal/managedproxy/service_other.go) 延续原有拒绝启动的行为，不启动临时替代进程。代理配置生成、API 调用与业务状态仍留在原包。

工作台中的 Unix 进程与锁调用尚无 Windows 实现，完整控制程序的 Windows 构建路径也需要适配；这部分不由桌面界面框架提供。

### 主机路径与客机路径需要分别看待

[projects/validation.go](../../internal/projects/validation.go) 负责解析真实主机目录并排除私有配置、主目录和版本控制元数据。[projects/shares.go](../../internal/projects/shares.go) 将授权目录通过受限 SFTP 与客机 SSHFS 连接，并验证客机读取到主机创建的随机内容。

当前目录契约是：**主机目录与 Linux 客机挂载点同名**。项目共享、编辑器打开和终端进入目录均使用同一个路径字符串。迁移 macOS 界面时保持这个契约；扩展 Windows 时，需要先明确主机路径到客机挂载点的映射，不能把 `C:\...` 直接作为 Linux 路径使用。仅使用 `filepath` 或替换斜杠，也没有处理盘符根目录、UNC、大小写和重解析点的授权边界。

[privatefile/json.go](../../internal/privatefile/json.go) 使用同目录临时文件、`0600` 文件权限、`0700` 目录权限和重命名发布私有配置。新宿主平台需要验证对应的访问控制、覆盖写入与失败处理；这些数字不能代替 Windows ACL。项目授权数据与私有控制文件继续分开，不为迁移改变原有配置含义。

客机继续运行 Linux，客机中的 Bash、systemd、Chromium、编辑器和 SSHFS 属于客机资源，不因更换主机界面而改写为主机原生程序。

## 后续迁移按这两条路线推进

**先更换 macOS 界面**：复用现有 Go 服务、VM、代理后台和配置，先替换目录选择、剪贴板、窗口、菜单和绘制；明确更新安装助手与新宿主的调用方式。用同一套服务命令和认证接口验收，不另建一套环境或会话状态。

**扩展到其他宿主平台**：分别落实 VM 与隔离网络、代理进程与出口识别、SSH 与 PTY、进程与文件锁、路径与权限、桌面集成和发行更新，再执行完整环境验收。现有 `vm.Backend`、模块入口与控制协议优先复用；实际替换某项能力时，再添加对应实现与必要接口，不预建空壳。

当前这一轮只明确职责、分开桌面桥接与已有代理平台实现，不实现新的宿主平台，也不改变网络状态机。平台支持最终以目标系统上真实的启动、共享、终端、联网隔离、停止和更新恢复结果为准。
