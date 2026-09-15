
# dsh-desktop 技术设计文档

| 文档版本 | 修改日期 | 修改人 | 修改内容 |
| :--- | :--- | :--- | :--- |
| V1.0 | 2026-09-02 | AI Assistant | 依据 PRD V1.1 与审阅意见，固化推荐设计决策，给出技术实现骨架与伪代码。 |

> 配套文档：[`dsh-desktop-prd.md`](./dsh-desktop-prd.md)（需求基线）。
> 依据的 `dsh` 实现事实见 PRD §1.1；本文所有 Windows API 与 Go 库用法以"推荐实现 + 需互操作处标注"方式给出。

---

## 0. 概述与目标
本 Wrapper 是 `dsh web` 的**桌面壳**：只做窗口管理、服务生命周期、单实例防重；不复制 Harness 本体。本设计文档将三项推荐决策固化为实现方案，并给出每个 FR 的可执行骨架。

---

## 1. 推荐设计决策（汇总，已定稿）

| # | 决策点 | 推荐选择 | 理由 |
| :-- | :-- | :-- | :-- |
| D1 | 生命周期 | **服务持久化复用 + 复用前健康校验 + 主动停止**；提供"关闭窗口即退出"配置项 | 保留 AC-03 意图，同时补齐停止/陈旧恢复；两种仅影响驻留与二次速度 |
| D2 | 命名空间 | **`Local\DSHDesktopApp`** | 避免 `Global\` 跨会话导致多用户/RDP 第二个用户无法启动；文档记录取舍 |
| D3 | 单实例定位 | **唯一原生窗口类** + `RegisterWindowMessage` 消息驱动激活（不靠窗口标题） | 标题是用户可配置的显示字段（`--title`，默认带 `{version}`），不保证唯一；类名/消息定位更稳 |
| D4 | 服务判据 | **端口 + HTTP 200 健康校验**双判据 | 区分"dsh 在跑"与"其它进程占端口 / 僵死" |
| D5 | 服务就绪 | 轮询健康校验 + 捕获 `dsh web: <url>` stdout 行 | 避免 TCP 可连但前端/`/api` 未就绪的白屏 |
| D6 | Windows spawn | 经 `cmd /c` 解析 npm shim（`.cmd`/`.ps1`），继承 `PATH`/`DSH_HOME` | Go `os/exec` 不按 PATHEXT 解析 `.cmd` |
| D7 | 安全 | 本地源白名单 + 拦截外部导航 + 默认关 DevTools/右键菜单 | `/api` 具备执行代码/读写 workspace 能力 |
| D8 | 加载态 | 先 `loading.html`，健康后 `Navigate` | 满足"窗口 ≤3s 出现加载态" |

---

## 2. 系统架构

```
┌─────────────────────────── dsh-desktop.exe（Go + CGO） ───────────────────────────┐
│                                                                                  │
│  [main]                                                                          │
│    ├─ SingleInstance(FR-03)  ── Mutex(Local\) + 原生锚点窗口(自定义类+WndProc)      │
│    │        │                     │                                                 │
│    │        │ 首次              └─(重复触发)→ FindWindowW(类) → SendMessage(激活)→退出│
│    │        ▼                                                                      │
│    ├─ ServiceProbe(FR-01)  ── TCP(3080) + GET / → 200 ?                            │
│    │        │ 未运行 → spawn("cmd /c dsh web --no-open") ──► 子进程 stdout/就绪行    │
│    │        ▼                                                                      │
│    ├─ WebViewWindow(FR-02) ── webview.New / WebView2 加载 loading.html → 跳转 3080   │
│    │        │ 导航白名单 / DevTools / 右键 控制(FR-02,§4.6)                          │
│    │        ▼                                                                      │
│    └─ Lifecycle(FR-04) ── 窗口关闭策略(保留/停止) → 进程树清理 / 退出                 │
│                                                                                   │
│  远端: dsh web (127.0.0.1:3080) ── 完整 Harness(webserver/api 网关/workspace/前端)    │
└──────────────────────────────────────────────────────────────────────────────────┘
```

进程归属：`dsh-desktop.exe` 与它经 `cmd /c` 启动的 `dsh` 子进程是 **父子关系**；`dsh` 自身可再派生 node worker / subagent，清理时按整棵进程树处理（D1）。

---

## 3. 包 / 文件结构

```
dsh-desktop/
├─ go.mod / go.sum
├─ main.go                 # 入口：组装组件、启动消息循环
├─ internal/
│  ├─ singleinstance/      # FR-03 单实例：Mutex + 原生窗口类 + 激活消息 + Win32 封装
│  │   ├─ singleinstance.go  # 互斥体 Acquire / ActivateExisting
│  │   ├─ window.go          # 窗口类注册 + WndProc + BringToFront
│  │   └─ win32.go           # user32/kernel32 的 LazyDLL 封装
│  ├─ service/             # FR-01/FR-04 服务探测、spawn、健康校验、生命周期
│  │   └─ service.go         # Healthy/PortOpen/Spawn/WaitHealthy/Stop
│  ├─ webview/             # FR-02 安全脚本 + 外部链接交接
│  │   └─ shell.go           # SecurityInit / BindExternal
│  └─ ui/                  # loading.html / error.html（go:embed）
└─ build/                  # MSI/安装配置、版本、签名备注（本期未建）
```

---

## 4. 功能实现

### 4.1 单实例（FR-03）

采用 D2 + D3：`Local\` 互斥体 + **自定义原生锚点窗口**（唯一窗口类 + 自定义 WndProc 处理激活消息）。该锚点窗口负责跨进程定位与激活，WebView 内容窗口与它关联。

**互斥体创建（`internal/singleinstance/singleinstance.go`）**
```go
package singleinstance

