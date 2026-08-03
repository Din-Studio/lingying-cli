# ly — Lingying Gateway CLI

[![Go](https://img.shields.io/badge/Go-1.23%2B-blue.svg)](https://go.dev/)
[![License](https://img.shields.io/badge/License-MIT-green.svg)](LICENSE)

AI 模型网关命令行工具。在终端中调用当前账号有权限使用的文本、图片、视频与音频模型。

```bash
ly text "解释量子计算"
ly image -p "赛博朋克猫"
ly video -p "cinematic flythrough" --model seedance2.0
ly audio "一段播客开场白"
```

---

## 安装

### curl（推荐，零依赖）

```bash
curl -fsSL https://raw.githubusercontent.com/Din-Studio/lingying-cli/main/scripts/install.sh | bash
```

### npm

```bash
npm install -g @lingying/cli
```

### go install

```bash
go install github.com/Din-Studio/lingying-cli@latest
```

### 源码编译

```bash
git clone https://github.com/Din-Studio/lingying-cli.git
cd lingying-cli
make install
```

---

## 快速开始

```bash
# 1. 登录
ly auth login
# 或使用 API Key
ly auth set-key <your-key>

# 2. 检查连通性
ly check

# 3. 文本对话
ly text "用三句话解释什么是 Docker"

# 4. 图片生成
ly image -p "一只戴着宇航头盔的柴犬，3D 渲染"

# 5. 浏览所有模型
ly model list
```

---

## 命令

### 快捷命令

| 命令 | 说明 |
|------|------|
| `ly text "..."` | 文本对话，默认模型 |
| `ly text --model <id> "..."` | 指定模型对话 |
| `ly image -p "..."` | 图片生成，默认 image-2 |
| `ly image -p "..." -i ./photo.jpg` | 图生图（需 OAuth） |
| `ly image --model <id> -p "..."` | 指定图片模型 |
| `ly video -p "..."` | 视频生成 |
| `ly audio "..."` | 音频生成 |

### 模型发现

| 命令 | 说明 |
|------|------|
| `ly model list` | 列出所有可用模型 |
| `ly model list --type image` | 按类型过滤 |
| `ly model info <id>` | 查看模型详情 + 参数 schema |
| `ly model search <关键词>` | 搜索模型 |
| `ly task get <task-id>` | 查询已提交的媒体任务 |

### 鉴权

| 命令 | 说明 |
|------|------|
| `ly auth login` | OAuth 浏览器登录（支持文件上传） |
| `ly auth set-key <key>` | 设置 API Key |
| `ly auth show` | 查看当前鉴权状态 |
| `ly auth path` | 显示本地配置文件路径 |
| `ly auth logout` | 清除本地保存的凭据 |
| `ly auth set-output-dir <path>` | 设置媒体结果默认输出目录 |
| `ly check` | 检查连通性和可用模型 |

### 全局选项

| 选项 | 说明 |
|------|------|
| `--json` | JSON 输出（Agent 模式） |
| `--dry-run` | 预览请求不执行 |
| `-v, --verbose` | 详细输出 |
| `--config <path>` | 覆盖默认的配置文件路径 |
| `--version` | 显示版本 |

---

## Agent 集成

所有命令支持 `--json` 输出统一信封。该模式下 stdout 只包含一个 JSON 对象，进度信息写入 stderr：

```json
{"ok": true, "data": {"reply": "..."}}
{"ok": true, "data": {"files": ["/path/result.png"], "task_id": "xxx"}}
{"ok": false, "data": {"code": "task_incomplete", "message": "错误描述", "task_id": "xxx"}}
```

Agent 应先读取 `ok`，失败时读取 `data.code` 和 `data.message`。Gateway 返回的错误码（如 `INSUFFICIENT_BALANCE`）会保留并以非零退出码结束，不由 CLI 计费或重试。异步任务可以使用 `--no-wait` 只获得 `task_id`，或在中断/超时后用 `ly --json task get <task-id>` 恢复查询。非交互环境推荐设置 `LY_ACCESS_TOKEN` 或 `LY_API_KEY`，避免把密钥放进命令参数。`skills/SKILL.md` 可随源码或 npm 包导入支持 Agent Skills 的客户端。

---

## 鉴权模式

### 本地配置

认证信息写入固定的用户配置文件，便于查看、备份或手动修改：macOS/Linux 为 `~/.config/ly/config.json`，Windows 为 `%APPDATA%\\ly\\config.json`。可通过 `ly auth path` 获取当前实际路径，或通过 `--config <path>` / `LY_CONFIG_FILE` 覆盖。`ly auth logout` 只清除本地凭据，会保留默认输出目录等非凭据配置。

解析优先级为 `LY_ACCESS_TOKEN`、`LY_API_KEY`、本地配置文件；环境变量适合 CI 和一次性调用，会覆盖文件中的凭据。配置文件仅保存 OAuth Token、API Key、默认模型和输出目录，Unix 文件权限为仅当前用户可读写。

| | OAuth | API Key |
|------|--------|----------|
| 文本对话 | ✅ | ✅ |
| 图片生成（URL 输入） | ✅ | ✅ |
| 图片生成（本地文件） | ✅ | ❌ |
| 视频生成（URL 输入） | ✅ | ✅ |
| 视频生成（本地文件） | ✅ | ❌ |

OAuth 模式自动将本地文件上传到 file.echojoy.cn 获取 download_url 后调用 Gateway。

---

## 项目结构

```
ly-cli/
├── main.go                    # 入口
├── cmd/                       # cobra 命令层
│   ├── root.go                # 根命令 + 全局 flags
│   ├── text.go                # ly text
│   ├── media.go               # ly image / video / audio
│   ├── model.go               # ly model list / info / search
│   ├── auth.go                # ly auth login / set-key / show
│   └── check.go               # ly check
├── internal/
│   ├── auth/config.go         # 凭证存储和解析
│   ├── client/gateway.go      # HTTP 客户端
│   ├── output/envelope.go     # 统一输出信封
│   └── registry/modelmap.go   # model_type → 端点映射
├── scripts/
│   ├── install.sh             # curl | bash 安装脚本
│   ├── install.js             # npm postinstall
│   └── run.js                 # npm bin 入口
├── skills/SKILL.md            # Agent 操作手册
├── Makefile
└── package.json
```

## 开发

```bash
git clone https://github.com/Din-Studio/lingying-cli.git
cd lingying-cli
make build    # 编译
make vet      # 静态检查
./ly check    # 测试连通性
```

---

## 许可证

MIT
