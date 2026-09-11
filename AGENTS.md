# AGENTS.md

面向在此仓库工作的 AI 代理与贡献者的速查指南。目标:让你在短时间内安全、正确地构建和修改本项目,而不是重新摸索一遍。

## 1. 这个项目是什么

`dsh-desktop` 是 DeepSeek Harness(`dsh`)的 **Windows 桌面壳(Wrapper)**,用 Go + WebView2 把 `dsh web` 提供的 Web UI(默认 `http://127.0.0.1:3080`)装进原生窗口。它不负责任何 Harness 逻辑,只做四件事:

- **内嵌渲染**:WebView2 加载 DSH Web UI(通过 `github.com/webview/webview_go`)。
- **服务生命周期**:探测并静默启动/复用/停止 `dsh web` 后端。
- **单实例防重**:重复启动时激活已有窗口并退出。
- **打包体验**:console 子系统构建(GUI 启动时隐藏控制台)+ DeepSeek 图标 + 内嵌 loading/error/token 页。

需求与实现约束见 `docs/dsh-desktop-prd.md`(V1.2)与 `docs/dsh-desktop-technical-design.md`。

## 2. 环境与工具链(重要)

| 依赖 | 说明 | 缺失后果 |
| :-- | :-- | :-- |
| **Go(含 CGO)** + **GCC/mingw-w64** | `webview_go` 是 CGO 库,必须本地编译 Windows 目标 | 无法交叉编译;需 Windows 工具链 |
| **WebView2 Evergreen 运行时** | 运行时依赖(Windows 11 预装;Win10 需装) | 启动时窗口报错 |
| **`dsh` 全局安装 + `PATH`** | `npm i -g @deepseek-ai/dsh`,且 `node`、`dsh` 均在 PATH | 无法启动服务 |
| **Node + `sharp`(仅当重新生成图标)** | `tools/make-icon.mjs` 用 `sharp` 渲染 SVG→ICO | 不重建图标则无需 |

> **构建缓存沙箱**:本仓库默认把 `GOPATH/GOMODCACHE/GOCACHE` 指向仓库内 `.gopath/`(见 `build.ps1`),因为沙箱可能拒绝写入用户目录。这也是被 `.gitignore` 忽略的原因。**不要提交 `.gopath/`。**

## 3. 构建 / 运行 / 验证

```powershell
# 推荐:自动处理 .syso 资源 + 构建(console 子系统,默认)
.\build.ps1

# 等价手写
go build -o dsh-desktop.exe .

# 看版本
go run . -version        # -> dsh-desktop 0.3.0

# 静态检查
go vet ./...
```

- 构建默认是 **console 子系统**(不再用 `-H=windowsgui`),这样 `--version`/`--check-update`/`--update`
  在 PowerShell/cmd 中会**同步等待并内联输出**。GUI 启动时由 `main.hideConsole()` 隐藏双击产生的控制台窗口
  (有极短黑框闪现);从已有终端启动 GUI 会复用该终端控制台,PowerShell 会阻塞直到窗体关闭。
- 运行 `.\dsh-desktop.exe` 会**真的拉起 `dsh web` 后台服务**(对端口 3080 探测/启动)。在没有 DSH 环境或不想污染当前环境时,不要盲目跑;**验证请优先用 `go run . -version` 或单元测试**。
- 修改 `internal/*` 后重建即可;修改图标需重新生成 `.ico`→`.syso`(见 §5)。

## 4. 目录结构

```
main.go                            入口:配置→单实例→窗口→启动决策(复用/token/自建)→HTML 页
internal/appdir/                   %LOCALAPPDATA%\dsh-desktop 状态目录(日志/端点记录)
internal/config/                   运行时配置与 flag;Version 常量;--url 派生 host/port
internal/service/                  探测/spawn/生命周期/端口挑选/token 校验/端点归属记录
internal/procinfo/                 端口占用者识别:监听 PID/镜像路径/命令行 + DSH 关键字匹配
internal/singleinstance/           互斥体+原生窗口类+激活消息+Win32 封装
internal/webview/                  安全脚本/外链交接/认证栅栏探测 + 页面 Bind 桥接
internal/ui/                       go:embed 的 loading/error/token 页
assets/                            应用图标(deepseek.ico / deepseek-256.png)
tools/make-icon.mjs                用 SVG 重新生成 .ico(node+sharp)
rsrc_windows_amd64.syso            嵌入图标的资源对象(go build 自动链接)
build.ps1                          一键构建
docs/                              PRD + 技术设计
CHANGELOG.md                       变更日志(Keep a Changelog)
```

