
# dsh-desktop 技术设计文档

| 文档版本 | 修改日期 | 修改人 | 修改内容 |
| :--- | :--- | :--- | :--- |
| V1.0 | 2026-09-02 | AI Assistant | 依据 PRD V1.1 与审阅意见，固化推荐设计决策，给出技术实现骨架与伪代码。 |

> 配套文档：[`dsh-destop-prd.md`](./dsh-destop-prd.md)（需求基线）。
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
| D3 | 单实例定位 | **唯一原生窗口类** + `RegisterWindowMessage` 消息驱动激活（不靠窗口标题） | 标题会随页面 `<title>` 变化，类名/消息定位更稳 |
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
│  ├─ singleinstance/      # FR-03 单实例：Mutex + 锚点窗口 + 激活消息
│  │   ├─ mutex.go
│  │   ├─ anchor.go        # 自定义窗口类注册 + WndProc
│  │   └─ activate.go      # FindWindowW + SendMessageTimeout
│  ├─ service/             # FR-01/FR-04 服务探测、spawn、健康校验、生命周期
│  │   ├─ probe.go         # 端口 + HTTP 健康校验
│  │   ├─ spawn.go         # cmd /c 启动、env 继承、stdout 捕获
│  │   └─ lifecycle.go     # 持久化复用、陈旧检测、进程树清理
│  ├─ webview/             # FR-02 WebView2 窗口 + 导航/安全控制
│  │   ├─ window.go
│  │   ├─ navpolicy.go     # 导航白名单 / 外部链接系统浏览器
│  │   └─ security.go      # DevTools/右键/注入 (需互操作)
│  └─ ui/                  # loading.html / error.html（go:embed）
└─ build/                  # MSI/安装配置、版本、签名备注
```

---

## 4. 功能实现

### 4.1 单实例（FR-03）

采用 D2 + D3：`Local\` 互斥体 + **自定义原生锚点窗口**（唯一窗口类 + 自定义 WndProc 处理激活消息）。该锚点窗口负责跨进程定位与激活，WebView 内容窗口与它关联。

**互斥体创建（`internal/singleinstance/mutex.go`）**
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

**锚点窗口注册 + WndProc（`internal/singleinstance/anchor.go`）**
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

**激活（`internal/singleinstance/activate.go`）**
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

> 互操作说明：锚点窗口自定义类 + WndProc 是我们自己注册的原生窗口；`github.com/webview/webview` 默认创建自己的窗口。二者协调：锚点窗口收到 `DSHDesktopApp.Activate` 后，通过 `FindWindowW`/`SetForegroundWindow(with AttachThreadInput)` 切换到实际 webview 窗口。若希望将 webview 直接创建为锚点子窗口，需改用暴露 HWND/窗口控制的 WebView2 绑定（如 `github.com/jchv/go-webview2`）——见 §7 开放问题。

> 前台激活限制（D3/§FR-03）：后台进程直接 `SetForegroundWindow` 常被系统拒绝。回退链：`ShowWindow(hwnd, SW_RESTORE)` → `SetForegroundWindow` → 失败时 `AttachThreadInput(foreground, hwnd)` 后再次 `SetForegroundWindow` → 仍失败则闪任务栏图标提醒。

### 4.2 服务检测与自动启动（FR-01）

采用 D4 + D5 + D6。

**健康校验（`internal/service/probe.go`）**
```go
func Healthy(addr string) bool {
    // addr = "http://127.0.0.1:3080"
    client := &http.Client{Timeout: 2 * time.Second}
    resp, err := client.Get(addr + "/")
    if err != nil { return false }
    defer resp.Body.Close()
    return resp.StatusCode == http.StatusOK && isDSH(resp) // isDSH: 校验响应体特征/头，确认是 dsh 前端
}

// 启动主流程：
// 1) Healthy→复用；2) 端口可连但 !Healthy→陈旧/非dsh；3) 任一方"端口冲突"→报错不重启。
```

**spawn（`internal/service/spawn.go`）**
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
// 就绪判定：轮询 Healthy() 每 500ms，30s 超时；同时用 bufio 扫 stdout 的 "dsh web:" 行作为提前就绪信号。
```

> 子进程归属：`dsh` 是 `cmd` 的子进程，`dsh` 再派生子进程。为了后续能按进程树清理，建议启动时记录 PID，并用 `taskkill /F /T /PID <pid>` 或 `TerminateProcess` + 作业对象（Job Object）治理整棵树。

### 4.3 WebView2 窗口（FR-02）

采用 `github.com/webview/webview`。

