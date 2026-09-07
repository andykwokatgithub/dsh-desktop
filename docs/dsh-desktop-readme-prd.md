# dsh-desktop README 需求文档（README PRD）

| 文档版本 | 修改日期 | 修改人 | 修改内容 |
| :--- | :--- | :--- | :--- |
| V1.0 | 2026-09-03 | AI Assistant | 依据对现有 `README.md` 的全面审阅与 GitHub README 最佳实践，给出 README 重写的需求基线。 |

> 配套文档：[`README.md`](../README.md)（现状）、[`dsh-desktop-prd.md`](./dsh-desktop-prd.md)（产品 PRD V1.1）、[`dsh-desktop-technical-design.md`](./dsh-desktop-technical-design.md)、[`publish.md`](./publish.md)。
> 本文的读者：README 作者 / 维护者，以及想在仓库中一次性把 README 写对的人。

---

## 0. 背景与目标

### 0.1 为什么要重写 README

`README.md` 是 GitHub 仓库的“门面”与“第一印象”。本项目同时面向**终端用户**、**插件市场收录方**、**贡献者**三类人，但现状 README 把大量“面向市场/发布”的运维信息（topic、`dsh.bundle` 校验、发版流程）与“面向用户”的信息混在一起，缺少最高价值的**视觉证据（截图/演示）**、**清晰的价值主张（为什么用不用它）** 以及**面向贡献者的入口链接**。审阅结论见 §1。

### 0.2 目标

让 `README.md` 用最小篇幅、按最佳实践结构，回答四件事：

1. **这是什么**（一句话定位 + 价值主张 + 边界）。
2. **我能得到什么**（功能亮点 + 截图，尤其是 GUI 产品的视觉证据）。
3. **我该怎么用**（三类安装通道 + 快速上手 + 配置表）。
4. **我该怎么参与 / 如何合规**（构建、贡献、许可、安全）。

---

## 1. 现状 README 审阅结论（问题清单）

> 结论先行：当前 README **内容真实性高、结构可用**，但**视野偏“运维/市场”**，缺少面向用户的可见性证据与清晰的读者分层。以下是按“最佳实践”逐条对照的差距。

### 1.1 强制项缺失/薄弱

| 编号 | 最佳实践要求 | 现状 | 建议 |
| :-- | :-- | :-- | :-- |
| G-01 | **Hero 视觉证据**（截图/GIF） | ❌ 无任何截图/演示图 | GUI 产品必须加截图或动图（放在标题下方/“是什么”之后） |
| G-02 | **一句话为什么（Why）** | 只有“是什么”，没有“为什么/值不值得用” | 补一页“和 `dsh web` 浏览器标签页相比好在哪” |
| G-03 | **面向贡献者的入口** | “文档”一节未链接 `CONTRIBUTING.md`/`SECURITY.md` | 增加 **Contributing**、**Security** 两个小节/链接 |
| G-04 | **配置表** | flags 是平铺列表，无默认值、无口径 | 改为 `flag / 默认值 / 含义` 表格（默认值见 `internal/config`） |
| G-05 | **跨文档链接** | 未链 `CHANGELOG.md` | 在“文档/版本”处链接 CHANGELOG（Keep a Changelog） |

### 1.2 结构与分层问题

| 编号 | 问题 | 建议 |
| :-- | :-- | :-- |
| G-06 | “发现与收录（面向插件市场）”与 `publish.md` 高度重复，且面向第三方，不属于终端用户 | 精简为 2-3 行并链到 `docs/publish.md`，避免在两处维护 |
| G-07 | 无 **Table of Contents**；随内容增长会越来越难扫 | 加 `## 目录`（或 GitHub 自动 anchor 导航） |
| G-08 | “环境要求”未按读者分层（终端用户只需 EXE+WebView2；运行才需 `dsh`） | 用“谁需要什么”拆解，或加注释说明 |
| G-09 | 缺少 **License 之外的第三方品牌/商标说明**的显式位置 | 已在“许可”一节有商标声明（保留），但应单列小节更醒目 |

