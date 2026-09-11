# dsh-desktop

> DeepSeek Harness(`dsh web`)的 **Windows 桌面壳**(Go + WebView2)——把 DSH Web UI 装进原生窗口,
> 而不是丢进浏览器标签页。可作 DSH 插件 bundle 发布,在插件市场(dshmarket)被发现,通过
> `dsh plugin` 或 npm 一键安装、更新。

[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)
[![Go](https://img.shields.io/badge/Go-1.21+-00ADD8?style=flat&logo=go)](go.mod)
[![Platform](https://img.shields.io/badge/Platform-Windows%20x64-0078D4?style=flat&logo=windows)](README.md)
[![GitHub release](https://img.shields.io/github/v/release/andykwokatgithub/dsh-desktop)](https://github.com/andykwokatgithub/dsh-desktop/releases)
[![npm](https://img.shields.io/npm/v/@andykwok/dsh-desktop)](https://www.npmjs.com/package/@andykwok/dsh-desktop)

---

## 演示

<!-- TODO: 请替换为真实运行截图(需要在能拉起 dsh web 的环境下截取 1-2 张,建议 1280px 宽)。
     素材放 assets/,例如 assets/screenshot-main.png,并用相对路径引用。 -->
> 占位:主窗口加载 DSH Web UI、加载态/错误态示意。

---

## 是什么 / 为什么

`dsh-desktop` 是 `dsh web`(默认 `http://127.0.0.1:3080`)的**壳**,只做这几件事:

- **内嵌渲染**:WebView2 加载 DSH Web UI。
- **服务生命周期**:探测并静默启动/复用/停止 `dsh web` 后端。
- **实例认人**:识别端口占用者(监听 PID / 镜像路径 / 命令行),只复用与停止**本壳自己启动**的实例。
- **单实例防重**:重复启动时激活已有窗口并退出。
- **打包体验**:console 子系统构建(GUI 启动时隐藏控制台)+ DeepSeek 图标 + 内嵌 loading/error/token 页。

**为什么值得用**:相比直接 `dsh web` 在浏览器开标签页,本壳给你一个独立的原生窗口、
单实例(不会重复弹窗/堆积标签页)、默认复用已在跑的服务(二次启动更快),且 GUI 启动时不闪黑色控制台。
同时它对**不属于自己的进程零伤害**:端口被占不会去"抢"或杀掉占用者,而是换端口;关窗时也只停
归属可证的实例(见[实例识别与安全边界](#实例识别与安全边界))。

**边界**:它**不承载任何 Harness 逻辑**——`/api` 的执行/读写库、workflow 等能力全部由 `dsh`
负责,本壳只负责把 Web UI 包进 Windows 窗口。

需求与设计见[产品 PRD](docs/dsh-desktop-prd.md)(V1.2)与[技术设计](docs/dsh-desktop-technical-design.md)。

---

## 目录

- [环境要求](#环境要求)
- [安装与更新](#安装与更新)
- [快速上手](#快速上手)
- [配置](#配置)
- [实例识别与安全边界](#实例识别与安全边界)
- [构建](#构建)
- [目录结构](#目录结构)
- [贡献与安全](#贡献与安全)
- [许可与商标](#许可与商标)
- [文档](#文档)

---

## 环境要求

| 场景 | 需要什么 |
| :-- | :-- |
| **终端用户**(下载 EXE) | Windows 10 (1803+) / Windows 11;WebView2 Evergreen 运行时(Win11 预装,Win10 需装) |
| **运行时**(任何场景) | 全局安装 `dsh`(`npm i -g @deepseek-ai/dsh`),且 `node`、`dsh` 均在 `PATH` |
| **构建者**(从源码) | 额外需要 Go 1.21+(含 CGO)+ GCC/mingw-w64;重建图标时才需要 Node + `sharp` |

---

## 安装与更新

### 1. GitHub Releases 直接下载(终端用户推荐)

到 [Releases](https://github.com/andykwokatgithub/dsh-desktop/releases) 下载
`dsh-desktop-win-x64.exe`,双击即可。该 EXE 内置自更新:

```powershell
.\dsh-desktop.exe --check-update   # 检查是否有新版本(输出到控制台)
.\dsh-desktop.exe --update         # 下载并应用,退出(不弹窗体;请重新运行以使用新版本)
```

> 说明:这两个命令是纯 CLI 命令,结果直接输出到**控制台**(stdout/stderr),并同时写入
> `%LOCALAPPDATA%\dsh-desktop\dsh-desktop.log`,不再弹出原生提示框,也不弹窗体。
>
> **控制台同步**:该 EXE 用**控制台子系统**构建(不再用 `-H=windowsgui`),所以 `--version`/
> `--check-update`/`--update` 在 PowerShell/cmd 中会像普通命令一样**同步等待并内联输出**,无副作用。
>
> **GUI 启动**:不带这些 flag 启动时,程序会 `ShowWindow(SW_HIDE)` 隐藏双击产生的控制台窗口
> (双击有极短暂的黑框闪现);从已有终端(PowerShell/cmd)启动 GUI 时会**复用该终端控制台**,
> PowerShell 会**阻塞直到窗体关闭**,或在已有实例时激活现有窗体并快速退出。

### 2. npm / `dsh plugin`(插件市场发现与一键管理)

本项目是 DSH 插件 bundle(npm 包,`package.json` 声明了 `dsh.bundle`),可被插件市场
(dshmarket 等)发现并一键安装:

```powershell
# 通过 npm 全局安装
npm i -g @andykwok/dsh-desktop
dsh-desktop

# 通过 dsh 插件系统安装到某个 profile(会初始化/复用该 profile)
dsh plugin --profile web add github:andykwokatgithub/dsh-desktop
```

更新:

```powershell
npm update -g @andykwok/dsh-desktop   # npm 途径
dsh plugin --profile web update          # dsh 插件途径(自动激活升到新版本的 bundle)
```

> 注:`dsh plugin add github:...` 会安装 git 源并运行包的脚本;pnpm 可能要求先在 profile 的
> `pnpm-workspace.yaml` 的 `allowBuilds` 里放行本包,命令失败时按 pnpm 提示操作即可。
> **npm 途径安装后,命令是 `dsh-desktop`(不带 `.exe`)**,没有 install 脚本——预编译 EXE 已内置,
> 首次运行 `dsh-desktop` 时自动拷贝到 `%LOCALAPPDATA%\dsh-desktop\dsh-desktop.exe`(不在 PATH)并启动,
> 不联网。

### 3. 从源码构建(贡献者)

见[构建](#构建)。

---

## 快速上手

```powershell
# 通过 npm / dsh plugin 安装后,命令是 dsh-desktop(不带 .exe):
dsh-desktop

# 直接使用从 GitHub 下载或本地构建的 EXE:
.\dsh-desktop.exe
```

> npm 安装后,真正的二进制在 `C:\Users\<你>\AppData\Local\dsh-desktop\dsh-desktop.exe`,并不在
> PATH 上;`dsh-desktop` 命令会把它拷贝到位并启动。想直接双击/引用它,可用上面这个路径。

典型场景:

- **冷启动**(服务未跑):自动 `dsh web --no-open` 拉起服务,先显示加载态,健康后再跳转 UI。
- **热启动**(服务已在跑,且是本壳启动的):健康校验通过后直接复用,快速进入(≤3s)。
- **认证失效自愈**(自有实例的 cookie 失效/过期):自动停掉该**自有**实例并以新 token 重新拉起,
  不把用户留在 401 页面上;非本壳实例不会被停,只转 token 输入页。
- **附着别人的实例**(端口上是你自己 `dsh web` 起的实例):先按持久化 cookie 直接进入;若取不到
  验证(cookie 失效/过期)则显示 **token 输入页**——把 `dsh web` 打印的整条认证 URL(或裸 token)
  粘进去即可附着,也可以点"改用本应用自己的实例"在 ≥10000 端口自建一个。
- **端口被别的进程占用**(非 dsh):**不动占用者**,自动在 `10000-65535` 里挑一个空闲端口自建,
  占用者身份写入日志。
- **重复启动**(窗口已存在):激活已有窗口并置顶(即使最小化),新进程立即退出。

> 所有交互(加载/错误/token/更新询问)都由窗口内的**本地 HTML 页**承载,由 `Bind` 桥接回调 Go;
> 原生 MessageBox 只保留给"窗口创建之前"的失败(单实例/建窗/参数校验错误)。窗口创建之后的错误
> 一律显示在页面上(错误原因可注入、可重试),同时写入
> `%LOCALAPPDATA%\dsh-desktop\dsh-desktop.log`。

---

## 配置

命令行 flags(`--url` `--host` `--port` `--command` `--stop-on-exit` `--devtools` `--context-menu`
`--startup-timeout` `--poll-ms` `--width` `--height` `--title` `--version` `--check-update` `--update`):

| Flag | 默认值 | 说明 |
| :-- | :-- | :-- |
| `--url` | （空，自动派生） | 可选覆盖:要附着的 DSH Web UI 地址,可携带启动 token(`--url "http://127.0.0.1:34567/?token=…"`)。**未**显式给 `--host`/`--port` 时,URL 的 host/port 会被采纳;若与显式 `--host`/`--port` 冲突则报错。URL 必须含**显式端口** |
| `--host` | `127.0.0.1` | 健康探测与 spawn 的绑定 host(校验阶段即拒绝 `0.0.0.0`) |
| `--port` | `3080` | `dsh web` 的监听端口(1-65535);被非 dsh 进程占用时自动改用 `10000-65535` 的空闲端口 |
| `--command` | `web` | 要 spawn 的 `dsh` 子命令(`web` = `--profile web` 别名) |
| `--stop-on-exit` | `false` | 关闭窗口时是否同时停止 dsh 服务(默认保留,二次启动更快)。只停**本壳自己的、归属可证**的实例;附着或他人启动的实例**永不**停止 |
| `--devtools` | `false` | 开启 WebView2 开发者工具(安全默认:关闭) |
| `--context-menu` | `true` | 保留 WebView2 默认右键菜单(默认开启;如需禁用传 `--context-menu=false`) |
| `--startup-timeout` | `30` | 等待服务就绪的超时(秒,须>0) |
| `--poll-ms` | `500` | 健康校验轮询间隔(毫秒,须>0) |
| `--width` | `1200` | 窗口宽度(须>0) |
| `--height` | `800` | 窗口高度(须>0) |
| `--title` | `DeepSeek Harness` | 窗口标题(仅用于显示;单实例定位不依赖标题) |
| `--version` | `false` | 打印版本并退出 |
| `--check-update` | `false` | 检查 GitHub Releases 是否有新版本并退出 |
| `--update` | `false` | 下载并应用最新版本,然后退出 |

> 参数校验：`--host/--port/--width/--height/--startup-timeout/--poll-ms/--url` 在启动前做语义校验，
> 值非法即在**控制台**(CLI 命令)或**窗口内错误页**(GUI 启动,窗口创建前则用原生对话框)中报错并退出。
> 规范写法为双横杠 `--flag`；Go 的 `flag` 包同时兼容单横杠 `-flag` 输入。
>
> **`--help` 文案跟随系统语言**:优先取 `LC_ALL`/`LC_MESSAGES`/`LANG`,未设置时回退 Windows 用户界面
> 语言(`GetUserDefaultUILanguage`)——中文系统显示中文,其他显示英文。`--check-update`/`--update`
> 的控制台输出同样如此。

---

## 实例识别与安全边界

本壳对"端口上的 dsh 是谁起的"分三层判定,结论决定它**复用**、**附着**还是**避让**:

| 层 | 判据 | 命中后的行为 |
| :-- | :-- | :-- |
| **L1 归属记录** | `%LOCALAPPDATA%\dsh-desktop\web-endpoint.json` 里的 `listenerPid` + 进程启动时间(±1s)仍与当前监听者一致 ⇒ 是**本壳自己**启动的 | 健康则直接复用(带持久化 cookie) |
| **L2 强证据** | 监听 PID 的命令行含 `@deepseek-ai\dsh`/`deepseek-harness`/`dsh web`/`…\dsh\lib\bin.js` 等强关键字 | 健康则先按 cookie 附着;取不到验证转 token 输入页 |
| **L3 弱证据** | 裸 `dsh`、`--port <该端口>` 这类弱关键字(本壳自身的 `dsh-desktop.exe --port …` 也会命中) | 不足以判定为 dsh,只作参考 |

判定与终止的安全约定:

- 监听者身份来自 **Win32 API**(`GetExtendedTcpTable(TCP_TABLE_OWNER_PID_LISTENER)` 取监听 PID、
  `QueryFullProcessImageNameW` 取镜像、`NtQueryInformationProcess` 取命令行),**不解析 `netstat` 文本**、
  不 spawn PowerShell/CIM;HTTP 侧认 dsh 的**认证边界**(`200`/`303`,或 `401` + 认证栅栏文案),
  不是"只看 200"。
- **终止进程必须有证明**:`--stop-on-exit` 与认证栅栏自愈都走 `service.StopOwned`,只接受
  "记录中的监听 PID + 启动时间仍一致"或"该 PID 是本壳的**直接子进程**";证明不成立就**放弃终止**并记日志
  (宁可不杀,也不误杀)。
- **任何分支都不终止端口占用者**:端口被别人占用时只换端口,不做抢占或清理。
- **token 是凭据**:只在页面 → Go → `Navigate(?token=…)` 之间于内存流转,**不落盘、不入日志**;
  日志只记 `host:port` 与占用者身份。

---

## 构建

推荐(也会内嵌 DeepSeek 图标):

```powershell
.\build.ps1
```

或直接构建(默认即 console 子系统,无需 `-H=windowsgui`):

```powershell
go build -o dsh-desktop.exe .
```

版本号可通过 `build.ps1 -Version X.Y.Z` 或 `-ldflags` 注入:

```powershell
.\build.ps1 -Version 1.2.3
go run . -version   # -> dsh-desktop 1.2.3
```

> **构建形态**:出于让 CLI 命令在 PowerShell/cmd 中同步输出等考量,EXE 用 **console 子系统**构建
> (不再用 `-H=windowsgui`)。GUI 运行时 `main.hideConsole()` 会隐藏双击产生的控制台窗口(有极短的
> 黑框闪现);从已有终端启动 GUI 时复用该终端控制台,PowerShell 会阻塞直到窗体关闭。
> `webview_go` 依赖 CGO,需要本机 Windows + GCC;不能跨平台交叉编译。
> 运行 `dsh-desktop.exe` 会真的拉起 `dsh web`,不要在无 DSH 环境时盲目运行;验证请用
> `go run . -version` 或单测。

### 应用图标

- `assets/deepseek.ico`(16–256px)、`assets/deepseek-256.png`。
- `tools/make-icon.mjs` 用 `sharp` 从官方 SVG 重新生成 ICO。
- `rsrc_windows_amd64.syso` 内嵌图标;`build.ps1` 在 ICO 更新时自动用
  `github.com/akavel/rsrc` 重建。

---

## 目录结构

```
main.go                         入口(配置→单实例→建窗→启动决策:复用/附着/token/自建;含自更新命令)
internal/appdir/                %LOCALAPPDATA%\dsh-desktop 状态目录(日志/端点记录)
internal/config/                运行配置与 flag;Version 常量;系统语言检测(中文/英文文案)
internal/service/               服务探测/spawn/生命周期;端点归属记录;回退端口;token 解析与校验
internal/procinfo/              端口占用者识别:监听 PID/镜像路径/命令行 + DSH 关键字匹配(纯 Win32)
internal/singleinstance/        互斥体+原生窗口类+激活消息+Win32 封装
internal/update/                GitHub Releases 自更新(校验+下载+应用)
internal/webview/               安全脚本/外链交接/认证栅栏探测 + 页面 Bind 桥接
internal/ui/                    go:embed 的 loading/error/token 页
bin/                            npm 启动器
scripts/fetch-exe.mjs           下载并校验预编译 EXE(npm postinstall)
cordis.patch.yml                DSH bundle patch 层(市场/插件系统识别)
assets/                         应用图标
tools/make-icon.mjs             用 SVG 重新生成 .ico
rsrc_windows_amd64.syso         嵌入图标的资源对象
build.ps1                       一键构建
.github/workflows/              CI 与发版流水线
docs/                           PRD + 技术设计 + 发布指南
```

---

## 贡献与安全

- **贡献**:环境要求、开发命令与提交约定见[`CONTRIBUTING.md`](CONTRIBUTING.md)。
- **安全**:漏报私报方式、下载/更新校验与品牌资产关注点见[`SECURITY.md`](SECURITY.md)。
- **变更日志**:见[`CHANGELOG.md`](CHANGELOG.md)(Keep a Changelog + 语义化版本)。

---

## 许可与商标

MIT License(详见[`LICENSE`](LICENSE))。项目内置的 DeepSeek 鲸鱼图标及其品牌元素属于
DeepSeek 方,本许可**不**授予商标/Logo/品牌资产使用权,相关使用需另行取得品牌方许可。

---

## 文档

- 需求基线:[`docs/dsh-desktop-prd.md`](docs/dsh-desktop-prd.md)
- 技术设计:[`docs/dsh-desktop-technical-design.md`](docs/dsh-desktop-technical-design.md)
- 发布/收录/安装/更新指南(面向市场与维护者):[`docs/publish.md`](docs/publish.md)
- README 自身的需求基线:[`docs/dsh-desktop-readme-prd.md`](docs/dsh-desktop-readme-prd.md)
