# 参与开发

Cage 的主要开发环境是 Apple Silicon macOS 14+。用户安装预构建 App 即可使用；本页面向需要修改工作台、控制服务或虚拟机底座的贡献者。

## 从源码构建

需要 Go 1.26.10 或更新版本、Node.js 24、`uv` 和 Xcode Command Line Tools。Go 版本以 `go.mod` 为准，前端依赖使用仓库中的 npm 锁文件，Python 工具统一通过 `uv` 运行。

```sh
git clone https://github.com/Lee-xeo/Claude-Cage.git
cd Claude-Cage
make app
```

构建会下载并校验固定版本的 Mihomo，构建前端和嵌入式客机组件，再生成 `_output/Cage.app`。未指定签名身份的本地开发构建使用 ad-hoc 签名；不能将其视为可通过 Gatekeeper 的正式发行包。

需要保留日常 App 时，指定独立输出并使用 `make app`：`CAGE_BUILD_OUTPUT="$PWD/_output/validation/Cage.app" make app`。输出必须是绝对 `.app` 路径；此模式不清理其他副本。目标 App 或控制程序仍在运行时，构建会拒绝覆盖。开发构建也不能用于验证正式签名包的自动安装成功路径。

安装到当前用户的 Applications 目录：

```sh
make install-app
```

安装位置为 `~/Applications/Cage.app`。不要在已启动的 App 上直接覆盖构建结果；先保存文件、停止环境并退出。正式发行的签名、公证与安装包构建见[发布 macOS 安装包](docs/releasing.md)。仓库中保留的上游 Nix 和多平台发行配置供参考，不代表 Cage 桌面版支持这些发行目标。

仅修改界面时可运行 `make dev-ui`，但独立前端服务器仍需要控制服务和正确会话，不能用空白页面代替真实 App 验收。`make` 或 `make cage` 构建控制程序及工作台资源，生成 `_output/bin/cage`；完整桌面 App 使用 `make app`。运行 `make help` 查看常用入口。`make install` 保留上游行为，安装的是 agent-vm 命令，不是 Cage App。

## 代码与设计入口

目录归属、生成资源和保留的上游工具见[仓库结构与职责](docs/repository.md)。

| 目录 | 职责 |
|---|---|
| `desktop/` | Swift 原生 Mac 窗口、菜单和系统集成 |
| `ui/` | React 工作台、代理管理、设置和网络状态 |
| `internal/workbench/` | 工作台控制接口、任务、终端和设置 |
| `internal/environment/` | 稳定环境身份、虚拟机命令、客机连接清单和组件版本读取 |
| `internal/projects/` | 项目目录校验、共享授权与共享连接生命周期 |
| `internal/privatefile/` | 各配置模块共用的私有 JSON 原子写入 |
| `internal/cage/` | 强制代理、网络门槛和客机服务 |
| `internal/cage/guest/` | 客机运行脚本、浏览器控制、编辑器辅助程序及其 JavaScript/Python 测试 |
| `internal/managedproxy/` | 订阅、节点、代理核心和私有配置 |
| `third_party/`、`patches/` | 固定的上游源码与补丁 |
| `www/` | 项目主页 |

修改前阅读对应的[工作台设计](docs/design/workbench.md)、[网络检查设计](docs/design/network-checks.md)或[内置代理设计](docs/design/managed-proxy.md)。调整接口、数据归属或安全边界时，同时更新设计与用户说明。

原生界面迁移与新平台适配分开推进。修改目录选择、剪贴板、更新安装，或 VM、终端、进程与路径处理前，先查[平台能力与迁移边界](docs/design/platform-boundaries.md)，保留现有服务的资源所有权和安全限制。

## 验证改动

选择覆盖本次行为的检查。工作台的基础检查：

```sh
npm ci --prefix ui
make check-version test-ui test-guest test-tools
go test -race ./internal/environment ./internal/projects ./internal/workbench ./internal/cage ./internal/managedproxy
make test
```

修改底层网络时，还需执行相关模块测试：

```sh
go test github.com/containers/gvisor-tap-vsock/pkg/services/dns \
  github.com/lima-vm/lima/v2/pkg/hostagent/dns \
  github.com/lima-vm/lima/v2/pkg/networks/usernet
```

修改浏览器启动或控制逻辑时，在 Linux 客机的独立 X 显示中运行 `DISPLAY=:183 node scripts/tests/browser-controls-smoke.mjs`。该检查使用临时配置和真实控制器，验证人工浏览标记、标签与导航、重启后的页面及 Cookie 恢复，并检查未认证请求和跨来源请求仍被拒绝。