### 1.3 准确性 / 一致性风险（审阅时发现，建议在重写时顺带核对）

| 编号 | 风险 | 建议 |
| :-- | :-- | :-- |
| G-10 | **模块路径与仓库地址不一致**：`go.mod` 的 module 为 `github.com/deepseek-ai/dsh-desktop`（`internal/config` 的 ldflags 注释亦如此），而 README/Badge/npm/插件均指向 `andykwokatgithub/dsh-desktop` / npm `@andykwok/dsh-desktop` | 确认归属后统一为同一可达路径；至少加一行说明，避免读者 confused |
| G-11 | **版本默认值口径**：`internal/config.Version` 默认 `0.1.0`，但已发布到 `0.2.4`；README “默认 0.1.0”易误导 | 写明“代码默认/发布版本”的差异，或不再强调序列 |
| G-12 | **Badge 目标**：npm badge owner=`andykwok`、GitHub release badge owner=`andykwokatgithub` | 核对两者是否为同一主体；如为两个账号需说明 |

---

## 2. README 的定位与读者

| 读者 | 核心诉求 | README 为其提供的价值 |
| :-- | :-- | :-- |
| **终端用户**（下载 EXE） | 会不会用、要不要装、装完怎么点开 | 安装 + 启动 + 截图 + 更新命令 |
| **走 npm / `dsh plugin` 的用户** | 怎么一键装、怎么更新、命令是什么 | npm/插件安装与更新命令、注意事项 |
| **插件市场 / dshmarket 收录方** | 是否可安装、怎么发现 | topic、`dsh.bundle` 声明、版本一致（**精简后链到 publish.md**） |
| **贡献者 / AI 代理** | 怎么构建、怎么提 PR、有什么坑 | 构建命令、目录结构、CGO/windowsgui 要点、链接到 CONTRIBUTING.md |

> 设计原则：**主 README 服务“用户”，把“运维/市场/构建”细节下沉到 `docs/` 与 `CONTRIBUTING.md`**。

---

## 3. 内容需求（按章节）

以下每章节给出“必填 / 选填 / 不建议”与要点。

### 3.1 标题区（必填）
- 项目名：`dsh-desktop`。
- **一句话副标题**：`Windows 桌面壳（Go + WebView2），把 dsh web 的 Web UI 装进原生窗口。`
- **Badge 行**（选填但建议保留并校正）：License(MIT)、Go 1.21、Platform(Windows x64)、GitHub release、npm version。**须与 §1.3 G-10/G-12 核对一致。**

### 3.2 “是什么 / 为什么”（必填）
- **是什么**：3-4 条能力子弹（内嵌渲染 / 服务生命周期 / 单实例 / 无控制台 GUI 体验）。
- **为什么（新增）**：短段说明“相比在浏览器打开 `dsh web`，本壳提供独立窗口、单实例防重、路径复用、无黑框体验”。
- **边界（Non-goals）**：一句话“不承载 Harness 逻辑，只做壳；`/api` 能力由 dsh 承担”。

### 3.3 演示 / 截图（必填，本轮补充）
- 放 1-2 张 1280px 宽的截图或轻量 GIF：主窗口加载 DSH UI、加载态/错误态。
- 资源放 `assets/`（如 `assets/screenshot-main.png`），README 里用相对路径引用。

### 3.4 目录（选填，README 较长时必加）
- `## 目录`，链接到主要章节（安装、使用、构建、许可、贡献）。

### 3.5 安装与更新（必填）
三种通道各一节，覆盖“终端用户 / npm+plugin / 源码”：
1. **GitHub Releases**：下载 `dsh-desktop-win-x64.exe`；`--check-update` / `--update` 自更新；注明“这两个命令是纯 CLI 命令，结果直接输出到控制台（stdout/stderr），并写入日志”。
2. **npm / `dsh plugin`**：`npm i -g @andykwok/dsh-desktop` 与 `dsh plugin --profile web add github:andykwokatgithub/dsh-desktop`；`npm update -g ...` / `dsh plugin ... update`；说明命令是 `dsh-desktop`（不带 `.exe`）、无 install 脚本、首次运行拷贝到 `%LOCALAPPDATA%`。
3. **从源码构建**：链到 §构建。