import (
    "syscall"
    "golang.org/x/sys/windows"
)

const (
    mutexName       = `Local\DSHDesktopApp`   // D2: Local\，避免跨会话
    anchorClassName = "DSHDesktopAppMainWindow" // D3: 唯一窗口类
    // RegisterWindowMessage 的激活消息 id，进程内缓存
    msgActivate windows.Message = 0
)

func Acquire() (handle windows.Handle, exists bool, err error) {
    name, _ := windows.UTF16PtrFromString(mutexName)
    h, err := windows.CreateMutex(nil, false, name)
    if err != nil {
        if err == syscall.ERROR_ALREADY_EXISTS {
            return h, true, nil // 已有实例在运行
        }
        return 0, false, err
    }
    return h, false, nil
}
```

**锚点窗口注册 + WndProc（`internal/singleinstance/window.go`）**
```go
// 注册唯一窗口类，并创建锚点窗口（隐藏、不进任务栏）。
// 该窗口是"单实例定位 + 激活消息接收器"，与 webview 窗口关联协作。
func RegisterAnchor(activate func()) error {
    msgActivate = windows.RegisterWindowMessage(windows.StringToUTF16Ptr(`DSHDesktopApp.Activate`))
    // ... RegisterClassExW 注册 DSHDesktopAppMainWindow，指定 wndProc
    // wndProc 中：
    //   case WM_messageActivate: 调用 activate() 将主窗口恢复并置顶
    //   default: DefWindowProcW
}
```

**激活（`internal/singleinstance/singleinstance.go`）**
```go
// 第二实例：找到已有锚点窗口，发激活消息，随后退出。
func SignalExisting() (bool, error) {
    cls, _ := windows.UTF16PtrFromString(anchorClassName)
    hwnd, _ := windows.FindWindowW(cls, nil)
    if hwnd == 0 {
        return false, nil // 该情况下回退到进程名/窗口兜底，或记为"找不到直接退出"
    }
    windows.SendMessageTimeout(hwnd, msgActivate, 0, 0, 0x0002 /*SMTO_ABORTIFHUNG*/, 3000, nil)
    return true, nil
}
```

> 互操作说明：`webview_go.NewWindow(debug, parentHwnd)` 将 WebView2 控件直接嵌入我们自建的锚点窗口（唯一窗口类 `DSHDesktopAppMainWindow`），因此该窗口同时承担"单实例定位 + 激活消息接收器 + WebView 宿主"。第二实例经 `FindWindowW(类名)` 定位并以注册消息通知其自行 `BringToFront`，无需在锚点窗口与实际 webview 窗口之间切换——见 `internal/singleinstance`。

> 前台激活限制（D3/§FR-03）：后台进程直接 `SetForegroundWindow` 常被系统拒绝。回退链：`ShowWindow(hwnd, SW_RESTORE)` → `SetForegroundWindow` → 失败时 `AttachThreadInput(foreground, hwnd)` 后再次 `SetForegroundWindow` → 仍失败则闪任务栏图标提醒。

### 4.2 服务检测与自动启动（FR-01）

采用 D4 + D5 + D6。

**健康校验（`internal/service/service.go`）**
```go
func (b Behavior) Healthy() bool {
    // dsh web 现在对根路径做浏览器认证：无 token、无 cookie 的裸 GET / 返回
    // 401（"dsh web authentication required;…"），而非 200。
    // 认 200 / 303（token→cookie 交换）为健康；认「401 且响应体带 dsh web 签名」也为健康。
    // 端口可连但不回 dsh-web 信号 → 仍判为不健康（区分 dsh 在跑 / 其它进程占端口 / 僵死）。
    resp, _ := client.Get("http://127.0.0.1:3080/")
    switch resp.StatusCode {
    case 200, 303: return true
    case 401:      return strings.Contains(strings.ToLower(read(resp)), "dsh web")
    }
    return false
}

