# ly — Lingying Gateway CLI

[![Go](https://img.shields.io/badge/Go-1.23%2B-blue.svg)](https://go.dev/)
[![License](https://img.shields.io/badge/License-MIT-green.svg)](LICENSE)

AI 模型网关命令行工具。在终端中直接调用 36 个 AI 模型——文本对话、图片生成、视频生成、音频生成。

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

### 鉴权

| 命令 | 说明 |
|------|------|
| `ly auth login` | OAuth 浏览器登录（支持文件上传） |
| `ly auth set-key <key>` | 设置 API Key |
| `ly auth show` | 查看当前鉴权状态 |
| `ly check` | 检查连通性和可用模型 |

### 全局选项

| 选项 | 说明 |
|------|------|
| `--json` | JSON 输出（Agent 模式） |
| `--dry-run` | 预览请求不执行 |
| `-v, --verbose` | 详细输出 |
| `--version` | 显示版本 |

---

## Agent 集成

所有命令支持 `--json` 输出统一信封：

```json
{"ok": true, "data": {"reply": "..."}}
{"ok": true, "data": {"files": ["/path/result.png"], "task_id": "xxx"}}
{"ok": false, "data": {"message": "错误描述"}}
```

Agent 读取 `ok` 一个字段判断成功/失败。`ly` 的 SKILL.md 文件位于 `skills/SKILL.md`，可直接导入到支持 Agent Skills 的客户端中。

---

## 鉴权模式

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
