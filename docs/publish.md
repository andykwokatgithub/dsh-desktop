# dsh-desktop 发布指南(Publish Guide)

面向本项目维护者与有意接入插件市场(dshmarket 等)的贡献者。本文说明如何让
`dsh-desktop` 满足四个发布诉求:**可被发现**(dshmarket/DH 插件市场)、**便于安装**
、**便于更新**、**许可明确**。

---

## 0. 一句话模型

DSH 插件( bundle )本质是一个 **npm 包**,它的 `package.json` 声明:

```json
"dsh": { "bundle": { "patch": "./cordis.patch.yml" } }
```

`dsh plugin --profile <name> add|update <spec>` 是这个 npm/pnpm 系统的薄封装:
安装(`add`)与更新(`update`)都通过 pnpm 完成,市场主要靠「是否存在 `dsh.bundle`
声明」来校验可安装性并收录。

`dsh-desktop` 的**真正交付物是 `dsh-desktop.exe`(Windows x64 预编译二进制)**。
因此我们用两条互补的通道交付它:

| 通道 | 面向谁 | 安装 | 更新 |
| :-- | :-- | :-- | :-- |
| **GitHub Releases** | 直接下载 EXE 的终端用户 | 下载 `dsh-desktop-win-x64.exe` | 内置 `-check-update` / `-update` 自更新 |
| **npm / `dsh plugin`** | 走 npm 或 DSH 插件的用户 | `npm i -g @andykwok/dsh-desktop` 或 `dsh plugin --profile web add github:andykwokatgithub/dsh-desktop` | `npm update -g ...` / `dsh plugin ... update` |

---

## 1. 发现与收录(让 dshmarket 能看到你)

1. **GitHub topic**:给仓库打上 `dsh-plugin`(必要时再加 `dsh-desktop`、
   `deepseek-harness`)。大多数第三方市场(含 dshmarket)会按该 topic 嗅探收录。
2. **`dsh.bundle` 声明**:保留 `package.json` 中的
   `"dsh": { "bundle": { "patch": "./cordis.patch.yml" } }`。市场据此判定
   「可安装」;缺失会被拒绝。
3. **`cordis.patch.yml`**:必须是「顶层的 YAML 数组」。
   当前实现是**空数组 + 注释**(合法、可被市场与 `dsh plugin` 识别)。
   > 说明:要真正在 Harness 里注册可点击启动桌面壳的命令/技能入口,需要用到基础
   > 应用配置里精确的入口 `id` 与 schema;这是**后续增强项**(见 §7),本版先用
   > 「合法但空」的 patch,避免对未知入口打补丁产生启动告警。
4. **README 与元数据**:紧邻仓库的 `README.md` 已给出安装/更新命令、许可、topic
   说明。`package.json` 的 `keywords`、`description`、`homepage` 也会被市场/搜索利用。
5. **稳定的仓库名与 owner**:市场通常按 `owner/repo` 索引;发布前确认
   `github:andykwokatgithub/dsh-desktop` 或你的实际地址在文档里一致。

---

## 2. 仓库卫生(提交前检查)

**必须提交 / 保留:**
- 源码 `main.go`、`internal/`、`go.mod`、`go.sum`。
- `assets/`、`rsrc_windows_amd64.syso`、`tools/`、`build.ps1`。
- 本发布相关的 `package.json`、`cordis.patch.yml`、`bin/`、`scripts/`、
  `LICENSE`、`README.md`、`CHANGELOG.md`、`docs/`、`.github/workflows/`、
  `.gitignore`、`.gitattributes`。

**不要提交(已在 `.gitignore`,核实即可):**
- `.gopath/`(仓库内构建缓存)、`dsh-desktop.exe`、`*.exe~`、`*~`、`*.dll`。
- 任何本地构建产物。

---

## 3. 版本管理与发版

- 版本号 = `internal/config.Version`(默认 `0.1.0`),也可用
  `build.ps1 -Version X.Y.Z` 或 `-ldflags -X .../config.Version=<ver>` 覆盖。
- 每次行为性变更同步更新 `CHANGELOG.md`(先写 `[Unreleased]`,发版时移入版本标题)。
- 打 tag(如 `v1.2.3`)触发 `.github/workflows/release.yml`,自动完成:
  1. 注入版本号构建 `dsh-desktop.exe`;
  2. 生成 `dsh-desktop-win-x64.exe`(重命名)、`dsh-desktop-win-x64.exe.sha256`、
     `dsh-desktop-win-x64.zip`;
  3. 附到 GitHub Release(含自动 release notes);
  4. 若配置了 `NPM_TOKEN` secret,`npm publish` 推送新版本到 npm。
