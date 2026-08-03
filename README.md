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

支持 macOS（Intel/Apple Silicon）、Linux（x64/arm64）和 Windows（x64/arm64，推荐 npm）。安装后运行 `ly --help` 验证命令可用。

### curl：macOS、Linux 和 WSL

安装脚本下载对应平台的发布包，校验 SHA-256，并将二进制放到 `~/.local/bin`。可用 `LY_INSTALL_DIR` 覆盖安装目录，`LY_VERSION` 固定版本。

```bash
curl -fsSL https://raw.githubusercontent.com/Din-Studio/lingying-cli/main/scripts/install.sh | bash
ly --help
```

如脚本提示 PATH 尚未生效，请重新打开终端或按提示执行 `source`。脚本在没有预编译包时会回退到 `go install`。

### npm：macOS、Linux 和 Windows

需要 Node.js 16 或更高版本。npm 安装包会下载并校验当前平台的二进制；Windows 请优先使用此方式。

```bash
npm install -g lingying-cli
ly --help
```

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

## 快速开始

```bash
# 1. 保存 OAuth Access Token（支持本地文件上传）
ly auth login

# 或保存 API Key（可调用文本与使用 URL 的媒体任务）
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
| `ly model list` | 列出当前账号可用模型 |
| `ly model list --type image` | 按模型类型筛选 |
| `ly model info <id>` | 查看模型详情与 Gateway 返回的 `input_schema` |
| `ly model search <keyword>` | 搜索模型 ID、名称或类型 |
| `ly check` | 检查鉴权、Gateway 连通性和模型数量 |

CLI 每次调用均从 Gateway 动态发现模型，不缓存或内置静态能力目录。未传 `--model` 时，CLI 从本次发现结果中为命令类型稳定选择一个可用模型；显式 `--model` 仍必须精确匹配，不会悔悔换模型。调用前可先用 `model info` 查看模型当前的输入 schema；字段是否合法由 Gateway 返回最终结果。

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

`--param` 会将 `true`、`false`、`yes`、`no`、`1`、`0` 识别为布尔值，将整数和小数识别为数值；其他内容按字符串传给 Gateway。CLI 会从本次模型 `input_schema` 补齐缺失的“必填且带 default”字段，用户传入的值优先；不做完整本地 schema 校验，其他字段错误仍由 Gateway 返回。

输入既可以是本地路径，也可以是 `http://` 或 `https://` URL。URL 输入可使用 OAuth 或 API Key；本地输入需要 OAuth，CLI 会通过 `file.echojoy.cn` 的预签名上传链路上传。单个本地文件上限为 **1 GiB**。

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

任务轮询中断、超时或下载失败时，保留任务 ID 后运行 `ly task get <task-id>` 查询，不要盲目重新提交。

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

仓库和 npm 包都包含 [`skills/SKILL.md`](skills/SKILL.md)，可显式复制或安装到所使用 Agent 的 skills 目录。CLI 不会自动改写 OpenClaw、Codex 或其他 Agent 的工作区规则。

为 Agent 或脚本使用 `--json`。成功和失败均在 stdout 输出一个 JSON 信封；进度信息写入 stderr。失败以非零状态码退出。

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
4. 执行命令；媒体长任务可加 `--no-wait`。
5. 读取 `ok`。失败时读取 `data.code`、`data.message` 及可选的 `data.request_id`；有 `data.task_id` 时用 `task get` 恢复。

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

本地文件上传只接受 OAuth Access Token。运行 `ly auth login`，或将本地文件替换为远程 `https://` URL。请确认单个文件不超过 1 GiB，且本地路径可读。

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

## 许可证

MIT
