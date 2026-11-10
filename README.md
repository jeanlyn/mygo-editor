# MyGo Editor · 工作区版

Go + [egoist/mygo](https://github.com/egoist/mygo) 原生桌面代码编辑器。深色、紧凑、编辑优先的布局参考 [Zed 的经典面板设计](https://zed.dev/blog/new-panel-system)，未使用 Zed 品牌资源。没有 WebView、Electron、Monaco、JavaScript 前端或 AI 服务。

## 运行

需要 **Go 1.27.3+**：

```sh
go mod download
CGO_ENABLED=0 go run .
# 文件夹和多个文件可一并传入：
go run . ./samples ./samples/hello.go ./samples/config.json
```

Windows PowerShell：`$env:CGO_ENABLED="0"; go run .`。提供的二进制可解压后直接运行；Linux 需 GTK 3、字体及图形运行库，macOS/Windows 构建未经签名/公证。三平台构建通过不等于三平台完整运行验收。

源码编译：

```sh
CGO_ENABLED=0 go build -buildvcs=false -trimpath -tags mygo_noinspector -ldflags "-s -w" -o mygo-editor .
```

Windows 输出名使用 `.exe`，ldflags 加 `-H windowsgui`。无需 WebKit/WebView。

## 本版真正实现的功能

- **项目树**：打开文件夹、按需异步读取、展开/收起、刷新、显示隐藏文件、目录优先排序、点击文件在当前窗口打开。相同层级固定左对齐，子级每层 16px 缩进。
- **多标签**：同窗口打开多个文件、点击/快捷键切换、关闭标签、未保存圆点；每个标签有独立内容、撤销记录、选区、滚动、换行和语言选择。重复打开相同路径/符号链接/硬链接不创建重复标签。
- **保存保护**：新建、打开、保存、另存为；关闭标签、窗口、退出时逐文件询问保存/不保存/取消。保存目标在另一标签打开时拒绝覆盖。保存或提示期间暂时只读，避免多份异步写盘与关闭重叠；关闭前再次检查批准的内容未变化。
- **外部修改检测**：保存前比对文件内容指纹。文件被修改/删除或另存为目标已存在时询问是否覆盖，不静默覆盖。不是实时文件监视器。
- **代码编辑**：多语言高亮、查找/替换（包含正则等框架内置选项）、跳转行、撤销重做、剪切复制粘贴、缩进、注释、行号、括号补全、折叠、多光标、自动换行。
- **文件格式**：保留 UTF-8 BOM 与 LF/CRLF。拒绝非 UTF-8、二进制、混合/旧式 CR 换行、超过 16 MiB 的文件或保存内容，以及 FIFO/设备等特殊文件。

窗口、系统菜单、文件对话框使用 MyGo 平台接口。代码区、项目树、标签栏与状态栏是 MyGo 自绘原生 UI，不是网页，也不是系统自带文本框。系统菜单/窗口装饰仍受平台主题影响。

## 快捷键

| 功能 | 快捷键 |
| --- | --- |
| 新建 / 打开文件 | Cmd/Ctrl+N / O |
| 打开文件夹 | Cmd/Ctrl+Shift+O |
| 保存 / 另存为 | Cmd/Ctrl+S / Shift+S |
| 关闭标签 | Cmd/Ctrl+W |
| 切换标签 | Ctrl+Tab / Ctrl+Shift+Tab |
| 显示/隐藏项目树 | Cmd/Ctrl+B |
| 查找 / 替换 | Cmd/Ctrl+F / Alt+F |
| 跳转到行 | Ctrl+G |
| 行注释 | Cmd/Ctrl+/ |
| 缩进 / 减少缩进 | Cmd/Ctrl+] / [ |

## 有意保留的边界

- 一个窗口一个工作区，不是完整 IDE。没有终端、Git、LSP、AI、分屏、文件重命名/删除或递归全项目搜索。
- 项目树每个目录最多列出 1000 项，并显示截断提示；读取的是有限子集，不保证包含目录全部项。符号链接不展开，指向普通文件的链接可打开，其他类型被拒绝。刷新会重置展开状态。
- **没有自动保存、会话恢复或崩溃恢复**。为实现同窗口多标签，本版不再使用框架一文档一窗口的 Documents 管理器。请及时保存。
- 保存采用同目录临时文件、同步写入、再次检查冲突后重命名替换；保留普通权限位，但不保留 ACL/xattr，且替换 inode 会让其他硬链接保留旧内容。不要用于依赖扩展文件元数据的重要原件。恶意并发文件替换不在安全保证范围内。
- 不自动重载外部修改；保存时检测冲突。大文件/慢网络文件系统读写期间暂停编辑；无法保证网络文件系统上所有原子/持久化语义。
- 状态栏列数按 Unicode 码点计算，不是 tab 展开后的视觉列。中文输入法和平台剪贴板仍需目标系统完整测试。

## 验证与来源

```sh
go test ./...
go vet ./...
```

验证细节见 VALIDATION.md。固定 MyGo 官方 commit `4abb83d87531888e94bd0c58bf8c8baa711bd1cb`，依赖通过 go.mod/go.sum 锁定。源码包包含 samples/ 示例、模型测试和真实布局测试，不含工具链或依赖缓存。

## GitHub 自动构建与下载

`.github/workflows/build.yml` 在推送 main、v 开头的标签或手动运行时构建 Windows x64、macOS Apple Silicon 和 macOS Intel 三种下载包。每个平台先运行测试与 vet，失败则不上传该平台制品。Go 版本由 go.mod 固定。工作流需要 GitHub 托管 runner 能下载 Go 与依赖；私有仓库的 Actions 额度以账户设置为准。

仓库 → Actions → Build desktop downloads → 成功的运行 → Artifacts，选择对应平台。GitHub 外层 ZIP 解压后，Windows 内层 ZIP 含 exe；macOS 内层 tar.gz 保留可执行权限。附带 SHA256SUMS.txt。制品默认保存 30 天，需要登录且拥有仓库访问权限；过期后可手动重新运行工作流。当前未配置永久 Release 发布。

这些构建未签名或公证，macOS Gatekeeper / Windows SmartScreen 可能提示。不要全局关闭系统安全保护；仅在确认来源后按系统对单个应用的提示处理。自动测试与编译不能替代各平台的图形界面验收。

## 固定发版流程

需要 Git、GitHub CLI（gh 已登录并有仓库发布权限）和 Python 3。源码与 CI 配置位于 main；首次版本 v0.1.0 为预览版。

1. 修改代码与 `release-notes/vX.Y.Z.md`，运行测试，提交并推送 main。
2. 等待 `Build desktop downloads` 三个平台全部成功，记录该提交的 Actions run ID。
3. 检查：`python3 scripts/release.py vX.Y.Z --run-id RUN_ID --notes release-notes/vX.Y.Z.md --check-only`
4. 发布：去掉 `--check-only` 后执行同一命令。默认创建预览版；明确要稳定版时加 `--stable`。

脚本要求干净的 main、远端相同提交、正确的成功工作流和三平台 job，下载并校验所有制品，再创建带准确 commit 的新版本标签和草稿 Release，确认资产完整后发布。既有版本不会被覆盖。中途失败可能留下草稿，先检查草稿，不要盲目重复发布或移动标签。当前脚本固定针对 jeanlyn/mygo-editor；不需要将令牌写入仓库。

Release 直接提供三个内层平台包和统一 SHA256SUMS.txt，不再需要解压 Actions 的外层 ZIP。项目自身许可证尚未确定；第三方组件通知见 THIRD_PARTY_NOTICES.md。