- 依赖:仓库需允许 GitHub Actions;`NPM_TOKEN` secret 用于 npm 发布(可选)。

---

## 4. npm / `dsh plugin` 安装与更新

```powershell
# 安装
npm i -g @andykwok/dsh-desktop        # npm 途径
dsh plugin --profile web add github:andykwokatgithub/dsh-desktop   # DSH 插件途径

# 更新
npm update -g @andykwok/dsh-desktop   # npm 途径
dsh plugin --profile web update          # DSH 插件途径(自动激活升到新版本的 bundle)
```

机制要点(`@deepseek-ai/dsh-app-boot`):`dsh plugin` 在 profile 目录跑 `pnpm add/update`;
匹配到的依赖若声明了 `dsh.bundle`,会加入 `dsh.profile.bundles`;**更新时会自动激活
一个「在新版本才获得 `dsh.bundle` 声明」的包**。`postinstall`(`scripts/fetch-exe.mjs`)
会按 `package.json` 的版本,从相应 GitHub Release 下载 `dsh-desktop-win-x64.exe`,并用
同名 `.sha256` 校验后写到 `%LOCALAPPDATA%\dsh-desktop\dsh-desktop.exe`。

> **git 安装的一个坑**:通过 `dsh plugin add github:...` 安装时,pnpm 会禁止运行
> 依赖的构建脚本,直到你在 profile 的 `pnpm-workspace.yaml` 的 `allowBuilds`
> 里放行本包。命令失败时按 pnpm 打印的提示操作即可。我们刻意**不**把 Go 构建放进
> `prepare`,避免让安装者背负 Go + GCC 工具链。

---

## 5. GitHub Releases 直接下载 + 内置自更新

```powershell
# 下载
#   https://github.com/andykwokatgithub/dsh-desktop/releases → dsh-desktop-win-x64.exe

# 自更新
.\dsh-desktop.exe -check-update   # 仅报告是否有新版本
.\dsh-desktop.exe -update         # 下载并校验最释版,调度一个延迟任务替换自身后重启
```

实现:`internal/update`(语义化版本比较、`sha256` 校验、下载后原子写入)。`-update`
通过一个分离的 `cmd` 辅助脚本,等当前进程退出后复制新 EXE 并重启,避免锁定文件。

---

## 6. 许可与商标

- 主许可:**MIT**(见 `LICENSE`)。
- **商标另行声明**:项目内嵌 DeepSeek 鲸鱼图标,该图标与「DeepSeek」品牌名属于
  DeepSeek 方。MIT **不**覆盖品牌资产;任何人使用该 Logo/名称需另行取得许可。
  已在 `LICENSE` 的「补充声明」段落说明。若后续改用/去掉官方图标,请同步更新。

---

## 7. 已知限制与后续增强(诚实清单)

- **Harness 级入口尚未接线**:`cordis.patch.yml` 目前是空数组;要在 DSH UI/侧边栏里
  一键启动桌面壳,需要按基础配置的 `command`/`skill` 入口 id 与 schema 补补丁(后续)。
- **CI 工具链**:`webview_go` 需要 CGO;CI 里通过 choco 安装 mingw。不同 runner/镜像
  的 MinGW 路径可能不同,首次运行时按实际路径微调 `ci.yml`/`release.yml` 的 PATH 步骤。
- **npm token**:`NPM_TOKEN` secret 不在仓库里,需在 GitHub 仓库设置里配置后 npm 发布
  才会生效(否则仅发 GitHub Release)。
- **许可证归属/版权行**:`LICENSE` 用了 2026 与「contributors」占位,发布前请改成真实的
  版权主体与年份。

---

## 8. 发布前检查清单

- [ ] `LICENSE` 填好真实版权主体 MIT,商标声明就位。
- [ ] `package.json` 版本号、`bin`、`dsh.bundle`、`keywords` 正确;`os`/`cpu` 为 `win32`/`x64`。
- [ ] `README.md` 安装/更新命令、badge、许可、topic 说明齐全。
- [ ] `CHANGELOG.md` 本版条目已写入。
- [ ] `.github/workflows/ci.yml` 能通过(`go vet` + 构建)。
- [ ] 仓库已打 `dsh-plugin`(等)topic。
- [ ] 打 `vX.Y.Z` tag,确认 `release.yml` 产出 EXE + sha256 + zip,附到 Release。
- [ ] (可选)配置 `NPM_TOKEN`,确认 `npm publish` 成功、`npm i -g @andykwok/dsh-desktop` 可用。
- [ ] 未提交 `.gopath/`、`dsh-desktop.exe`、`*~` 等构建产物。
