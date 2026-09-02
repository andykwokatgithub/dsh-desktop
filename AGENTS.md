# AGENTS.md

面向在此仓库工作的 AI 代理与贡献者的速查指南。目标:让你在短时间内安全、正确地构建和修改本项目,而不是重新摸索一遍。

## 1. 这个项目是什么

`dsh-desktop` 是 DeepSeek Harness(`dsh`)的 **Windows 桌面壳(Wrapper)**,用 Go + WebView2 把 `dsh web` 提供的 Web UI(默认 `http://127.0.0.1:3080`)装进原生窗口。它不负责任何 Harness 逻辑,只做四件事:

- **内嵌渲染**:WebView2 加载 DSH Web UI(通过 `github.com/webview/webview_go`)。
- **服务生命周期**:探测并静默启动/复用/停止 `dsh web` 后端。
- **单实例防重**:重复启动时激活已有窗口并退出。
- **打包体验**:GUI 子系统(无控制台)+ DeepSeek 图标 + 加载/错误页。

需求与实现约束见 `docs/dsh-destop-prd.md`(V1.1)与 `docs/dsh-desktop-technical-design.md`。

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
# 推荐:自动处理 .syso 资源 + GUI 构建
.\build.ps1

# 等价手写
go build -ldflags "-H=windowsgui" -o dsh-desktop.exe .

# 看版本(go run 默认 console 子系统,可见 stdout)
go run . -version        # -> dsh-desktop 0.1.0

# 静态检查
go vet ./...
```

- `-H=windowsgui` 是**必须的**:否则 exe 是 console 子系统,双击会弹黑色控制台。
- 运行 `.\dsh-desktop.exe` 会**真的拉起 `dsh web` 后台服务**(对端口 3080 探测/启动)。在没有 DSH 环境或不想污染当前环境时,不要盲目跑;**验证请优先用 `go run . -version` 或单元测试**。
- 修改 `internal/*` 后重建即可;修改图标需重新生成 `.ico`→`.syso`(见 §5)。

## 4. 目录结构

```
main.go                            入口:配置→单实例→服务→窗口→消息循环
internal/config/                   运行时配置与 flag;Version 常量
internal/service/                  服务探测/spawn/生命周期/进程树清理
internal/singleinstance/           互斥体+原生窗口类+激活消息+Win32 封装
internal/webview/                  安全脚本与外部链接交接
internal/ui/                       go:embed 的 loading/error 页
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
- 服务就绪判定是"端口 + HTTP 200 健康校验"双判据(不是只看端口),区分"dsh 在跑/僵死/端口被他人占用"。
- 生命周期默认"持久化复用 + 复用前健康校验 + 主动停止";`--stop-on-exit` 才在关窗时停止服务。

## 6. 已知坑 / 注意点

- **`go vet` 有 1 处已知假阳性**:`internal/singleinstance/window.go:95` 的 `webview.NewWindow(debug, unsafe.Pointer(hwnd))` 是把窗口句柄(整数)转指针,安全,但被 vet 标记 `possible misuse of unsafe.Pointer`。**不要为了消掉它而重构出 bug。**
- **`*.exe~` 备份文件**:Go 在 Windows 上 `-o` 输出会生成 `dsh-desktop.exe~` 等临时/备份文件;`.gitignore` 已含 `*.exe~` 与 `*~`。若提交前发现被暂存,`git rm --cached` 并删除即可。
- **GUI 子系统没有可见 stderr**:所以 `main.go` 的致命错误统一走 `messageBox` + 写入 `%LOCALAPPDATA%\dsh-desktop\dsh-desktop.log`。给用户看的错误别只写 `os.Stderr`。
- **`-version` 在 GUI exe 下 stdout 不一定可见**(无控制台);脚本/CI 用重定向或 `go run . -version` 最可靠。
- **CGO 不能交叉编译**,`webview_go` 需要本机 Windows + GCC。

## 7. 约定

- 版本:`internal/config.Version`(默认 `0.1.0`),发布时用 `-ldflags -X github.com/deepseek-ai/dsh-desktop/internal/config.Version=<ver>` 覆盖,并同步更新 `CHANGELOG.md` 与打 Git tag(如 `v0.1.0`)。
- 变更日志:凡是影响行为的变更,同步更新 `CHANGELOG.md`(先写 `[Unreleased]`,发布时移入版本)。
- 语言/注释:代码注释与文档以中文为主,与现有仓库一致。
- 不要提交 `dsh-desktop.exe`、`.gopath/`、`dsh-desktop.exe~`。