### 3.6 环境要求（必填）
- 按“谁需要什么”分层：
  - **终端用户**：Windows 10 1803+/Win11、WebView2 Evergreen（Win11 预装）。
  - **运行时（所有场景）**：全局安装 `dsh`，`node`/`dsh` 在 `PATH`。
  - **构建（贡献者）**：Go 1.21+（CGO）+ GCC/mingw-w64；重建图标才需 Node+`sharp`。

### 3.7 使用 / 快速上手（必填）
- 两行启动命令（`dsh-desktop` 与 `.\dsh-desktop.exe`）＋典型场景（热启动/冷启动/重复启动）一句话。
- **配置表（新增，替代平铺 flags）**：`flag / 默认值 / 说明`，逐行列出 `internal/config` 中的全部项（url/host/port/command/stop-on-exit/devtools/context-menu/startup-timeout/poll-ms/width/height/title/version/check-update/update）。

### 3.8 构建（贡献者，必填）
- `.\build.ps1`；等价 `go build -o dsh-desktop.exe .`（默认 console 子系统,无需 `-H=windowsgui`）。
- `-Version X.Y.Z` 注入；`go run . -version` 验证。
- 保留 **CGO / console 子系统(GUI 隐藏控制台) / 不要盲目运行** 三条注意事项。
- 图标管线：`assets/deepseek.ico` → `rsrc` → `.syso`。

### 3.9 目录结构（选填，保留并精简）
- 保留 §目录结构；标注到 `internal/` 各包与 `docs/`、`.github/workflows/`。

### 3.10 贡献 / 安全（必填——新增）
- **Contributing**：一行链接到 `CONTRIBUTING.md`（环境/开发/提交约定/发布入口）。
- **Security**：一行链接到 `SECURITY.md`（漏洞私报、校验与品牌资产关注点）。

### 3.11 许可与商标（必填，保留）
- MIT License + 显式小节：“DeepSeek 图标与品牌资产归 DeepSeek 方所有，MIT 不授予商标/Logo 使用权，需另行许可”。

### 3.12 文档 / 关联（必填）
- 链接：PRD、技术设计、`publish.md`、`CHANGELOG.md`（新增）。

### 3.13 支持 / 路线图（选填）
- 可选：简短“未来规划”（跨平台、托盘、偏好设置、自动更新），链到产品 PRD §10。

---

## 4. 结构与组织要求

- **标题层级**：`#` 仅标题，正文用 `##`；章节顺序按 §3：标题区 → 是什么/为什么 → 截图 → 目录 → 环境要求 → 安装 → 使用 → 构建 → 目录结构 → 贡献/安全 → 许可 → 文档。
- **“用户优先”**：把“安装/使用/截图”放前面，“构建/市场/发布”放后面或下沉。
- **长度**：目标 120–200 行（现状 188 行，重写后应更聚焦；把运维细节交给 `docs/`）。
- **链接**：内部链接用相对路径；指向 GitHub 的链接用绝对 URL。

---

## 5. 质量与风格要求

