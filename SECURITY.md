# 安全策略(Security)

## 报告漏洞

如果你发现本项目存在安全漏洞,请**不要**在公开 Issue 中披露。请通过以下任一方式
私下报告:

- 在本仓库开一个 **private advisory**(GitHub 仓库 → Security → Report a vulnerability)。
- 或直接联系仓库维护者,并在描述中包含复现步骤、影响范围与可能的修复建议。

我们会在收到报告后尽快确认并给出回应,协商发布窗口后(通常 90 天内)再公开。

## 关注点

- **下载与更新**:`scripts/fetch-exe.mjs` 与 `internal/update` 都按发布时间下载
  `dsh-desktop.exe` 并校验随发布的 `sha256` 侧车文件;校验不通过即删除并报错,
  绝不留存未经校验的二进制。若你发现可绕该校验的问题,请按上面方式报告。
- **品牌资产**:项目内嵌 DeepSeek 图标,属于 DeepSeek 方;品牌相关使用需另行许可。
- **WebView2**:通过官方 WebView2 Evergreen 运行时渲染;请保持运行时更新到最新。

感谢你的配合。
