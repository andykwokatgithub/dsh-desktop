# Changelog

所有对本项目的重大变更都会记录在此文件中。

本文件的格式基于 [Keep a Changelog](https://keepachangelog.com/zh-CN/1.1.0/)，
且本项目遵循 [语义化版本](https://semver.org/lang/zh-CN/)（Semantic Versioning）。

## [0.3.1] - 2026-09-14

### Added
- **窗口标题显示当前版本**:默认标题模板改为 `DeepSeek Harness Desktop {version}`,窗口标题栏(与任务栏)
  显示如 `DeepSeek Harness Desktop 0.3.1`,`{version}` 展开为运行时版本(`internal/config.Version`,可被
  `-ldflags -X` 覆盖后同步变化)。`--title` 支持 `{version}` 占位符;自定义标题若不含占位符则**原样显示**
  (不强行追加版本),空/空白标题回退为默认标题。解析集中在 `config.Config.WindowTitleText()`,`main.go`
  建窗时使用;`--help` 中文文案与 README 配置表同步更新。

### Fixed
- **`--stop-on-exit` 现在真的会关掉自有实例（此前可能"日志说已停止、实例仍在跑"）**:旧实现把归属全押在
  **记录中的 `listenerPid`** 上——一旦真正服务端口的进程换了 PID（服务重启/再次派生）或记录过期，
  `StopOwned` 会直接 `return nil`，关窗逻辑据此记下"已按 --stop-on-exit 停止自有实例"，而实例与端口原封
  不动（实测复现）。现在归属按**血脉**判定:主目标是本壳 spawn 的 `cmd.exe` **包装器进程树**
  （`spawnerPid`，随实例终生存在，`taskkill /F /T` 会连带终止当前服务端口的进程，哪怕它换了 PID），
  记录中的 `listenerPid` 作为第二目标；本次会话判为自有而记录不可用时，仍以"包装器是本壳直接子进程"
  为证终止。结果也不再含糊:端口上仍有实例却证明不了归属 ⇒ 返回 `ErrNotOwned`、**放弃终止**并记日志;
  实例确已退出 ⇒ 记"无需停止"；成功时日志列出被终止的 PID。`Endpoint` 新增 `Owned()` 作为
  `--stop-on-exit` 的准入闸门（本次 spawn 的内存归属 **或** 记录可证）。
- **已退出但句柄未关的进程不再被当作"存活"**:新增 `procinfo.Exited`（`WaitForSingleObject` 等句柄）——
  进程对象会因句柄未关而在进程死后继续应答 `OpenProcess`/`GetProcessTimes`，而 Go 的 `os/exec` 为每个
  子进程保留句柄。`processAliveAt` 现在同时要求"启动时间一致"且"进程未退出"，`service.Stop` 也不再对
  这类目标报出虚假的 `taskkill failed`（日志里出现过的那类错误）。新增 `procinfo.IsDescendantOf`，用于
  "当前监听者仍在本壳包装器进程树内"的判定。

## [0.3.0] - 2026-09-11

### Added
- **端口占用者识别层 `internal/procinfo`(纯 Win32,零子进程、零 WMI)**:新增
  `ListenerPID` / `ImagePath` / `CommandLine` / `StartTime` / `ParentPID` / `Ancestors` 与
  端口占用者身份 `Info`。实现要点:
  - `GetExtendedTcpTable(TCP_TABLE_OWNER_PID_LISTENER)` 取**监听 PID**——**不解析 `netstat -ano`
    文本**(表头随系统语言本地化,`findstr :3080` 会误配 `:30801`/对端地址/IPv6,且多一次进程开销);
  - `QueryFullProcessImageNameW` 取镜像路径,`NtQueryInformationProcess(ProcessCommandLineInformation)`
    取命令行(实测:类 60 会把字符串**复制进调用者缓冲区**,自包含 `UNICODE_STRING`,须先按其返回长度
    分配;同用户进程**免提权、免 `ReadProcessMemory`**),因此**不需要 spawn PowerShell/CIM**;
  - `dsh.go` 提供 DSH 关键字匹配,分**强证据**(`@deepseek-ai\dsh` / `deepseek-harness` / `dsh web` /
    `…\dsh\lib\bin.js`;`/` 与 `\`、重复分隔符、大小写均已归一)与**弱证据**(裸 `dsh`、`--port <端口>`,
    本壳自身的 `dsh-desktop.exe --port …` 也会命中);弱证据不足以判定为 dsh。
- 新增 `internal/procinfo` 单元测试:关键字匹配(用真实命令行样本:node 监听者 / npm shim `cmd /c "dsh web …"` /
  本壳自身 / 无关进程)、监听 PID(自建监听器=本进程、通配绑定回退、已关闭端口报"未知")、
  镜像/命令行/启动时间/父进程/祖先链,以及 MIB 表端口字节序解码。
- **token 附着既有 DSH 实例(FR-05)**:端口上是**非本壳启动**的 DSH 实例时,先按持久化 cookie 附着(升级后旧实例没有归属记录也不至于一上来就要 token);**若取不了验证**(命中认证栅栏)则在窗口内显示 `internal/ui/token.html`,让用户粘贴 `dsh web` 打印的整条认证 URL(或裸 token):
  - `internal/service.ParseTokenInput`:兼容"裸 token / 整条 URL / 夹在其它文本里的 `token=`",并按 base64url 形状校验;
  - `internal/service.ValidateToken`:用 no-redirect 客户端 `GET /?token=…`——`303/200` ⇒ 有效并 `Navigate` 完成 token→cookie 交换,`401` ⇒ 页内报错可重试,传输错误 ⇒ 提示"无法连接"而**不误判为 token 无效**;
  - 页面另提供"改用本应用自己的实例"(≥10000 端口自建);页面停留期间不 spawn、不终止对方;token **不落盘、不入日志**。
- **实例归属记录(`internal/service/endpoint.go`)**:`%LOCALAPPDATA%\dsh-desktop\web-endpoint.json` 存 `{host,port,listenerPid,listenerStartedAt,spawnerPid}`;`Endpoint.Live()` 用**监听 PID + 进程启动时间(±1s)** 判定"是不是本壳启动的"(PID 复用不会误判),`spawnerPid` 用于 `taskkill /T`。
- **回退端口(`internal/service/ports.go`)**:`PickFreePort(host, 10000)` 在 `[10000,65535]` 取当前可用端口。
- **HTML 模式窗体桥接(`internal/webview`)**:认证栅栏探测脚本(`__dshAuthRequired`)+ 页面桥接(`__dshTokenTarget`/`__dshSubmitToken`/`__dshSkipToken`/`__dshRetry`);`error.html` 改为可注入原因 + 可重试(不再 `location.reload()`);`loading`/`error`/`token` 全部为内嵌本地页。
- **`internal/appdir`**:统一 `%LOCALAPPDATA%\dsh-desktop` 状态目录(日志/端点记录),`LOCALAPPDATA` 缺失时回退临时目录。
- **测试**:`token_test.go`(解析 + 模拟 dsh 闸栏的三种校验结果)、`ports_test.go`、`endpoint_test.go`(含 PID 复用防护)、`internal/ui/ui_test.go`(页面↔桥接契约、页面不加载远端资源)、`internal/webview/shell_test.go`(栅栏脚本与绑定名),以及 config 的 `--url` 派生/token 用例。

### Changed
- **`--stop-on-exit` 只作用于自有实例**:关闭窗口时**不再**可能停掉用户自己启动（或本壳只是附着）的 dsh 实例;并且终止前必须能**证明归属**——新增 `service.StopOwned` 只接受"记录中的 `listenerPid` + 启动时间仍一致"或"该 PID 是本壳的**直接子进程**"(`procinfo.IsChildOf`),证明不成立时**放弃终止**并记日志,避免 PID 复用误杀无关进程。端点记录新增 `spawnerStartedAt`;`service.Stop` 现在对真正失败的终止返回错误(进程已退出不算失败)。`--stop-on-exit` 遇到附着实例时会在日志里说明"按归属不停止"。
- **启动流程重构(窗口先建、HTML 页承载交互)**:`main.go` 现在先建窗口并显示 loading,再由后台 goroutine 决策——自有实例复用 / `--url` token 附着 / **非本壳 DSH ⇒ 先按 cookie 附着,取不了验证(栅栏)再转 token 输入页** / 端口被占 ⇒ `[10000,65535]` 自建 / 空闲 ⇒ 首选端口自建;**任何分支都不终止端口占用者**。端口被占时不再弹原生错误框退出(占用者身份改为写入日志),原生 MessageBox 只保留给窗口创建前的失败(单实例/建窗/参数错误)。
- **认证栅栏自愈**:复用自有实例时若页面命中 `dsh web authentication required`(cookie 失效/过期),自动停掉自有实例并以新 token 重新拉起;**非本壳**实例则转 token 输入页(不再把用户留在 401 文本页)。
- **`--url` 语义**:未显式指定 `--host`/`--port` 时,`--url` 的 host/port 被采纳(`--url "http://127.0.0.1:34567/?token=…"` 可单独使用);显式端点与 URL 冲突仍报错;URL 必须含显式端口;新增 `Config.URLToken()`/`HostSet`/`PortSet`。
- `--host 0.0.0.0` 在参数校验阶段即报错(dsh 顶层已拒绝该绑定)。
- **README 对齐本轮实现**:`README.md` 新增「实例识别与安全边界」一节(L1/L2/L3 判据、`StopOwned`
  终止证明、任何分支都不终止端口占用者、token 不落盘),典型场景补齐"附着他人实例 / token 输入页 /
  端口被占改用回退端口 / 认证栅栏自愈",配置表更新 `--url`(采纳 host/port、必须含显式端口)与
  `--stop-on-exit`(只停归属可证的自有实例)的口径,并补系统语言文案、状态目录与目录结构说明;
  `--help` 的中文 `--url`/`--stop-on-exit` 描述同步更正。
- **文档**:`docs/dsh-desktop-prd.md` V1.2 的 FR-01 判据改为 L1/L2/L3 三层(§1.1 补进程形态实测事实、
  FR-04 归属记录字段与 NFR 安全行同步);**新增两项交互需求**:FR-02 统一为 **HTML 模式窗体**
  (`loading`/`error`/`token`/`update`/`updating` 内嵌本地页 + `Bind` 桥接,取消原生 MessageBox,
  仅窗口创建前的参数错误例外),FR-05 改为**取不到验证时显示 HTML token 输入页**(用户粘贴
  `dsh web` 打印的认证 URL/token 即可附着该实例,或选择改用 ≥10000 端口自有实例,页面停留期间
  不 spawn、不终止对方);FR-06 的更新询问/进度/失败也改为 HTML 页。`AGENTS.md` 更新目录结构与关键事实。
- **CLI 文案按系统语言显示中文或英文**:`internal/config` 新增区域检测(`useChineseUI` / 导出的
  `UseChineseUI`),优先取 `LC_ALL`/`LC_MESSAGES`/`LANG` 环境变量,未设置时回退到 Windows
  用户界面语言(`GetUserDefaultUILanguage`,主语言为 `LANG_CHINESE` 即判为中文)。据此让:
  - `--help` 输出中文或英文的用法/选项/默认值文案;
  - `--check-update` / `--update` 的控制台输出(标题与主要提示)同样按系统语言显示中文或英文。
- 新增 `internal/config/locale_test.go`,覆盖中文/英文语言 ID 与区域名判定、locale 优先级及
  `LC_ALL` 覆盖 `LANG` 的行为。

## [0.2.9] - 2026-09-07

### Changed
- **自更新下载更健壮(支持本地代理与国内镜像回退)**:GitHub release 的**二进制资产**由 CDN 下发,
  在部分网络(尤其国内)直连被限速/阻断,导致 `--update` 在下载 15.3MB 的 exe 时撞上 60 秒
  超时报 `context deadline exceeded`。现改为:
  - **候选顺序失败回退**:官方直连 →(若检测到本地 HTTP 代理,env 或 `127.0.0.1:7890` 等常见
    Clash/V2Ray 端口)**经该代理** → **国内镜像**(`ghfast.top`/`ghproxy.net`/`ghproxy.com`/
    `gh-proxy.com`)。首个成功者胜出。
  - **每次尝试独立超时**(45s)、每候选 1 次,避免单个卡死的 host 吃完整个更新窗口。
  - **取版本/取 SHA 回退到默认客户端**,不再强制走本地代理(`api.github.com` 本身直连可用,
    避免把这项也弄坏)。
  - **SHA256 校验不受影响**:`FetchSHA` 走同一套回退取官方校验和,镜像下载的二进制仍按官方
    SHA256 校验,镜像无法投放未校验的产物。
  - 顶层超时由 60s 放宽到 **12 分钟**,并给「解析版本 / 取 SHA」单独 **2 分钟**预算(不拖住整体)。
- **新增 `internal/update/update_test.go`**:覆盖 `SemverCompare`/`NewerThan`/`LoadSHA`/
  `Release.Version` 与 `Release.AssetURL`。

### Changed
- **`--context-menu` 默认开启**:`ContextMenu` 默认值由 `false` 改为 `true`,即默认保留 WebView2 右键菜单
  (此前 D7 安全默认关闭);如需禁用可传 `--context-menu=false`。
- **修正版本号默认值**:`internal/config.Version` 默认值从遗留的 `0.1.0` 更新为当前发布版 `0.2.7`(与
  `package.json` 及 `[0.2.7]` 条目一致),避免 dev 构建误报旧版本、把已发布版本当成"可更新"。
- **`--update` 更新后不再自动拉起窗体**:`applyUpdate` 不再 `start "" target` 自动重启,改为更新
  完成即退出(纯 CLI,与帮助文案"下载并应用最新版本,然后退出"一致);用户需重新运行 dsh-desktop
  以使用新版本。
- **统一参数用法为双横杠**:CLI 参数的规范写法统一为 `--flag`(如 `--check-update`/`--port`),
  `--help` 也按双横杠输出用法;为兼容旧用法,`flag` 包仍接受单横杠 `-flag` 输入。
- **自更新命令改为控制台输出**:`--check-update` / `--update` 的结果不再弹出**原生提示框**,
  而是直接输出到**控制台**(信息走 stdout、错误走 stderr),同时保留写入
  `%LOCALAPPDATA%\dsh-desktop\dsh-desktop.log`。这两个命令是纯 CLI 命令,在打开 GUI
  窗口前执行并退出,因此改为文本输出以便终端用户与脚本读取。
- **改为控制台子系统构建(替代 GUI 子系统)**:`build.ps1` 不再用 `-H=windowsgui`,EXE 以
  **console 子系统**构建,使 `--version`/`--check-update`/`--update` 在 PowerShell/cmd 中像
  普通命令一样**同步等待并内联输出**(不再有"无输出"或"按任意键"的副作用)。GUI 启动时新增
  `main.hideConsole()`:`GetConsoleProcessList` 判断该控制台是否为**本进程独有**(双击产生),
  是则 `ShowWindow(SW_HIDE)` 隐藏控制台窗口,避免黑框;若为复用父终端控制台则不动它。
  (注:从已有终端启动 GUI 时,PowerShell 会阻塞直到窗体关闭,或激活现有实例后快速退出。)
- **规范参数传递**:统一 CLI 参数的流向与校验。
  - **统一端点来源**:以 `--host`/`--port` 为唯一事实来源,页面地址自动派生(`CanonicalURL`);
    `--url` 改为**可选显式覆盖**,设置时必须与 `--host`/`--port` 一致,否则报错(修复了「改 `--port`
    后 WebView 仍导航到旧地址」的隐患)。
  - **清理失效字段**:删除 `service.Behavior.URL` 与 `singleinstance.WindowOptions.URL`(及其
    `window.go` 里永不触发的导航分支),把页面导航收敛到 `main.go` 一处。
  - **集中单位换算**:`Config` 新增 `StartupTimeoutDuration()`/`PollIntervalDuration()`,把
    `main.go` 里分散的 `time.Duration(...)` 换算收拢;`StartupTimeout` 字段更名为
    `StartupTimeoutSec` 以显式表达单位。
  - **统一命名**:`WindowOptions.Debug` 更名为 `WindowOptions.DevTools`,与 `--devtools` 保持一致。
  - **增加输入校验**:`config.Parse` 对 `--port`(1-65535)、`--width`/`--height`(>0)、
    `--startup-timeout`(>0)、`--poll-ms`(>0)、`--host`(非空)与 `--url`(与 host/port 一致)做
    语义校验,值非法即报错退出。
  - **配错呈现**:参数/配置错误在 **CLI 命令**(`--version`/`--check-update`/`--update`)下输出到
    **控制台**,在 **GUI 启动**下保留**原生对话框**(无控制台,需可见错误)。
- **新增 `internal/config/config_test.go`**:覆盖默认值、`CanonicalURL`/`PageURL`/时长换算与
  参数校验(`validate`),含 `--url` 一致性校验与单/双横杠兼容用例。

## [0.2.6] - 2026-09-06

### Fixed
- **服务就绪判定兼容 dsh web 的浏览器认证**:`dsh web` 现在用**每次进程随机生成的启动 token**对
  Web UI 做浏览器认证(访问地址形如 `http://127.0.0.1:3080/?token=…`)。旧代码对根路径
  `GET /` 只认 **HTTP 200**,而带认证的 dsh web 对无 token、无 cookie 的裸请求返回**401**
  (`dsh web authentication required;…`),导致桌面壳误判为「端口被其它进程占用,无法启动
  dsh」。现将健康校验改为**识别 dsh web 的认证边界**:200 / 303(token→cookie 交换)视为健康,
  401 且响应体带 `dsh web` 签名时也视为健康,从而正确「复用」已在运行的 dsh 服务,而非误报端口冲突。
- **冷启动导航到带 token 的认证地址**:服务由本壳 spawned 时,从子进程 stdout 的
  `dsh web: <url>` 就绪行解析出**带启动 token 的认证 URL**,并让 WebView2 导航到该地址,从而完成
  token→cookie 交换并展示真实 UI(而非 401 文案页)。冷却复用路径仍导航到 `cfg.URL`,由 WebView2
  持久化 cookie(位于 `%APPDATA%\dsh-desktop.exe`)完成授权。
- 新增 `internal/service/service_test.go`,对认证边界的健康判定与就绪行 URL 解析做单元测试。

## [0.2.5] - 2026-09-03

### Added
- `docs/dsh-desktop-readme-prd.md`：README 需求基线（README PRD），按 GitHub README
  最佳实践给出 README 重写的章节/结构/质量/验收要求。

### Changed
- 依 README PRD 重写 `README.md`：新增「演示」占位、「是什么/为什么/边界」、读者分层环境表、
  配置表格（全量 flags + 默认值）、「贡献与安全」入口；将「发现与收录/topic」下沉到
  `docs/publish.md`，用配置表替代平铺 flags。
- `docs/publish.md` §1.4「README 与元数据」：同步 README 改动后的引用口径（README 不再含
  topic 说明，市场/发现细节以 publish.md 为准）。

## [0.2.4] - 2026-09-03

### Changed
- **自更新命令的可见反馈**:`-check-update` / `-update` 的结果改由**原生提示框**展示
  (因为该 EXE 是 GUI 子系统、无控制台,`fmt.Printf` 进不了 PowerShell),同时写入
  `%LOCALAPPDATA%\dsh-desktop\dsh-desktop.log`。

## [0.2.3] - 2026-09-03

### Changed
- **移除 install 脚本**:不再有 `postinstall`,从而兼容开启了严格 `allow-scripts` 策略的
  npm(例如默认拦截安装脚本,且 `--allow-scripts` 在部分 npm 版本有 bug)。EXE 已内置,
  改为由 `bin/dsh-desktop.mjs` 启动器在**首次运行 `dsh-desktop` 时**从包内拷贝到
  `%LOCALAPPDATA%\dsh-desktop\dsh-desktop.exe`,零网络、零安装脚本。

## [0.2.2] - 2026-09-03

### Changed
- **npm 包改为内置预编译 EXE**:`dsh-desktop-win-x64.exe`(约 14.6MB)直接打进
  npm tarball(`files`),postinstall 只做**本地拷贝**,不再在安装时联网下载,从而规避
  Node `fetch` 不走系统代理/TLS 限制导致的 `fetch failed`。`bin/dsh-desktop.mjs` 与
  `scripts/fetch-exe.mjs` 均优先使用包内置二进制,缺失时才回退到 GitHub 下载(带 UA)。

## [0.2.1] - 2026-09-03

### Fixed
- **npm 安装拉取 EXE**:`scripts/fetch-exe.mjs` 的 `readVersion()` 误用
  `dirname(import.meta.url)`(URL 字符串)导致读不到 `package.json` 版本、回退成 `0.1.0`
  并下载不存在的 `v0.1.0` 资产;已改用 `fileURLToPath` 正确解析包目录,并让下载源在
  精确版本资产缺失时**回退到 latest release**。发布为 `0.2.1`。

## [0.2.0] - 2026-09-02

### Added
- `AGENTS.md`：面向 AI 代理与贡献者的仓库速查指南（工具链/构建/结构/坑点/约定）。
- **发布/分发打通**：新增 `LICENSE`（MIT，含 DeepSeek 图标商标声明）、`package.json`（npm 包，声明 `dsh.bundle.patch`）、`cordis.patch.yml`、`bin/dsh-desktop.mjs` 启动器、`scripts/fetch-exe.mjs`（下载并 SHA256 校验预编译 EXE）。
- **发布流水线**：`.github/workflows/ci.yml`（go vet / 构建 / npm 清单校验）与 `.github/workflows/release.yml`（tag 触发：注入版本号构建、生成 `sha256`、附到 GitHub Release、可选 `npm publish`）。
- **内置自更新**：新增 `internal/update`（GitHub Releases 解析、语义化版本比较、带校验的下载与原子替换）与 `-check-update`、`-update` 两个 CLI flag。
- `build.ps1` 新增 `-Version` 参数，用于在构建时通过 `-ldflags -X .../config.Version` 注入版本号。
- 文档：`docs/publish.md`（发布/收录/安装/更新指南）。

### Fixed
- 二次实例激活时不再改动主窗口几何：移除 `activateWindow` 回退路径中无条件的 `ShowWindow(SW_RESTORE)`，改为仅置顶（`SetForegroundWindow` + `AttachThreadInput` 回退）；最小化窗口的恢复（回到用户此前大小）仍由 `BringToFront` 处理。
- 外部链接交接不再闪黑色控制台：`internal/webview/shell.go` 打开系统默认浏览器时给 `cmd /c start` 补上 `CREATE_NO_WINDOW`（与后端服务 spawn 一致），避免每次 Ctrl+点击外部链接闪现 `cmd` 黑框。

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
- 文档：需求基线 PRD（`docs/dsh-desktop-prd.md`，V1.1）与技术设计（`docs/dsh-desktop-technical-design.md`）、`CHANGELOG.md`。
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