## 5. 关键设计决策与实现事实

- **依赖库是 `github.com/webview/webview_go`**,不是 `github.com/webview/webview`(后者当前版本已无 Go 绑定,只有 C++ 核心)。
- `webview_go.NewWindow(debug, parentHwnd)` 可以把 WebView2 嵌进**你自己创建的原生窗口**,从而用"唯一窗口类 + 注册消息"做单实例定位(而不是靠窗口标题)。这正是 `internal/singleinstance` 的做法。
- 图标管线:`assets/deepseek.ico`(16–256px)→ `rsrc` 生成 `rsrc_windows_amd64.syso` → `go build` 自动链接。`build.ps1` 在 `.ico` 比 `.syso` 新时自动重跑 `go run github.com/akavel/rsrc@latest ...`。
- 主窗体也显示图标:窗口类上设 `hIcon/hIconSm` + 发 `WM_SETICON`(见 `window.go` 的 `loadImageIcon`)。
- Windows spawn 必须经 `cmd /c dsh web --no-open`,因为全局 `dsh` 是 npm `.cmd`/`.ps1` shim,Go 的 `os/exec` 不会自动按 PATHEXT 解析;且需继承 `PATH`/`DSH_HOME`。
- 服务识别是**三层判据**:`internal/procinfo` 用 `GetExtendedTcpTable(TCP_TABLE_OWNER_PID_LISTENER)` 取**监听 PID**、`QueryFullProcessImageNameW` 取镜像路径、`NtQueryInformationProcess(ProcessCommandLineInformation)` 取命令行;HTTP 侧认 dsh 的**认证边界**(`200`/`303`,或 `401` + `dsh web authentication required` 栅栏文案),**不是"只看 HTTP 200"**。**不要解析 `netstat` 文本(表头随语言本地化、`:3080` 会误配 `:30801`/对端地址),也不要 spawn PowerShell/CIM。**
- `internal/procinfo` 两个实测事实:①真正监听端口的是 **`node.exe`**(不是 npm shim 的 `cmd.exe`,也不是本壳),命令行为 `"node" "…\npm\\node_modules\@deepseek-ai\dsh\lib\bin.js" web --no-open --host … --port …`;②`ProcessCommandLineInformation`(类 60)会把字符串**复制进调用者缓冲区**(自包含 `UNICODE_STRING`,先按返回长度分配),同用户进程免提权、免 `ReadProcessMemory`。DSH 关键字分强弱两档(见 `internal/procinfo/dsh.go`):`@deepseek-ai\dsh`/`deepseek-harness`/`dsh web`/`\dsh\lib\bin.js` 为强证据;裸 `dsh`、`--port <端口>` 仅弱证据(本壳自身的 `dsh-desktop.exe --port …` 也会命中)。
- **启动决策在窗口内完成(窗口先建)**:先建窗口并 `SetHtml(loading)`,再由后台 goroutine 决策——(1) 归属记录命中且健康 ⇒ 复用持久化 cookie;(2) `--url` 的 token 校验通过 ⇒ 直接附着;(3) 端口上是**非本壳 DSH 实例**(L2 强证据或 L3 强关键字,含祖先链)⇒ 健康时**先按 cookie 附着**,命中认证栅栏(取不了验证)才显示 `token.html` 让用户粘贴 `dsh web` 打印的认证 URL/token(有效即 `Navigate(?token=…)` 附着、无效页内重试;仅 L3 命中/未就绪则直接显示 token 页);(4) 端口被其它进程占用 ⇒ 在 `[10000,65535]` 自建;(5) 端口空闲 ⇒ 在首选端口自建。**任何分支都绝不终止端口占用者**。
- **归属记录**:`%LOCALAPPDATA%\dsh-desktop\web-endpoint.json` = `{host,port,listenerPid,listenerStartedAt,spawnerPid,spawnerStartedAt}`;`Endpoint.Live()` 用 **listenerPid + 进程启动时间(±1s)** 判定"是不是本壳启动的"(PID 复用不会误判)。
- **终止进程必须有证明**:`--stop-on-exit` 与"认证栅栏自愈"都走 `service.StopOwned`,它只接受"记录中的 `listenerPid`+启动时间仍一致"或"该 PID 是本壳**直接子进程**"(`procinfo.IsChildOf`),否则**放弃终止**并记日志(宁可不杀也不误杀)。**非本壳实例即使用户带 `--stop-on-exit` 也绝不停止**;附着的他人实例同理。
- **HTML 模式窗体**:`internal/ui` 的 `loading/error/token` 页经 `view.Bind` 与 Go 双向通信(`__dshSubmitToken`/`__dshSkipToken`/`__dshTokenTarget`/`__dshRetry` ↔ `__dshTokenResult`/`__dshError`);**Bind 回调在 UI 线程同步执行,必须立刻返回**——耗时工作丢 goroutine,再 `Dispatch`+`Eval` 回页面。原生 MessageBox 只保留给"窗口创建前"的配置错误。
- **token 是凭据**:只在内存中流转(页面 → Go → `Navigate(?token=…)`),**不落盘、不入日志**;日志只记 `host:port`。
- 生命周期默认"持久化复用 + 复用前健康校验 + 主动停止";`--stop-on-exit` 才在关窗时停止服务(附着的他人实例永不停止)。