| 维度 | 要求 |
| :-- | :-- |
| **准确性** | 每条命令、flag、路径、环境要求均与源码/`docs/` 一致；杜绝“官网/别的工具”式空话。 |
| **真实性** | 不夸大（如“极快”“最稳”等绝对化）；明确“什么不做”。 |
| **可扫读** | 多用子弹与表格；每节 ≤ 5-8 行；双空白行分隔章节。 |
| **语言** | 以**中文**为主（与仓库/PRD 一致）；如需国际覆盖，另加 `README.en.md` 或双语句（决策点见 §7 Q1）。 |
| **一致性** | Badge、仓库名、npm 包名、模块路径、默认版本号口径统一（见 §1.3）。 |
| **代码块** | 命令用 ``` ```powershell ```；配置表用 Markdown 表格。 |

---

## 6. 非功能性需求

- **渲染**：Markdown 在 GitHub Web 与移动端均可读；无超宽表格。
- **图片**：截图 ≤ 1MB，GIF/图用相对路径资源。
- **可访问性**：对比度足够；重点用 `**加粗**`，避免纯颜色/emoji 承载信息。
- **可维护性**：README 与 `docs/`、`CONTRIBUTING.md` 单一真相；`publish.md` 作为市场/发布权威。
- **版本一致性**：README 中的命令/默认值随版本变更同步更新（与 CHANGELOG 一起）。

---

## 7. 开放问题 / 决策点（重写前需拍板）

| # | 问题 | 选项 | 建议 |
| :-- | :-- | :-- | :-- |
| Q1 | **语言** | 仅中文 / 中英双语 / 英文为主 | 仓库与用户目前面向中文，**建议先仅中文**；若需国际覆盖再补 `README.en.md` |
| Q2 | **截图来源** | 用真实运行截图 / 先用占位图 | 用**真实截图**（GUI 产品最高价值）；需在能拉起 `dsh web` 的环境截一次 |
| Q3 | **“发现与收录”去留** | 保留精简版 / 完全移入 `publish.md` | 建议**精简为 2-3 行 + 链到 publish.md**，避免双处维护 |
| Q4 | **模块路径 vs 仓库地址**（G-10） | 统一为 `deepseek-ai` / 统一为 `andykwokatgithub` / 加说明 | **先核对归属**，再统一或显式说明，别留给读者猜 |
| Q5 | **配置表完整度** | 全量列出全部 flags / 只列常用 | **全量列出**（README 是配置权威之一），并标注默认值 |
| Q6 | **架构图/原理** | 加一张简单流程图 / 不加 | 可选用 `<details>` 折叠一张“壳→服务→窗口”简图 |

---

## 8. 验收标准（AC）

- [ ] **AC-01**：README 首屏（标题 + 副标题 + 一张真实截图）即能让读者明白“这是什么”。
- [ ] **AC-02**：有“为什么用（与浏览器打开 dsh web 的对比）”与“边界（不做什么）”两段。
- [ ] **AC-03**：三种安装通道各一节，命令正确、可复制；自更新行为说明清晰。
- [ ] **AC-04**：配置以表格展示，列全 `internal/config` 全部 flags 与默认值。
- [ ] **AC-05**：环境要求按“终端用户 / 运行时 / 构建者”分层，无歧义。
- [ ] **AC-06**：包含 **Contributing** 与 **Security** 两个小节/链接；文档区链接 `CHANGELOG.md`。
- [ ] **AC-07**：许可与商标声明独立成节；Badge、仓库名、npm 名、模块路径口径统一。
- [ ] **AC-08**：README 长度收敛到 ~120–200 行；“发现与收录”等市场信息瘦身并链到 `publish.md`。
- [ ] **AC-09**：每条命令/路径/默认值经与源码核对；不引用不存在的信息。

---

## 9. 非目标（本次 README 重写不做）

- 不新增产品功能（本 PRD 只针对 README 文档形态）。
- 不重写产品 PRD / 技术设计（仅引用）。
- 不提交 `dsh-desktop.exe`、`.gopath/`、`*~` 等构建产物。
- 不建立 Issue/PR 模板（如需另开任务）。

---

## 10. 后续迭代建议

- 若决定国际化：新增 `README.en.md`，并同步 `package.json`/市场元数据的英文描述。
- 若 README 持续变长：拆出 `docs/usage.md`、`docs/configuration.md`，README 只保留入口。
- 增加 `.github/ISSUE_TEMPLATE/` 与 `CONTRIBUTING` 联动（可选）。
