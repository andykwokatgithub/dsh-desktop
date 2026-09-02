# Changelog

所有对本项目的重大变更都会记录在此文件中。

本文件的格式基于 [Keep a Changelog](https://keepachangelog.com/zh-CN/1.1.0/)，
且本项目遵循 [语义化版本](https://semver.org/lang/zh-CN/)（Semantic Versioning）。

## [Unreleased]
<!-- 下一版本将在此记录 -->

### Added
- `AGENTS.md`：面向 AI 代理与贡献者的仓库速查指南（工具链/构建/结构/坑点/约定）。

### Fixed
- 二次实例激活时不再改动主窗口几何：移除 `activateWindow` 回退路径中无条件的 `ShowWindow(SW_RESTORE)`，改为仅置顶（`SetForegroundWindow` + `AttachThreadInput` 回退）；最小化窗口的恢复（回到用户此前大小）仍由 `BringToFront` 处理。

## [0.1.0] - 2026-09-02

### Added
- 初始化 Go 工程脚手架：`main.go` 入口 + `internal/{config,service,singleinstance,webview,ui}` 分层，基于 Go + WebView2（`github.com/webview/webview_go`）。
- 内嵌 DeepSeek Harness 的 Web UI（默认 `http://127.0.0.1:3080`），窗口标题 `DeepSeek Harness`，默认 1200×800。
- 服务生命周期（FR-01/FR-04）：**端口 + HTTP 健康双判据**探测、`cmd /c dsh web --no-open` 静默启动、就绪轮询、复用前健康校验、陈旧/僵死服务清理、`--stop-on-exit` 可配置停止。
- 单实例防重（FR-03）：`Local\DSHDesktopApp` 互斥体 + 唯一窗口类 `DSHDesktopAppMainWindow` + `RegisterWindowMessage` 激活消息，重复启动时激活已有窗口并退出。
- GUI 子系统构建（`-H=windowsgui`），启动时不出现黑色控制台。
- 应用图标：DeepSeek Harness 鲸鱼 logo（蓝底白鲸），通过 `assets/deepseek.ico` 与 `rsrc_windows_amd64.syso` 嵌入 exe 与窗口。
- 加载态/错误态页面（`internal/ui` 内嵌 `loading.html` / `error.html`）。
- 安全脚本与外部链接交接（`internal/webview`）：默认禁用右键菜单、拦截外部 `http(s)`/`mailto:` 并交给系统默认浏览器。
- 配置项：`-url/-host/-port/-command/-stop-on-exit/-devtools/-context-menu/-startup-timeout/-poll-ms/-width/-height/-title/-version`。
- 版本号来源：`internal/config.Version`（默认 `0.1.0`，可在构建时用 `-ldflags -X ...=...` 覆盖），`-version` 打印版本。
- 构建与工具：`build.ps1`、`tools/make-icon.mjs`、`README.md`、`.gitignore`、`.gitattributes`。
- 文档：需求基线 PRD（`docs/dsh-destop-prd.md`，V1.1）与技术设计（`docs/dsh-desktop-technical-design.md`）、`CHANGELOG.md`。
- Git 仓库初始化（`main` 分支）。

### Changed
- （无）

### Fixed
- （无）

<!--
# 维护约定
1. 每次合并会影响行为的变更时，在本次任务中同步更新本文件。
2. 变更分类：Added（新增）/ Changed（变更）/ Deprecated（弃用）/ Removed（移除）/ Fixed（修复）/ Security（安全）。
3. 新变更先写入顶部的 [Unreleased]；发布时将其条目移动到对应版本标题（如 [0.1.1]），并在该标题后补日期，同时打上 Git tag（`v0.1.1`）。
4. 版本号遵循语义化版本；破坏性变更递增 MAJOR，新增功能递增 MINOR，修复递增 PATCH。
-->
