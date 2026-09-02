# dsh-desktop

> DeepSeek Harness 的 Windows 桌面壳(Go + WebView2)——把 `dsh web` 的 Web UI
> 装进原生窗口。作为一个 DSH 插件 bundle 发布,可在插件市场(dshmarket)被发现,
> 并通过 `dsh plugin` 或 npm 一键安装、更新。

[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)
[![Go](https://img.shields.io/badge/Go-1.21+-00ADD8?style=flat&logo=go)](go.mod)
[![Platform](https://img.shields.io/badge/Platform-Windows%20x64-0078D4?style=flat&logo=windows)](README.md)
[![GitHub release](https://img.shields.io/github/v/release/andykwokatgithub/dsh-desktop)](https://github.com/andykwokatgithub/dsh-desktop/releases)
[![npm](https://img.shields.io/npm/v/@andykwok/dsh-desktop)](https://www.npmjs.com/package/@andykwok/dsh-desktop)

---

## 是什么

`dsh-desktop` 是把 DeepSeek Harness(`dsh web`,默认 `http://127.0.0.1:3080`)
装进 Windows 原生窗口的壳。它不承载任何 Harness 逻辑,只做:

- **内嵌渲染**:WebView2 加载 DSH Web UI。
- **服务生命周期**:探测并静默启动/复用/停止 `dsh web` 后端。
- **单实例防重**:重复启动时激活已有窗口并退出。
- **打包体验**:GUI 子系统(无控制台)+ DeepSeek 图标 + 加载/错误页。

需求与设计见 `docs/dsh-desktop-prd.md`(V1.1)与
`docs/dsh-desktop-technical-design.md`。

---

## 安装与更新(3 条路径)

### 1. GitHub Releases 直接下载(终端用户推荐)

到 [Releases](https://github.com/andykwokatgithub/dsh-desktop/releases) 下载
`dsh-desktop-win-x64.exe`,双击即可。该 EXE 内置自更新:

```powershell
.\dsh-desktop.exe -check-update   # 检查是否有新版本
.\dsh-desktop.exe -update         # 下载并应用到自身,重启生效
```

### 2. npm / `dsh plugin`(插件市场发现与一键管理)

本项目是 DSH 插件 bundle(npm 包,`package.json` 声明了 `dsh.bundle`),因此可被
插件市场(dshmarket 等)发现并一键安装:

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
dsh plugin --profile web update          # dsh 插件途径(会自动激活升到新版本的 bundle)
```

> 注:`dsh plugin add github:...` 会安装 git 源并运行包的脚本;pnpm 可能要求先在
> profile 的 `pnpm-workspace.yaml` 的 `allowBuilds` 里放行本包,命令失败时按 pnpm
> 的提示操作即可。发布流水线也会以 `NPM_TOKEN` 把新版本推送到 npm(含预编译 EXE
> 的下载与校验)。

### 3. 从源码构建(贡献者)

见下文 [构建](#构建)。

---

## 发现与收录(面向插件市场)

要让 `dshmarket` 等市场收录本项目,请确保仓库:

1. 打上 GitHub topic **`dsh-plugin`**(以及可选的 `dsh-desktop`、`deepseek-harness`)。
2. `package.json` 保留 **`"dsh": { "bundle": { "patch": "./cordis.patch.yml" } }`**
   声明(市场据此校验可安装性)。
3. 版本号与 Git tag 一致(`vX.Y.Z`),发版走 `.github/workflows/release.yml`
   自动产出 EXE + `sha256` 校验文件并发布到 GitHub Release。

---

## 环境要求

- Windows 10 (1803+) / Windows 11。
- WebView2 Evergreen 运行时(Windows 11 预装;Windows 10 需安装)。
- `dsh` 全局安装(`npm i -g @deepseek-ai/dsh`),且 `node`、`dsh` 均在 `PATH`
  (运行 dsh-desktop 时后端需要)。

---

## 构建

推荐(也会内嵌 DeepSeek 图标):

```powershell
.\build.ps1
```

或直接构建:

```powershell
go build -ldflags "-H=windowsgui" -o dsh-desktop.exe .
```

版本号可通过 `build.ps1 -Version X.Y.Z` 或 `-ldflags` 注入:

```powershell
.\build.ps1 -Version 1.2.3
go run . -version   # -> dsh-desktop 1.2.3
```

> **必须保留 `-H=windowsgui`**,否则生成的是 console 子系统,双击会弹黑色控制台。
> `webview_go` 依赖 CGO,需要本机 Windows + GCC;不能跨平台交叉编译。
> 运行 `dsh-desktop.exe` 会真的拉起 `dsh web`,不要在无 DSH 环境时盲目运行;验证请
> 用 `go run . -version` 或单测。

### 应用图标

- `assets/deepseek.ico`(16–256px)、`assets/deepseek-256.png`。
- `tools/make-icon.mjs` 用 `sharp` 从官方 SVG 重新生成 ICO。
- `rsrc_windows_amd64.syso` 内嵌图标;`build.ps1` 在 ICO 更新时会自动用
  `github.com/akavel/rsrc` 重建。

---

## 运行

```powershell
.\dsh-desktop.exe
```

命令行 flags(与配置面一致):`-url` `-host` `-port` `-command` `-stop-on-exit`
`-devtools` `-context-menu` `-startup-timeout` `-poll-ms` `-width` `-height`
`-title` `-version` `-check-update` `-update`。

---

## 许可

MIT License(详见 [LICENSE](LICENSE))。项目内置的 DeepSeek 鲸鱼图标及其品牌元素
属于 DeepSeek 方所有,本许可**不**授予商标/Logo/品牌资产使用权,相关使用需另行取得
品牌方许可。

---

## 目录结构

```
main.go                         入口(配置→单实例→服务→窗口→消息循环;含自更新命令)
internal/config/                运行配置与 flag;Version 常量
internal/service/               服务探测/spawn/生命周期/进程树清理
internal/singleinstance/        互斥体+原生窗口类+激活消息+Win32 封装
internal/update/                GitHub Releases 自更新(校验+下载+应用)
internal/webview/               安全脚本与外部链接交接
internal/ui/                    go:embed 的 loading/error 页
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

## 文档

- 需求基线:[`docs/dsh-desktop-prd.md`](docs/dsh-desktop-prd.md)
- 技术设计:[`docs/dsh-desktop-technical-design.md`](docs/dsh-desktop-technical-design.md)
- 发布/收录/安装/更新指南:[`docs/publish.md`](docs/publish.md)