修改更新或恢复界面时，运行 `node ui/scripts/recovery-ui-smoke.mjs`；修改画面可见性时，运行 `node ui/scripts/remote-visibility-smoke.mjs`。两者需要可用的 Playwright 和已启动的前端开发服务，可通过 `CAGE_UI_URL` 指定地址；测试使用隔离接口，后者还使用真实 RFB 握手与帧验证停止和恢复，不连接日常 VM。修改原生更新流程时运行 `make test-app-updates`，正式签名、公证及真实安装仍须单独验收。组件清单、固定工具安装器和网络续租回归已包含在上述 Go 包测试中，无需重复执行一套独立命令。

修改上游源码时，保持 `patches/` 与 `third_party/` 一致，并按 `patches/README.md` 核对同步方式。UI 改动需要在实际 App 中验证状态、失败恢复和关键交互；编译通过不能证明登录、联网或用户操作已成功。

建议使用独立测试配置和非敏感测试项目，避免将开发验证写入日常环境。启动前的 Git 保护检查属于主机状态：单独指定 Cage 配置目录不会隔离全局 Git 设置，测试修改路径时应使用隔离的测试用户目录。应用必须先解释影响，用户明确确认后才将 `safe.bareRepository` 设置为 `explicit`，并在复检通过后启动环境；读取状态不得修改 Git 配置。真实网络验证应区分代理连通、环境放行、浏览器访问、Claude 授权与模型请求，不能互相替代。

验证服务不依赖 React 或 Swift 窗口时，运行 `make test-headless HEADLESS_UPSTREAM=socks5://127.0.0.1:<已授权端口>`。它使用真实 VM 和全新的配置、用户目录与项目，覆盖启动、共享、终端断开重连、编辑器、控制服务重启、停止和退出；准备条件、报告与失败处理见[无窗口完整验收](docs/headless-acceptance.md)。

## 开发诊断

配置与状态默认在 `~/.claude-cage/`。`CLAUDE_CAGE_CONFIG` 可以指向另一份私有配置，其父目录拥有独立状态。配置文件权限为 `0600`；不要把该目录作为共享项目或提交到 Git。

`environment.json` 固定记录环境的 VM 目录，`projects.json` 保存明确打开的共享目录，`workbench.json` 保存当前项目与偏好。选择项目不会改变 VM 身份。浏览器配置在客机的 `/home/cage-browser/profile`，受管工具的最低版本与包摘要见 `internal/cage/guest/tool-versions.env`。替换 App、同步组件和升级工具是不同操作，契约见[工作台设计](docs/design/workbench.md#持久环境的组件与工具版本)。

```sh
make proxy-core
make cage
./_output/bin/cage check   # 检查配置的代理出口
./_output/bin/cage shell   # 打开共享 Cage 环境中的 shell
./_output/bin/cage stop    # 停止该环境
./_output/bin/cage desktop # 在本机浏览器打开一次性入口
```

`cage access`、独立 VNC 和编辑器端口仅供诊断，输出可能包含访问凭据，不要直接上传。普通用户通过 App 或 `cage desktop` 自动建立会话，无需输入内部访问密码。

代理节点统一通过 Cage 内的订阅、本地 YAML 或手动节点入口配置。运行时只使用 App 随附核心。来源、运行配置和日志位于私有配置的 `proxy/` 子目录，通过独立用户 launchd 任务运行；来源与凭据文件权限为 `0600`。`config.json` 中的代理端点和版本由节点管理器生成，通用设置及恢复操作不能覆盖。

## 维护第三方许可

更改 Go 依赖或固定第三方源码后，运行：

```sh
make licenses        # 更新根目录 THIRD_PARTY_LICENSES
make check-licenses  # 只检查，不修改文件
```

检查生成文件的差异并随依赖改动提交。`make app`、`make install-app` 和完整的 `make package-app` 构建会在下载代理核心、编译和替换 App 前检查 Go 许可；过期时停止，不自动改写已提交文件。直接调用底层构建脚本也执行相同检查。

`NOTICE` 人工维护项目来源、组件归属及修改说明；`THIRD_PARTY_LICENSES` 由 Go 依赖收集脚本生成。前端运行依赖、noVNC 的许可与修改后源码，以及 Mihomo 的许可材料，在 App 构建时自动收集到 `Contents/Resources/licenses/`。

## 提交改进

提交说明应包含用户遇到的具体问题、改动后的行为、验证方式及尚未覆盖的情形。保留与本次任务无关的在途改动，不将本机路径、账号、代理订阅、密钥、构建产物或私人截图提交到仓库。

新增依赖需保留许可和来源。Cage 主许可为 MIT，随附组件有各自许可；更新 Mihomo 等独立程序时同时更新固定版本、校验值、来源与发行包中的许可资料。