## 6. 已知坑 / 注意点

- **`go vet` 有 1 处已知假阳性**:`internal/singleinstance/window.go:94` 的 `webview.NewWindow(debug, unsafe.Pointer(hwnd))` 是把窗口句柄(整数)转指针,安全,但被 vet 标记 `possible misuse of unsafe.Pointer`。**不要为了消掉它而重构出 bug。**
- **`*.exe~` 备份文件**:Go 在 Windows 上 `-o` 输出会生成 `dsh-desktop.exe~` 等临时/备份文件;`.gitignore` 已含 `*.exe~` 与 `*~`。若提交前发现被暂存,`git rm --cached` 并删除即可。
- **GUI 启动时控制台被隐藏**:console 子系统构建下,GUI 启动会 `ShowWindow(SW_HIDE)` 隐藏自身控制台窗口,此时 `os.Stderr` 对双击用户不可见。因此**窗口创建之后**的用户可见错误一律走 `internal/ui` 的本地 HTML 页(`error.html`,Go 用 `Eval(__dshError(msg))` 注入原因);只有**窗口创建前**的失败(单实例/建窗/参数错误)才用 `messageBox` + 写入 `%LOCALAPPDATA%\dsh-desktop\dsh-desktop.log`(路径见 `internal/appdir`)。给用户看的错误别只写 `os.Stderr`。
- **CLI 命令走控制台,`fatal` 走对话框**:`--version`/`--check-update`/`--update` 及其配置错误输出到控制台(stdout/stderr);GUI 启动路径的致命错误用 `messageBox`。区分看 `main.go` 的 `reportConfigError`/`fatal`。
- **CGO 不能交叉编译**,`webview_go` 需要本机 Windows + GCC。

## 7. 约定

- 版本:`internal/config.Version`(默认 `0.3.0`,与 `package.json` 及 CHANGELOG 当前发布版一致),发布时用 `-ldflags -X github.com/deepseek-ai/dsh-desktop/internal/config.Version=<ver>` 覆盖,并同步更新 `CHANGELOG.md`、`package.json` 与打 Git tag(如 `v0.3.0`)。
- 变更日志:凡是影响行为的变更,同步更新 `CHANGELOG.md`(先写 `[Unreleased]`,发布时移入版本)。
- 语言/注释:代码注释与文档以中文为主,与现有仓库一致。
- 不要提交 `dsh-desktop.exe`、`.gopath/`、`dsh-desktop.exe~`。
