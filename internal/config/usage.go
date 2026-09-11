package config

import (
	"flag"
	"fmt"
)

// zhFlagUsage maps a flag name to its Chinese help description. Any flag not
// listed here falls back to the English description registered at parse time.
var zhFlagUsage = map[string]string{
	"host":            "健康探测与 dsh 启动使用的绑定地址",
	"port":            "dsh web 服务的监听端口",
	"command":         "要启动的 dsh 子命令 (web)",
	"stop-on-exit":    "关闭窗口时停止本壳自己启动的 dsh 服务 (他人/附着的实例不会被停止)",
	"devtools":        "启用 WebView2 开发者工具",
	"context-menu":    "保留 WebView2 默认右键菜单",
	"startup-timeout": "等待服务就绪的超时秒数 (须 > 0)",
	"poll-ms":         "健康检查轮询间隔 (毫秒, 须 > 0)",
	"width":           "窗口宽度 (须 > 0)",
	"height":          "窗口高度 (须 > 0)",
	"title":           "窗口标题",
	"version":         "打印版本并退出",
	"check-update":    "检查 GitHub Releases 是否有新版本并退出",
	"update":          "下载并应用最新版本, 然后退出",
	"url":             "要附着的 dsh web URL (可带 ?token=…; 未显式指定 --host/--port 时采纳其 host/port, 冲突则报错)",
}

// writeUsage prints the flag help for the given FlagSet in the current UI
// language. When chinese is true the Chinese headers/descriptions and the
// localised "默认" default label are used; otherwise the English descriptions
// registered on the flags are printed with the "default" label.
func writeUsage(fs *flag.FlagSet, chinese bool) {
	out := fs.Output()

	if chinese {
		fmt.Fprintln(out, "用法: dsh-desktop [选项]")
		fmt.Fprintln(out)
		fmt.Fprintln(out, "选项:")
	} else {
		fmt.Fprintln(out, "Usage: dsh-desktop [options]")
		fmt.Fprintln(out)
		fmt.Fprintln(out, "Options:")
	}

	label := "default"
	if chinese {
		label = "默认"
	}

	fs.VisitAll(func(f *flag.Flag) {
		name := "--" + f.Name
		desc := f.Usage
		if chinese {
			if d, ok := zhFlagUsage[f.Name]; ok {
				desc = d
			}
		}
		def := ""
		if f.DefValue != "" && f.DefValue != "false" {
			def = fmt.Sprintf("  (%s %s)", label, f.DefValue)
		}
		fmt.Fprintf(out, "  %-24s  %s%s\n", name, desc, def)
	})
}
