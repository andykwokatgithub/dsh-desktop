# 贡献指南(Contributing)

感谢你愿意为本项目做贡献。这里给出开发、构建与提交约定,让改动安全且可合并。

## 环境

- Windows + Go(≥1.21,需 CGO)+ GCC/mingw-w64(`webview_go` 依赖)。
- WebView2 Evergreen 运行时。
- `dsh` 全局安装且 `node`、`dsh` 均在 `PATH`(运行 `dsh web` 需要)。
- 仅当需要重建图标时才需要 Node + `sharp`。

## 开发

```powershell
# 构建(console 子系统;GUI 启动隐藏控制台;使用仓库内 .gopath 缓存)
.\build.ps1

# 静态检查(注意:singleinstance 的 unsafe.Pointer 是已知误报,勿为此重构)
go vet ./internal/config ./internal/service ./internal/ui ./internal/webview .

# 查看版本(go run 走 console 子系统,stdout 可见)
go run . -version
```

> 不要随手运行 `dsh-desktop.exe`——它会真的拉起 `dsh web` 后端。验证优先用
> `go run . -version` 或追加的单测。

## 提交约定

- 中文注释与文档(与仓库一致)。
- 影响行为的变更务必同步更新 `CHANGELOG.md`(先写 `[Unreleased]`)。
- 不要提交构建产物:`dsh-desktop.exe`、`.gopath/`、`*.exe~`、`*~`。
- 版本号:`internal/config.Version` 默认 `0.1.0`,发版用
  `build.ps1 -Version X.Y.Z` 或 `-ldflags -X .../config.Version=<ver>` 覆盖。

## 发布

见 [`docs/publish.md`](docs/publish.md):打 `vX.Y.Z` tag 触发的 `release.yml` 会构建
EXE、生成校验和并附到 GitHub Release(可选 `npm publish`)。
