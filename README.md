# agentmover · Agent 数据搬家 / C 盘瘦身

一个 Go 写的单文件小工具，专治"C 盘被各种 AI/Agent 客户端塞满"。
它不改程序、不改注册表（除了环境变量那一项），靠 **robocopy 搬运 + 目录联接（mklink /J）** 让程序以为数据还在原地。

## 为什么会有这个问题

- Electron 打包的客户端默认把数据写 `%APPDATA%`、`%LOCALAPPDATA%`，也就是 C 盘；
- 模型权重、embedding 缓存动辄几十 GB；
- 跨平台开发者习惯 `os.homedir()` + 隐藏目录（`.claude`、`.codex`、`.cache`），Windows 上照样落在 C；
- 写用户目录不需要管理员权限，所以"默认最安全"。

## 目标位置不是写死的

三个层次，随便改，改完全部记进 `config.json`：

| 层级 | 怎么改 | 例子 |
|---|---|---|
| 统一根目录 | 首次启动会问；或菜单 7 / `agentmover dest <路径>` | `D:\AgentData`、`E:\AI\data` |
| 目录结构 | 菜单 8 / `agentmover layout flat` | 分支：根/类别/应用；平铺：根/应用 |
| 单个应用 | 迁移时当场输入新路径；或 `agentmover target <id> <路径>` | `agentmover target claude-cli D:\AI\claude` |

`agentmover target --reset` 可清除单项自定义，回到默认布局。

## 目录布局

所有数据统一落到一个根目录，根目录下按类别开分支，分支下每个应用一个子目录：

```
D:\AgentData\                 ← 根目录（agentmover dest 可改）
├── agents\                    ← 桌面 Agent 类
│   ├── claude-desktop\
│   └── dsh\
├── cli\                       ← 命令行 Agent
│   ├── codex\
│   └── claude-cli\
├── ide\                       ← AI 编辑器
│   └── cursor\
├── models\                    ← 模型缓存
│   └── lmstudio\
├── packages\                  ← 包管理器缓存
└── other\                     ← 自动发现但归不了类的
```

C 盘原来的位置会变成指向这些目录的 **目录联接**，程序完全无感。

## 功能

| 命令 | 作用 |
|---|---|
| `agentmover` | 交互菜单（推荐） |
| `agentmover scan` | 只体检：列出所有占地的 Agent 目录、体积、建议 |
| `agentmover plan` | 预览每项会落到哪个分支（不动文件） |
| `agentmover dest D:\AI\data` | 换统一根目录，写进 config.json |
| `agentmover move 3` | 把第 3 项搬到 `<根目录>\<分支>\<id>` 并建联接 |
| `agentmover revert` | 按 manifest 还原回 C 盘 |
| `agentmover env --apply` | 写 `HF_HOME` / `OLLAMA_MODELS` / `XDG_CACHE_HOME` 到用户环境变量 |
| `agentmover clean` | 清 `Cache` / `GPUCache` / `logs` 等可再生缓存 |
| `agentmover report` | 生成 report.html |

默认 **dry-run**：不加 `--apply`（交互里不确认）就不会动文件。

## 下载 / 编译

仓库里直接带了编译好的 `agentmover.exe`（windows/amd64，2.9 MB，纯静态无依赖），下载双击即可。

自己编译（需要 Go 1.21+）：

```powershell
git clone https://github.com/pengxiaozi/go-agent-mv
cd go-agent-mv
.\build.ps1            # 出 windows/amd64
.\build.ps1 -All       # 顺带出 linux-amd64 / darwin-arm64
```

## 出错了怎么看

- 每个子进程调用的原始输出都会解码后写进 `agentmover.log`（旁边另一个文件）。中文系统下 cmd/robocopy 的 GBK 报错已转成 UTF-8，不会乱码。
- 搬之前会做"改名探测"判断目录是否被占用，被占用直接拒绝，不会搬一半。
- 源目录若还有清不掉的残留（比如内部嵌套的目录联接、被占用的文件），会先改名挪到 `<目录>.leftover-时间戳`，让联接能建起来，成功后自动清理。
- 任何一步失败都会把文件搬回原处，`agentmover.log` 里有完整命令与退出码。

## 安全性

- 搬运前检查目标盘剩余空间、目标目录是否已存在；
- 搬运后比对体积（< 95% 视为失败，自动搬回）；
- 建联接失败自动回滚原地；
- 每一步写 `manifest.json`，`revert` 一键撤回；
- 完全跳过重解析点（`/XJ`），不会顺着联接把别处数据搬乱。

## 注意

- 目录联接 **不需要管理员权限**（符号链接才需要）；
- 已经在运行的程序先关掉，否则 robocopy 会因文件占用报错（工具会检测失败并回滚）；
- 部分程序会校验"数据目录是不是普通目录"，极少数情况下需要改用环境变量方案；
- `%LOCALAPPDATA%\Packages` 下的 UWP 应用不要搬。
