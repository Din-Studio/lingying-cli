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

## 鉴权

| 命令 | 说明 |
|------|------|
| `ly auth login` | OAuth 浏览器登录 (支持文件上传) |
| `ly auth set-key <key>` | API Key (不支持本地文件上传) |
| `ly auth show` | 查看当前鉴权状态 |
| `ly auth path` | 显示本地配置文件路径 |
| `ly check` | 检查连通性和可用模型 |

无人值守调用优先设置 `LY_ACCESS_TOKEN`（OAuth）或 `LY_API_KEY`，不要把密钥放到 `ly auth set-key` 的命令行参数中。持久化配置位于 macOS/Linux `~/.config/ly/config.json` 或 Windows `%APPDATA%\\ly\\config.json`；运行 `ly auth path` 可查看实际位置。

OAuth 模式支持本地文件上传 (自动上传到 file.echojoy.cn 并拿到 download_url)。
API Key 模式只能传远程 URL，不能传本地路径。

## 快捷命令

### 文本对话

```bash
ly text "解释量子计算"                                          # 默认模型
ly text --model claude-sonnet-4.6 "写一段 Go 代码"              # 指定模型
ly text --max-tokens 4096 "写一篇文章"                          # 控制输出长度
```

默认模型: claude-sonnet-4.6。每次调用会发现当前账号允许的模型；指定的模型必须精确匹配 ID 或 display_name，CLI 不会自动换成其他模型。

### 图片生成

```bash
ly image -p "赛博朋克猫"                                       # 默认模型 image-2
ly image --model nanobanana2 -p "cat"                          # 指定模型
ly image -p "把背景换成海滩" -i ./photo.jpg                     # 图生图 (需 OAuth)
ly image --model 抠图 -i ./photo.jpg                            # 图片编辑
ly image -p "cat" --param resolution=4K --param aspect_ratio=16:9  # 附加参数
```

### 视频生成

```bash
ly video -p "cinematic flythrough"                             # 默认 seedance2.0
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
{"ok": true, "data": {"reply": "...", "model": "claude-sonnet-4.6"}}
{"ok": true, "data": {"files": ["/path/result.png"], "task_id": "xxx", "duration": 42}}
{"ok": false, "data": {"message": "未配置鉴权"}}
```

Agent 工作流:
1. 先 ly --json check 检查鉴权和可用模型
2. ly --json model list 了解可选模型
3. ly --json model info <id> 查看参数 schema
4. ly --json <命令> --dry-run 预览请求（不会上传文件或提交任务）
5. ly --json <命令> 执行
6. 如果 ok=false，读 data.code 和 data.message；若有 data.task_id，用 ly --json task get <id> 恢复查询，不要重新提交

--dry-run 预览请求不执行，用于 Agent 验证参数后再提交。

## 参数类型

--param 传参时 CLI 自动猜测类型:
- true/false/yes/no/1/0 → boolean
- 纯数字 → integer
- 数字含小数点 → float
- 其他 → string

非法参数会被 Gateway 拒绝，Agent 读错误后修正重试。CLI 不校验。

## 错误处理

| 场景 | 处理 |
|------|------|
| 未鉴权 | 引导用户 ly auth login 或 ly auth set-key |
| API Key + 本地文件 | 提示切换到 OAuth 或使用远程 URL |
| Gateway 422 参数错误 | 读 error 中的字段提示，修改参数后重试 |
| 轮询失败 | 自动重试 5 次后熔断 |
| 任务超时 | 保存 task_id，运行 `ly --json task get <task_id>` 查询 |
