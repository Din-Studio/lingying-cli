---
name: lingying-gateway
version: 1.0.0
description: "Use when a user needs Lingying Gateway text, image, video, or audio generation from a conventional command-line environment."
metadata:
  requires:
    bins: ["ly"]
  cliHelp: "ly --help"
---

# Lingying Gateway (ly)

**CRITICAL — Agent 必须使用 `--json`。stdout 只会输出一个统一信封 `{ok, data, meta}`；先检查 `ok`，再读 `data`。**

**CRITICAL — 所有提交到 Gateway 的文本必须为 UTF-8。** Prompt、`--param` 值以及 JSON object/array 内的字符串都应以 UTF-8 编码传入。CLI 会在发送请求前检测无效 UTF-8 并以非零退出，绝不静默替换为乱码；图片、视频、音频等二进制文件不受此规则限制。

## 鉴权

| 命令 | 说明 |
|------|------|
| `ly auth login` | 仅交互式 OAuth Token 粘贴登录（支持文件上传；Agent 不要加 `--json` 调用） |
| `ly auth set-key <key>` | API Key (支持本地文件上传) |
| `ly auth show` | 查看当前鉴权状态 |
| `ly auth path` | 显示本地配置文件路径 |
| `ly check` | 检查连通性和可用模型 |

无人值守调用必须设置 `LY_ACCESS_TOKEN`（OAuth）或 `LY_API_KEY`，不要把密钥放到 `ly auth set-key` 的命令行参数中，也不要调用交互式 `auth login`。持久化配置位于 macOS/Linux `~/.config/ly/config.json` 或 Windows `%APPDATA%\\ly\\config.json`；运行 `ly auth path` 可查看实际位置。

OAuth 与 API Key 均支持本地文件上传 (经 Gateway 代理上传并拿到 download_url)。

## 快捷命令

### 文本对话

```bash
ly text "解释量子计算"                                          # 动态发现的默认模型
ly text --model <model-id> "写一段 Go 代码"                       # 指定模型
ly text --max-tokens 4096 "写一篇文章"                          # 控制输出长度
ly text --param temperature=0.7 "写一个更有创意的标题"            # 动态 schema 字段
```

未指定模型时，CLI 从本次 Gateway 动态发现结果中为目标类型选择一个稳定的默认模型。指定的 `--model` 或配置中的默认模型必须精确匹配 ID 或 display_name，CLI 不会自动换成其他模型。

### 图片生成

```bash
ly image -p "赛博朋克猫"                                       # 动态发现的默认图片模型
ly image --model nanobanana2 -p "cat"                          # 指定模型
ly image -p "把背景换成海滩" -i ./photo.jpg                     # 图生图 (本地文件)
ly image --model 抠图 -i ./photo.jpg                            # 图片编辑
ly image -p "cat" --param resolution=4K --param aspect_ratio=16:9  # 附加参数
```

### 视频生成

```bash
ly video -p "cinematic flythrough"                             # 动态发现的默认视频模型
ly video --model "seedance2.0 fast" -p "cat running"            # 指定模型
ly video -p "dog" --param duration=10                            # 控制时长
```

### 音频生成

```bash
ly audio "你好世界"                                             # 默认模型
ly audio --model 音频智能克隆 -i ./voice.mp3 "要合成的文本"      # 语音克隆
ly audio --model 音频智能设计 "一段播客开场白"                   # 语音设计
```

## 模型发现

```bash
ly model list                        # 列出所有可用模型
ly model list --type image           # 只显示图片模型
ly model list --type text            # 只显示文本模型
ly model info <id>                   # 查看模型详情 + input_schema
ly model search <keyword>            # 搜索模型 (display_name 模糊匹配)
```

## Agent 使用

所有命令加 --json 输出统一信封:

```json
{"ok": true, "data": {"reply": "...", "model": "<discovered-model-id>"}}
{"ok": true, "data": {"files": ["/path/result.png"], "task_id": "xxx", "duration": 42}}
{"ok": false, "data": {"message": "未配置鉴权"}}
```

Agent 工作流:
1. 先 ly --json check 检查鉴权和可用模型
2. ly --json model list 了解可选模型
3. ly --json model info <精确-id> 查看参数 schema（`data.input_schema` 已是 JSON object；后续 `--model` 使用同一精确 ID）
4. ly --json <命令> --dry-run 预览请求（不会上传文件或提交任务；text 的 data.request 是实际顶层请求体）
5. ly --json <命令> 执行
6. 如果 ok=false，读 data.code 和 data.message；Gateway 的错误码（例如 INSUFFICIENT_BALANCE）直接处理，不要在 CLI 端扣费或重试；若有 data.task_id，用 ly --json task get <id> 恢复查询，不要重新提交

--dry-run 预览请求不执行，用于 Agent 验证参数后再提交。

媒体任务可添加 `--no-wait`：CLI 只提交并返回 `data.task_id`，后续用 `ly --json task get <task_id>` 查询状态或结果。每条 `--no-wait` 命令是一个独立任务；提交后可以立即提交下一条，记录每个 `data.task_id`，由 Gateway 决定是否接收、排队和并发执行。不要对同一 `task_id` 重复提交。

批量提交示例（每行一条提示词）：

```bash
while IFS= read -r prompt; do
  [ -n "$prompt" ] && ly --json image --no-wait -p "$prompt"
done < prompts.txt
```

## 参数类型


`--param` 对本次模型 schema 编码：
- string 保持原文（例如 `9:16`、`1K`）
- integer / number 使用严格完整数值解析
- boolean 支持 true/false、yes/no、1/0
- 嵌套 object 使用点路径，例如 `--param metadata.quality=high`
- object / array 整体值使用 JSON，例如 `--param 'metadata={"quality":"high"}'`、`--param 'images=["https://example.com/a.png"]'`
- 未知顶层字段保持 string，由 Gateway 校验；未声明的嵌套路径会在 CLI 端拒绝，避免生成字面量 `metadata.quality` 这类无效键

对 Gateway schema 中“必填且有 default”的缺失字段，CLI 会自动补齐；`--param` 显式传入的值优先。其他非法参数会被 Gateway 拒绝，Agent 读错误后修正重试。CLI 不做完整 schema 校验。位置提示词里的 `=` 是普通文本，额外字段必须显式使用 `--param`。

## 错误处理

| 场景 | 处理 |
|------|------|
| 未鉴权 | 引导用户 ly auth login 或 ly auth set-key |
| Gateway 422 参数错误 | 读 error 中的字段提示，修改参数后重试 |
| 轮询失败 | 自动重试 5 次后熔断 |
| 任务超时 | 保存 task_id，运行 `ly --json task get <task_id>` 查询 |