// 启动主流程：
// 1) Healthy→复用；2) 端口可连但 !Healthy→陈旧/非dsh；3) 任一方"端口冲突"→报错不重启。
// 复用路径直接 Navigate(cfg.URL)（WebView2 持久化浏览器 cookie 已授权）。
// spawn 路径则用 WaitHealthy 返回的认证 URL（含启动 token）Navigate，触发 cookie 交换。
```

**spawn（`internal/service/service.go`）**
```go
func Spawn(ctx context.Context) (stdout io.Reader, err error) {
    // D6: 经 cmd /c 解析 shim；继承 PATH/DSH_HOME；--no-open
    cmd := exec.CommandContext(ctx, "cmd", "/c", "dsh web --no-open")
    cmd.Env = append(os.Environ(), "DSH_WEB_HEADLESS=1")
    pipe, _ := cmd.StdoutPipe()      // 捕获 stdout，用于读取 "dsh web: <url>"
    cmd.Stderr = os.Stderr
    cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: CREATE_NO_WINDOW}
    return pipe, cmd.Start()
}
// 就绪判定：轮询 Healthy() 每 500ms，30s 超时；同时用 bufio 扫 stdout 的 "dsh web:" 行作为提前就绪信号，
// 并从该行解析出含启动 token 的认证 URL（WaitHealthy 返回它，供冷启动 Navigate 触发 cookie 交换）。
```

> 子进程归属：`dsh` 是 `cmd` 的子进程，`dsh` 再派生子进程。为了后续能按进程树清理，建议启动时记录 PID，并用 `taskkill /F /T /PID <pid>` 或 `TerminateProcess` + 作业对象（Job Object）治理整棵树。

### 4.3 WebView2 窗口（FR-02）

采用 `github.com/webview/webview_go`——经 `webview.NewWindow(debug, parentHwnd)` 将 WebView2 嵌入我们自建的原生窗口（见 `internal/singleinstance`）。

```go
// 嵌入到我们自建的 DSHDesktopAppMainWindow 锚点窗口（D3: 我们拥有窗口类）
view := webview.NewWindow(false /*debug 默认关,见 D7*/, unsafe.Pointer(hwnd))
defer view.Destroy()
view.Init(securityInit)      // 注入安全脚本(右键/外链拦截,见 internal/webview)
view.SetHtml(loadingHTML)    // 先加载占位页(go:embed)
// 服务健康后跳转
view.Navigate("http://127.0.0.1:3080")
// 页面加载后无需复核标题（实测页面 <title> 不改写原生标题,见下）
view.Run() // 主循环（阻塞）
```

- **加载态/错误态**：`ui/loading.html` 与 `ui/error.html` 用 `go:embed` 打包；健康校验**成功前**先加载 loading，**失败超时**后加载 error（含"重试/端口冲突/dsh 未安装/WebView2 未安装"递进提示）。
- **标题**：建窗时一次性取 `config.Config.WindowTitleText()`（默认模板 `DeepSeek Harness Desktop {version}` ⇒ `DeepSeek Harness Desktop 0.3.2`），保证 AC-04；`--title` 支持 `{version}` 占位符，不含占位符时原样显示。webview_go 不订阅 WebView2 的 `DocumentTitleChanged`，页面 `<title>`（loading/error/token 页）**不会**改写原生标题，故不需要加载后 `SetTitle` 复核。标题不用于单实例定位（D3）。

### 4.4 服务生命周期（FR-04）

采用 D1。

```go
// 状态机
ServiceState = STOPPED → STARTING → READY → (窗口关闭) → KEEP|STOP

