# ly — Lingying Gateway CLI

[![Go](https://img.shields.io/badge/Go-1.26%2B-blue.svg)](https://go.dev/)
[![License](https://img.shields.io/badge/License-MIT-green.svg)](LICENSE)

`ly` 是 Lingying Gateway 的命令行客户端。它在终端中发现并调用当前账号有权限使用的文本、图片、视频与音频模型。

模型目录、参数校验、任务执行和计费均由 Gateway 负责；CLI 负责鉴权、文件传输、提交任务、轮询和将结果保存到本地。

```bash
ly text "用三句话解释量子计算"
ly image -p "赛博朋克风格的柴犬，电影灯光"
ly video --model seedance2.0 -p "cinematic flythrough"
ly audio "一段播客开场白"
```

## 安装

支持 macOS（Intel/Apple Silicon）、Linux（x64/arm64）和 Windows（x64/arm64）。安装后运行 `ly --help` 验证命令可用。

### curl：macOS、Linux 和 WSL

安装脚本下载对应平台的发布包，校验 SHA-256，并将二进制放到 `~/.local/bin`。

```bash
curl -fsSL https://github.com/Din-Studio/lingying-cli/releases/latest/download/install.sh | bash
ly --help
```

如脚本提示 PATH 尚未生效，请重新打开终端或按提示执行 `source`。

### PowerShell：Windows 原生终端

无需 Node.js。安装器识别 x64/arm64，下载当前 Windows ZIP，强制校验 SHA-256 后安装到 `%LOCALAPPDATA%\ly\bin`，并写入当前用户的 PATH。

```powershell
irm https://github.com/Din-Studio/lingying-cli/releases/latest/download/install.ps1 | iex
ly --help
```

重新打开 PowerShell 或 Windows Terminal 以获取更新后的 PATH。校验和、下载或解压失败时安装器会直接失败，不会安装未经校验的二进制。

### 国内网络

安装与更新默认直连 GitHub，失败时自动回退到公共加速镜像。也可以用 `LY_MIRROR` 指定自己的镜像前缀，它会排在公共镜像之前：

```bash
LY_MIRROR="https://your-mirror.example.com" \
  curl -fsSL https://github.com/Din-Studio/lingying-cli/releases/latest/download/install.sh | bash
```

校验和文件始终优先从 GitHub 直连获取——只要它来自可信来源，即便安装包走镜像也无法被篡改。若直连获取校验和也失败，安装器会退而从镜像获取，并明确打印警告。

### 安装期环境变量

| 变量 | 作用 |
|---|---|
| `LY_VERSION` | 锁定版本，如 `0.1.6` |
| `LY_MIRROR` | 自定义镜像前缀，排在公共镜像之前 |
| `LY_INSTALL_DIR` | 安装目录，默认 `~/.local/bin`（Windows 为 `%LOCALAPPDATA%\ly\bin`） |

### Go

需要 Go 1.26.2 或更高版本。

```bash
go install github.com/Din-Studio/lingying-cli@latest
ly --help
```

### 从源码构建

```bash
git clone https://github.com/Din-Studio/lingying-cli.git
cd lingying-cli
make build
./ly --help
```

在 Unix-like 系统安装到指定前缀：`make install PREFIX=$HOME/.local`。

## 更新

```bash
ly update            # 更新到最新版本
ly update --check    # 只检查是否有新版本，不做任何改动
```

`ly update` 从 GitHub Releases 下载当前平台的发布包，校验 SHA-256、确认新版本可以正常运行后，才替换正在运行的可执行文件。无论 ly 是通过安装脚本还是 `go install` 安装的，更新方式都相同。校验和不匹配或新版本跑不起来时直接中止，当前可用的 ly 保持不变。

更新与安装共用同一套来源策略：直连优先，失败时回退到 `LY_MIRROR` 与公共镜像。若校验和只能从镜像取得，命令会打印警告说明它无法防篡改。

若 ly 安装在当前用户无写权限的目录（如 `/usr/local/bin`），命令会失败——用有写权限的方式重新执行即可。

## 快速开始

```bash
# 1. 保存 OAuth Access Token（支持本地文件上传）
ly auth login

# 或保存 API Key（同样支持本地文件上传）
ly auth set-key <your-key>

# 2. 验证鉴权和 Gateway 连通性
ly check

# 3. 调用文本模型
ly text "用三句话解释什么是 Docker"

# 4. 提交图片任务并等待下载
ly image -p "一只戴宇航头盔的柴犬，3D 渲染"

# 5. 发现当前账号可用的模型
ly model list
```

非交互环境不要把凭据写进命令历史。改用 `LY_ACCESS_TOKEN` 或 `LY_API_KEY`：

```bash
export LY_ACCESS_TOKEN='<access-token>'
ly --json model list
```

## 常用命令

### 文本与模型发现

| 命令 | 说明 |
| --- | --- |
| `ly text "..."` | 使用默认文本模型对话 |
| `ly text --model <id> "..."` | 指定模型 ID 或 display name |
| `ly text --max-tokens 4096 "..."` | 设置最大输出 token 数 |
| `ly text --param temperature=0.7 "..."` | 传递当前文本模型 schema 的动态字段 |
| `ly model list` | 列出当前账号可用模型 |
| `ly model list --type image` | 按模型类型筛选 |
| `ly model info <id>` | 查看模型详情与 Gateway 返回的 `input_schema` |
| `ly model search <keyword>` | 搜索模型 ID、名称或类型 |
| `ly check` | 检查鉴权、Gateway 连通性和模型数量 |

CLI 每次调用均从 Gateway 动态发现模型，不缓存或内置静态能力目录。未传 `--model` 时，CLI 从本次发现结果中为命令类型稳定选择一个可用模型；显式 `--model` 仍必须精确匹配，不会悔悔换模型。调用前可先用 `model info` 查看模型当前的输入 schema；字段是否合法由 Gateway 返回最终结果。

对 Agent，`ly --json model info <精确-id>` 的 `data.input_schema` 是 JSON 对象（不是需要再次解析的字符串），且精确 ID 优先于模糊搜索结果；因此应使用同一个精确 ID 传给后续命令的 `--model`。

文本请求同样使用动态 schema 参数：`ly text --param temperature=0.7 "写一个更有创意的标题"`。CLI 自行生成单条用户 `messages`；`max_tokens` 可用 `--param max_tokens=2048` 或既有快捷项 `--max-tokens 2048`（后者优先）。`--dry-run --json` 的 `data.request` 会展示实际将发送的顶层请求体。

### 图片、视频和音频

| 命令 | 说明 |
| --- | --- |
| `ly image -p "..."` | 生成或编辑图片 |
| `ly image -i ./photo.jpg -p "..."` | 使用本地图片作为输入 |
| `ly video -p "..."` | 生成或编辑视频 |
| `ly video -i ./clip.mp4 -p "..."` | 使用本地图片/视频作为输入 |
| `ly audio -i ./source.mp3 -p "..."` | 使用本地音频作为输入 |
| `ly <image\|video\|audio> --model <id> -p "..."` | 指定媒体模型 |
| `ly <image\|video\|audio> --param key=value` | 传递额外模型字段，可重复使用 |
| `ly <image\|video\|audio> --output ./result.png` | 指定第一个结果文件的保存位置 |

`--param` 按本次模型 `input_schema` 编码：字符串始终保留原文（例如 `9:16`、`1K`），整数和数值使用严格完整解析，布尔字段接受 `true/false`、`yes/no`、`1/0`。嵌套对象可用点路径传递，数组和整个对象使用 JSON：

```bash
# image-2 的 metadata 是 object；两种写法产生等价的 object 请求体
ly image --model image-2 -p "赛博朋克猫" --param metadata.quality=high
ly image --model image-2 -p "赛博朋克猫" --param 'metadata={"quality":"high","output_format":"webp"}'

# 需要自行传远程数组字段时使用 JSON 数组；也可与 -i 本地文件混用
ly video --model seedance2.0 -p "镜头推进" --param 'images=["https://example.com/reference.jpg"]'
```

未知的顶层字段仍保留为字符串，由 Gateway 校验；未声明的嵌套路径会在 CLI 端报错，因为它无法表示为合法嵌套请求。CLI 会补齐缺失的“必填且带 default”字段，用户传入的值优先；不做完整本地 schema 校验。位置提示词中的 `=` 会保持为提示词文本，额外字段请始终显式使用 `--param`。

输入既可以是本地路径，也可以是 `http://` 或 `https://` URL。OAuth 与 API Key 均可用于 URL 输入或本地输入；本地输入时 CLI 会自动上传：小文件走单次直传，大文件（≥50 MiB）自动切换为分片上传。对同时支持图片和视频输入的模型（如 Seedance），CLI 按文件扩展名将 `-i` 文件放入 `images` 或 `videos` 字段。单个本地文件上限为 **1 GiB**。

```bash
# 本地小文件（<50 MiB）：单次直传
ly image -i ./photo.jpg -p "转换成水彩画风格"

# 本地大文件（≥50 MiB）：CLI 自动切换为分片上传，无需额外参数
ly video -i ./raw-footage.mp4 -p "剪辑成 15 秒预告片"

# 远程 URL 输入无需上传，可直接使用 API Key
ly image -i https://example.com/photo.jpg -p "转换成水彩画风格"
```

### 异步任务、下载和恢复

媒体命令默认执行“提交 → 轮询 → 下载”。成功后会打印 `OUTPUT_FILE`；未指定 `--output` 时，结果保存到默认输出目录（默认为 `~/ly-output`）。

对长任务或 Agent 调用，使用 `--no-wait` 只提交任务：

```bash
ly image --no-wait -p "低多边形山脉，日出"
# TASK_ID: task_123

ly task get task_123
```

JSON 模式下从 `data.task_id` 读取任务 ID：

```bash
ly --json video --no-wait -p "镜头穿越云层"
ly --json task get <task-id>
```

`--no-wait` 每次只创建一个独立任务，但提交完成后可立刻继续提交下一条；CLI 不限制队列或并发，Gateway 决定是否接受、排队和执行。脚本调用时应保存每行 JSON 中的 `data.task_id`：

```bash
while IFS= read -r prompt; do
  [ -n "$prompt" ] && ly --json image --no-wait -p "$prompt"
done < prompts.txt
```

不要为同一个 `task_id` 重复提交。任务轮询中断、超时或下载失败时，保留任务 ID 后运行 `ly task get <task-id>` 查询。

## 鉴权与配置

### 凭据命令

| 命令 | 说明 |
| --- | --- |
| `ly auth login` | 在浏览器完成登录后粘贴 Access Token |
| `ly auth set-key <key>` | 保存 API Key |
| `ly auth show` | 显示鉴权类型、来源和配置路径，不显示密钥原文 |
| `ly auth path` | 显示当前实际配置文件路径 |
| `ly auth logout` | 清除本地保存的 OAuth Token 和 API Key |
| `ly auth set-output-dir <path>` | 设置媒体结果默认保存目录 |

配置文件是普通、可查看和可编辑的 JSON：

| 平台 | 默认路径 |
| --- | --- |
| macOS / Linux | `~/.config/ly/config.json` |
| Windows | `%APPDATA%\ly\config.json` |

Unix 上配置文件权限为仅当前用户可读写。覆盖路径可使用全局选项或环境变量：

```bash
ly --config ./ly-config.json auth path
LY_CONFIG_FILE=./ly-config.json ly auth show
```

凭据解析优先级如下，越靠前优先级越高：

1. `LY_ACCESS_TOKEN`
2. `LY_API_KEY`
3. 配置文件中的 OAuth Access Token
4. 配置文件中的 API Key

`auth logout` 只删除本地凭据，保留 `output_dir` 和默认模型等非凭据设置。环境变量由调用方管理，不会被 `logout` 修改。

## Agent 集成

仓库包含 [`skills/SKILL.md`](skills/SKILL.md)，可显式复制或安装到所使用 Agent 的 skills 目录。CLI 不会自动改写 OpenClaw、Codex 或其他 Agent 的工作区规则。

为 Agent 或脚本使用 `--json`。成功和失败均在 stdout 输出一个 JSON 信封；进度信息写入 stderr。失败以非零状态码退出。

提交到 Gateway 的 prompt、`--param` 和 JSON 嵌套字符串必须是 UTF-8。CLI 在发出 JSON 请求前检测无效 UTF-8，并拒绝发送而非静默替换字符；本地媒体二进制文件不受此限制。

```json
{"ok": true, "data": {"reply": "...", "model": "..."}}
{"ok": true, "data": {"task_id": "task_123", "status": "pending"}}
{"ok": true, "data": {"files": ["/path/result.png"], "task_id": "task_123"}}
{"ok": false, "data": {"code": "INSUFFICIENT_BALANCE", "message": "余额不足", "request_id": "req_123"}}
```

建议的调用流程：

1. `ly --json check` 确认鉴权与模型可用性。
2. `ly --json model list` / `ly --json model info <id>` 动态选择模型和字段。
3. 使用 `ly --json <command> --dry-run` 预览请求；dry-run 不上传文件也不提交任务。
4. 对需要连续提交的媒体任务，为每条请求添加 `--no-wait`，解析并持久化各自的 `data.task_id`；CLI 完成提交后即可发起下一条，Gateway 负责队列与并发调度。
5. 读取 `ok`。失败时读取 `data.code`、`data.message` 及可选的 `data.request_id`；有 `data.task_id` 时用 `task get` 恢复，不要对同一任务重复提交。

Gateway 返回的结构化错误会被保留。例如 `INSUFFICIENT_BALANCE` 表示由 Gateway 统一计费后的上游结果，CLI 只透传并非零退出，不会自行扣费、猜测余额或重试。

## 全局选项

| 选项 | 说明 |
| --- | --- |
| `--json` | 输出适合 Agent 解析的 JSON 信封 |
| `--dry-run` | 预览请求，不上传文件或提交任务 |
| `--config <path>` | 覆盖默认配置文件路径 |
| `-v, --verbose` | 输出详细信息 |
| `--version` | 显示版本 |

使用 `ly <command> --help` 查看各子命令支持的参数。

## 常见问题

### `ly: command not found`

重新打开终端以加载 PATH；curl 安装可按安装器提示执行 `source`。也可直接检查安装目录，例如 `~/.local/bin/ly --help`。

### 本地文件上传被拒绝

OAuth Access Token 和 API Key 均可上传本地文件。请先运行 `ly check` 确认鉴权和 Gateway 连通性；确认单个文件不超过 1 GiB，且本地路径可读，或将本地文件替换为远程 `https://` URL。

### 媒体任务超时或调用中断

从先前输出中取得 `task_id`，然后运行 `ly task get <task-id>`。这不会创建新任务。

### Gateway 返回参数错误或余额不足

读取 JSON 错误中的 `data.code`、`data.message` 和 `data.request_id`。根据 Gateway 提示更正请求字段；`INSUFFICIENT_BALANCE` 需要在 Gateway 侧处理，不应由 CLI 重试。

## 开发

```bash
git clone https://github.com/Din-Studio/lingying-cli.git
cd lingying-cli

GOTOOLCHAIN=go1.26.5 go test ./...
GOTOOLCHAIN=go1.26.5 go vet ./...
GOTOOLCHAIN=go1.26.5 make build
scripts/validate-skill.sh
```

项目采用 Go + Cobra；HTTP mock 覆盖本地上传、任务提交、轮询和下载的完整传输生命周期。

### 发版

**本项目不使用 GitHub Actions 发版**，全部在本地执行。仓库内没有 workflow 文件——推送 tag 不会触发任何自动化。

```bash
make check                  # vet + test -race + skill 契约 + shellcheck
make snapshot               # 本地构建全平台产物，不发布；核对 dist/
make release TAG=v0.1.8     # 打 tag、推送、goreleaser 发布
```

`make release` 在打 tag 前会依次校验：TAG 格式、工作区干净、当前在 `main`、tag 未占用、凭据可用（`GITHUB_TOKEN` 或已登录的 `gh`）。任一不满足即中止，不会留下半成品 tag。

发布后验证：

```bash
curl -fsSL https://github.com/Din-Studio/lingying-cli/releases/latest/download/install.sh | bash
ly --version
```

注意 GitHub 的 `releases/latest` 在各边缘节点间收敛需要约一分钟。刚发完版立刻执行 `ly update` 有可能仍拿到上一个版本，稍等再试即可。

发布的资产名**不含版本号**（`ly-darwin-arm64.tar.gz`）。版本由 URL 路径承载，使 `releases/latest/download/X` 与 `releases/download/v<版本>/X` 指向同一份文件，安装脚本因此无需先发现版本号。

## 许可证

MIT