```go
w := webview.New(webview.Settings{
    Title:    "DeepSeek Harness",
    Width:    1200,
    Height:   800,
    Resizable:true,
    Debug:    false,
    URL:      loadLocal("loading.html"), // 先加载占位页
})
defer w.Destroy()
// 服务健康后跳转
w.Navigate("http://127.0.0.1:3080")
// 页面加载后复核标题（WebView2 可能随 <title> 改动）
w.Dispatch(func() { w.SetTitle("DeepSeek Harness") })
w.Run() // 主循环（阻塞）
```

- **加载态/错误态**：`ui/loading.html` 与 `ui/error.html` 用 `go:embed` 打包；健康校验**成功前**先加载 loading，**失败超时**后加载 error（含"重试/端口冲突/dsh 未安装/WebView2 未安装"递进提示）。
- **标题**：`SetTitle` 复核，保证 AC-04；不用于单实例定位（D3）。

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
- 主动停止：托盘/菜单提供"退出（保留服务）"与"退出并停止服务"两个动作（见 PRD §10 托盘）。

### 4.5 加载态/错误态（FR-02）
已并入 §4.3。关键点：窗口在服务健康前**立刻**显示 loading（满足 NFR"窗口 ≤3s 出现加载态"），服务就绪后再跳转，避免 `ERR_CONNECTION_REFUSED` 白屏。

### 4.6 安全（FR-02 / NFR 安全行）

- **导航白名单**：`NavPolicy` 只放行 `http://127.0.0.1:3080`；对 `NewWindowRequested`/`NavigationStarting` 拦截外部 `http(s)`/`mailto:` → 交给系统默认浏览器（`cmd /c start <url>` 或 `xdg` 对应）。
- **DevTools / 右键菜单（需互操作）**：`github.com/webview/webview` 的 `Settings` 未直接暴露 `AreDefaultContextMenusEnabled`/`AreDevToolsEnabled`。推荐的可行路径：
  1. 通过 WebView2 `ICoreWebView2Settings` 接口调用 `put_AreDevToolsEnabled(FALSE)`、`put_AreDefaultContextMenusEnabled(FALSE)`（经 `webview` 底层 HWND/Controller 取接口，需额外 CGO 互操作代码）；或
  2. 改用暴露更多 WebView2 控制的绑定（`github.com/jchv/go-webview2`）；或
  3. 最低限度用 JS 注入 `document.addEventListener('contextmenu', e => e.preventDefault())` + 不开启 `Debug`（但也只覆盖 DOM 层）。
  > 本期建议：`Debug:false`（默认不开 DevTools），右键菜单采用(1)或(2)断开默认菜单；若互操作成本过高，退化为(3) + 说明局限，见 §7 开放问题。

---

## 5. 构建与打包

- **CGO**：`github.com/webview/webview` 依赖 CGO，需在本机/Windows 工具链上 `go build`；无法从 Linux 交叉编译到 Windows。
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
| WebView2 缺失 | `webview.New`/Runtime 初始化的系统错误 → 提示下载 Runtime | — |
| 启动超时 | 30s 轮询失败 → error.html + 退出；不残留半启动子进程 | — |
| 锚点窗口找不到 | 互斥体存在但无锚点窗口（僵尸/跨会话）→ 记录日志并退出 | — |
| 外部链接/下载 | 外部导航 → 系统浏览器；下载按需处理 | AC-07 |

---

## 7. 风险与开放问题

1. **WebView2 窗口与自定义锚点窗口的关系**：推荐方案 D3 里，"锚点窗口"作为单实例消息接收器；但如何精确关联并置顶实际的 webview 内容窗口（尤其是当 webview 库默认自建窗口时）需要落地验证。若 `github.com/webview/webview` 无法方便取得 HWND/自定义类，建议切换到能暴露窗口控制的绑定，或把 webview 作为锚点子窗口（需额外互操作）。
2. **DevTools/右键菜单的关闭**：受库能力限制，需互操作或换绑定（§4.6），是本期主要的"超出库默认"工作量。
3. **`dsh web` 版本差异**：持久化服务可能因升级/改配置而陈旧，需约定健康/版本校验口径（PRD FR-04）。
4. **多会话/多用户**：D2 用 `Local\` 已缓解；若业务要求"机器级唯一"，需评估 `Global\` 的副作用。
5. **子进程树清理的可靠性**：`dsh` 可能派生子节点；采用 Job Object 或 `taskkill /T`，需在验收中覆盖"强制退出"路径，避免僵尸进程。