OnClose():
    if cfg.stopServiceOnExit:  // 配置项
        KillTree(dshPid)       // taskkill /F /T /PID | Job Object
    else:
        KEEP                   // 保留服务（默认），下次复用

OnStart():                     // 每次启动
    if Healthy(addr): reuse            // 复用已有
    else if portOpen && !Healthy:      // 端口开放但健康失败 → 陈旧/僵死
        KillTree(dshPidFromProbe)      // 清理整棵树
        Spawn(...)                     // 重新启动
    else if portConflict:              // 非 dsh 占端口
        ShowError("port in use")       // 不重启（§7/AC-06）
    else:
        Spawn(...)
```
- 陈旧检测可加"PID 是否存活 / `dsh --version` 比对 / 进程启动时间戳"辅助。
- **实现补充（停归属，见 PRD FR-04）**：`KillTree` 的目标不是"记录里的监听 PID"而是**本壳 spawn 的
  `cmd.exe` 包装器进程树**（`spawnerPid`）：真正服务端口的进程可能换 PID，而包装器随实例终生存在，
  `taskkill /F /T` 连带终止当前服务。终止前必须先证明归属（包装器是本壳直接子进程，或包装器/监听者仍以
  记录中的启动时间在运行，且进程**未退出**——`procinfo.Exited`）；证明不成立就放弃，且**不得把"没停"记成
  "已停止"`。`main.go` 的关窗路径据此区分"已停止（列出被终止 PID）"/"无需停止"/"证明不足，已放弃"。
- 主动停止：托盘/菜单提供"退出（保留服务）"与"退出并停止服务"两个动作（见 PRD §10 托盘）。

### 4.5 加载态/错误态（FR-02）
已并入 §4.3。关键点：窗口在服务健康前**立刻**显示 loading（满足 NFR"窗口 ≤3s 出现加载态"），服务就绪后再跳转，避免 `ERR_CONNECTION_REFUSED` 白屏。

### 4.6 安全（FR-02 / NFR 安全行）

- **导航白名单**：仅放行本地规范源 `http://127.0.0.1:3080`。高层绑定不暴露导航事件（`NavigationStarting`/`NewWindowRequested`），因此实际在 DOM 层通过注入脚本拦截外部 `http(s)`/`mailto:` 点击并交给系统默认浏览器（`cmd /c start <url>`），见 `internal/webview/shell.go`。重定向/表单提交/新窗口不在拦截范围（已知局限，见 §7）。
- **DevTools / 右键菜单（需互操作）**：`github.com/webview/webview_go` 的 `NewWindow(debug, hwnd)` 仅暴露 debug 开关，未直接暴露 `AreDefaultContextMenusEnabled`/`AreDevToolsEnabled`。实际实现采用折中路径（见 `internal/webview/shell.go`）：
  1. `Debug` 默认 `false`（不开 DevTools）；或
  2. 最低限度用 JS 注入 `document.addEventListener('contextmenu', e => e.preventDefault())`（仅覆盖 DOM 层）。
  > 说明：本期采纳了上述折中方案（PRD D7）；如需完全关闭 WebView2 默认右键菜单/DevTools，需互操作 ICoreWebView2Settings（经底层 HWND/Controller 取接口），或改用 `github.com/jchv/go-webview2`（见 §7 开放问题）。

---

## 5. 构建与打包

- **CGO**：`github.com/webview/webview_go` 依赖 CGO，需在本机/Windows 工具链上 `go build`；无法从 Linux 交叉编译到 Windows。
- **依赖产物**：WebView2Loader.dll（WebView2 相关）；或依赖系统已装的 WebView2 Evergreen Runtime。
- **分发**：`MSI`（WiX/msi 工具）或便携版；`go:embed` 打进 loading/error 资源。
- **代码签名**：签名以避免 SmartScreen 警告；自动更新列入后续（PRD §10）。
- **版本信息**：`--version` 与资源版本号用于 AC/陈旧检测。

---

## 6. 异常矩阵（实现对照 PRD §7）

| 场景 | 实现策略 | 验证点 |
| :-- | :-- | :-- |
| `dsh` 未安装/不可解析 | `cmd /c dsh ...` 返回非 0 / `exec: executable file not found` → 友好提示 + error.html | AC-05 |
| 端口被非 dsh 占用 | 端口可连但 `Healthy()` 为假 → 报"端口冲突"，**不重启** | AC-06 |
| `dsh` 服务僵死 | 端口开放但健康失败 → `KillTree` + 重新 `Spawn` | AC-03 |
| WebView2 缺失 | `webview.NewWindow`/Runtime 初始化的系统错误 → 提示下载 Runtime | — |
| 启动超时 | 30s 轮询失败 → error.html + 退出；不残留半启动子进程 | — |
| 锚点窗口找不到 | 互斥体存在但无锚点窗口（僵尸/跨会话）→ 记录日志并退出 | — |
| 外部链接/下载 | 外部导航 → 系统浏览器；下载按需处理 | AC-07 |

---

## 7. 风险与开放问题

1. **WebView2 窗口与自定义锚点窗口的关系**（已在 0.1.0 落地）：`webview_go.NewWindow(debug, parentHwnd)` 将 WebView2 直接嵌入我们自己注册的原生锚点窗口（唯一窗口类 `DSHDesktopAppMainWindow`），因此锚点窗口同时作为单实例接收器与 WebView 宿主，无需再"切换到实际 webview 窗口"。已解决。
2. **DevTools/右键菜单的关闭**：受 `webview_go` 高层绑定能力限制，本期折中为 `Debug:false` + JS 注入（§4.6）；如需完全关闭默认右键菜单，仍须互操作 ICoreWebView2Settings 或换绑定。
3. **`dsh web` 版本差异**：持久化服务可能因升级/改配置而陈旧，需约定健康/版本校验口径（PRD FR-04）。
4. **多会话/多用户**：D2 用 `Local\` 已缓解；若业务要求"机器级唯一"，需评估 `Global\` 的副作用。
5. **子进程树清理的可靠性**：`dsh` 可能派生子节点；采用 Job Object 或 `taskkill /T`，需在验收中覆盖"强制退出"路径，避免僵尸进程。
