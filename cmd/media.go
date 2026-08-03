// Copyright (c) 2026 Lingying. SPDX-License-Identifier: MIT
// ly image / video / audio — async media commands.
// All share: submit → poll → download.
package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/Din-Studio/lingying-cli/internal/auth"
	"github.com/Din-Studio/lingying-cli/internal/client"
	"github.com/Din-Studio/lingying-cli/internal/output"
	"github.com/Din-Studio/lingying-cli/internal/registry"
	"github.com/spf13/cobra"
)

// ── Shared async runner ──

func runMedia(
	cmd *cobra.Command,
	modelTypes map[string]bool,
	modelFlag string,
	prompt string,
	inputFiles []string,
	extraParams []string,
	outputPath string,
	noWait bool,
) error {
	jsonMode := isJSON(cmd)
	dryRun := isDryRun(cmd)

	resolved := auth.Resolve()
	if resolved.Value == "" {
		env := output.Envelope{OK: false, Data: map[string]any{
			"message": "未配置鉴权，请先 ly auth login 或 ly auth set-key",
		}}
		if jsonMode {
			return output.JSON(env)
		}
		fmt.Println("❌ 未配置鉴权。请先 ly auth login 或 ly auth set-key")
		return nil
	}

	if len(inputFiles) > 0 && !resolved.HasUpload() {
		env := output.Envelope{OK: false, Data: map[string]any{
			"message": "本地文件上传仅 OAuth 模式下可用，请先 ly auth login。或传入 URL 而非本地路径。",
		}}
		if jsonMode {
			return output.JSON(env)
		}
		fmt.Println("❌ 本地文件上传仅 OAuth 模式下可用。请先 ly auth login，或传入远程 URL 而非本地文件路径。")
		return nil
	}

	c := client.New(resolved.Value)
	ctx := context.Background()
	allModels, err := c.ListModels(ctx)
	if err != nil {
		return err
	}

	var candidates []client.GatewayModel
	for _, m := range allModels {
		if modelTypes[m.ModelType] {
			candidates = append(candidates, m)
		}
	}
	if len(candidates) == 0 {
		return fmt.Errorf("没有找到可用的目标类型模型")
	}

	// Pick model
	modelID := modelFlag
	matched, err := selectMediaModel(allModels, modelTypes, modelID)
	if err != nil {
		return err
	}
	modelID = matched.ModelID
	if modelID == "" {
		return fmt.Errorf("模型 %s 缺少 model_id", matched.ID)
	}
	entry, ok := registry.Lookup(matched.ModelType)
	if !ok {
		return fmt.Errorf("模型 %s 使用了不支持的类型 %s", matched.ID, matched.ModelType)
	}

	if prompt == "" {
		showModelList(candidates)
		fmt.Printf("当前选择: %s (%s)\n", matched.DisplayName, matched.ID)
		fmt.Print("提示词: ")
		fmt.Scanln(&prompt)
		if prompt == "" {
			if jsonMode {
				return output.JSON(output.Envelope{OK: false, Data: map[string]any{"message": "未输入提示词"}})
			}
			return nil
		}
	}

	inputData := map[string]any{"prompt": prompt}
	for _, p := range extraParams {
		parts := strings.SplitN(p, "=", 2)
		if len(parts) != 2 || parts[0] == "" {
			return fmt.Errorf("非法 --param %q，应为 key=value", p)
		}
		if err := applySchemaParam(inputData, parts[0], parts[1], matched.InputSchema); err != nil {
			return fmt.Errorf("非法 --param %q: %w", p, err)
		}
	}
	applyRequiredSchemaDefaults(inputData, matched.InputSchema)

	if dryRun {
		for _, file := range inputFiles {
			if err := appendInputURL(inputData, inputFieldForFile(matched, matched.ModelType, file), file); err != nil {
				return err
			}
		}
		env := output.Envelope{OK: true, Data: map[string]any{
			"model_name": matched.DisplayName,
			"model_id":   modelID,
			"endpoint":   entry.Endpoint,
			"input_data": inputData,
			"dry_run":    true,
		}}
		if jsonMode {
			return output.JSON(env)
		}
		fmt.Printf("[dry-run] %s → %s\n   input: %v\n", matched.DisplayName, entry.Endpoint, inputData)
		return nil
	}

	for _, fi := range inputFiles {
		fieldName := inputFieldForFile(matched, matched.ModelType, fi)
		if strings.HasPrefix(fi, "http://") || strings.HasPrefix(fi, "https://") {
			if err := appendInputURL(inputData, fieldName, fi); err != nil {
				return err
			}
		} else {
			name := filepath.Base(fi)
			if !jsonMode {
				fmt.Fprintf(cmd.ErrOrStderr(), "上传 %s... ", name)
			}
			url, err := c.UploadFile(ctx, name, fi)
			if err != nil {
				return err
			}
			if !jsonMode {
				fmt.Fprintln(cmd.ErrOrStderr(), "完成")
			}
			if err := appendInputURL(inputData, fieldName, url); err != nil {
				return err
			}
		}
	}

	if !jsonMode {
		fmt.Fprintf(cmd.ErrOrStderr(), "使用模型: %s\n提交任务", matched.DisplayName)
	}
	taskID, err := c.SubmitTask(ctx, c.MediaEndpoint(entry.Endpoint), modelID, inputData)
	if err != nil {
		return fmt.Errorf("提交失败: %w", err)
	}
	if !jsonMode {
		fmt.Fprintln(cmd.ErrOrStderr(), " - 完成")
	}
	if noWait {
		env := output.Envelope{OK: true, Data: map[string]any{
			"task_id": taskID,
			"status":  "pending",
			"model":   matched.DisplayName,
		}, Meta: &output.Meta{TaskID: taskID}}
		if jsonMode {
			return output.JSON(env)
		}
		fmt.Printf("TASK_ID: %s\n", taskID)
		return nil
	}

	start := time.Now()
	if !jsonMode {
		fmt.Fprint(cmd.ErrOrStderr(), "轮询中")
	}
	taskBody, err := c.PollTask(ctx, taskID, 3, 1200)
	if err != nil {
		if jsonMode {
			detail := client.ErrorDetails(err)
			extra := map[string]any{"task_id": taskID}
			if detail.RequestID != "" {
				extra["request_id"] = detail.RequestID
			}
			return output.JSON(output.Failure(detail.Code, detail.Message, extra))
		}
		return fmt.Errorf("❌ %v (task_id: %s)", err, taskID)
	}
	elapsed := int64(time.Since(start).Seconds())
	if !jsonMode {
		fmt.Fprintf(cmd.ErrOrStderr(), " - 完成 (%ds)\n", elapsed)
	}

	urls, err := client.ExtractResultURLs(taskBody)
	if err != nil {
		return err
	}
	if len(urls) == 0 {
		if jsonMode {
			return output.JSON(output.Failure("missing_result", "任务已完成但未返回可下载文件", map[string]any{"task_id": taskID}))
		}
		return fmt.Errorf("任务已完成但未返回可下载文件 (task_id: %s)", taskID)
	}

	outDir := outputDir()
	if err := os.MkdirAll(outDir, 0755); err != nil {
		return err
	}
	files := make([]string, 0, len(urls))
	for i, url := range urls {
		outFile := filepath.Join(outDir, fmt.Sprintf("result-%s-%d.%s", taskID, i+1, entry.OutputExt))
		if outputPath != "" && i == 0 {
			outFile = outputPath
		}
		if err := c.Download(ctx, url, outFile); err != nil {
			if jsonMode {
				return output.JSON(output.Failure("download_failed", err.Error(), map[string]any{"task_id": taskID, "files": files}))
			}
			return err
		}
		files = append(files, outFile)
	}

	env := output.Envelope{
		OK: true,
		Data: map[string]any{
			"files":    files,
			"task_id":  taskID,
			"duration": elapsed,
			"model":    matched.DisplayName,
		},
		Meta: &output.Meta{TaskID: taskID, Duration: elapsed},
	}
	if jsonMode {
		return output.JSON(env)
	}
	fmt.Printf("OUTPUT_FILE: %s\n", files[0])
	return nil
}

func showModelList(candidates []client.GatewayModel) {
	for i, m := range candidates {
		fmt.Printf("  %2d. %-30s %s\n", i+1, m.ID, m.DisplayName)
	}
}

func selectMediaModel(models []client.GatewayModel, eligible map[string]bool, requested string) (*client.GatewayModel, error) {
	if requested == "" {
		return selectDynamicModel(models, eligible)
	}
	var found *client.GatewayModel
	for i := range models {
		m := &models[i]
		if m.ID == requested || strings.EqualFold(m.DisplayName, requested) {
			found = m
			break
		}
	}
	if found == nil {
		return nil, fmt.Errorf("未找到模型: %s", requested)
	}
	if !eligible[found.ModelType] {
		return nil, fmt.Errorf("模型 %s 的类型 %s 不适用于此命令", found.ID, found.ModelType)
	}
	return found, nil
}

// selectDynamicModel chooses a stable default from the models returned by the
// current Gateway discovery response. Explicit model selections never call this
// function, so a user-provided ID cannot silently fall back to another model.
func selectDynamicModel(models []client.GatewayModel, eligible map[string]bool) (*client.GatewayModel, error) {
	candidates := make([]*client.GatewayModel, 0)
	for i := range models {
		if eligible[models[i].ModelType] {
			candidates = append(candidates, &models[i])
		}
	}
	if len(candidates) == 0 {
		return nil, fmt.Errorf("没有找到可用的目标类型模型")
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		if candidates[i].ID == candidates[j].ID {
			return candidates[i].DisplayName < candidates[j].DisplayName
		}
		return candidates[i].ID < candidates[j].ID
	})
	return candidates[0], nil
}

func guessFieldName(matched *client.GatewayModel, modelType string) string {
	switch modelType {
	case "image", "image_edit":
		return "images"
	case "video", "video_edit":
		return "videos"
	case "audio_edit":
		return "audios"
	}
	return "images"
}

// inputFieldForFile maps a local or remote media input to a field exposed by
// the selected model. This avoids depending on FeatureTypes map iteration,
// which is deliberately non-deterministic in Go.
func inputFieldForFile(matched *client.GatewayModel, modelType, input string) string {
	wanted := mediaFieldForPath(input)
	if hasInputField(matched, wanted) {
		return wanted
	}
	return guessFieldName(matched, modelType)
}

func mediaFieldForPath(input string) string {
	path := strings.ToLower(strings.SplitN(input, "?", 2)[0])
	switch filepath.Ext(path) {
	case ".mp4", ".mov", ".m4v", ".webm", ".mkv", ".avi", ".mpeg", ".mpg":
		return "videos"
	case ".mp3", ".wav", ".m4a", ".aac", ".flac", ".ogg", ".opus":
		return "audios"
	default:
		return "images"
	}
}

func hasInputField(matched *client.GatewayModel, wanted string) bool {
	if matched == nil {
		return false
	}
	for _, feature := range matched.FeatureTypes {
		for _, field := range feature.MatchFields {
			if field == wanted {
				return true
			}
		}
	}
	return false
}

func appendInputURL(input map[string]any, fieldName, value string) error {
	current, exists := input[fieldName]
	if !exists {
		input[fieldName] = []any{value}
		return nil
	}
	switch values := current.(type) {
	case []any:
		input[fieldName] = append(values, value)
	case []string:
		input[fieldName] = append(values, value)
	default:
		return fmt.Errorf("输入字段 %q 已由 --param 设置为非数组，无法追加文件", fieldName)
	}
	return nil
}

// ── Image ──

var (
	imageModelFlag  string
	imageInputFiles []string
	imageParamFlags []string
	imageOutPath    string
	imagePromptFlag string
	imageNoWait     bool
)

var imageCmd = &cobra.Command{
	Use:   "image",
	Short: "图片生成和编辑",
	RunE: func(cmd *cobra.Command, args []string) error {
		p := getPromptFromArgs(args)
		if p != "" {
			imagePromptFlag = p
		}
		return runMedia(cmd, map[string]bool{"image": true, "image_edit": true}, imageModelFlag, imagePromptFlag,
			imageInputFiles, imageParamFlags, imageOutPath, imageNoWait)
	},
}

// ── Video ──

var (
	videoModelFlag  string
	videoInputFiles []string
	videoParamFlags []string
	videoOutPath    string
	videoPromptFlag string
	videoNoWait     bool
)

var videoCmd = &cobra.Command{
	Use:   "video",
	Short: "视频生成和编辑",
	RunE: func(cmd *cobra.Command, args []string) error {
		p := getPromptFromArgs(args)
		if p != "" {
			videoPromptFlag = p
		}
		return runMedia(cmd, map[string]bool{"video": true, "video_edit": true}, videoModelFlag, videoPromptFlag,
			videoInputFiles, videoParamFlags, videoOutPath, videoNoWait)
	},
}

// ── Audio ──

var (
	audioModelFlag  string
	audioParamFlags []string
	audioOutPath    string
	audioPromptFlag string
	audioInputFiles []string
	audioNoWait     bool
)

var audioCmd = &cobra.Command{
	Use:   "audio",
	Short: "音频生成",
	RunE: func(cmd *cobra.Command, args []string) error {
		p := getPromptFromArgs(args)
		if p != "" {
			audioPromptFlag = p
		}
		return runMedia(cmd, map[string]bool{"audio": true, "audio_edit": true}, audioModelFlag, audioPromptFlag,
			audioInputFiles, audioParamFlags, audioOutPath, audioNoWait)
	},
}

// ── Helpers ──

func getPromptFromArgs(args []string) string {
	return strings.Join(args, " ")
}

// coerceParamValue performs the minimal conversion needed to encode a CLI
// parameter according to the current Gateway schema. Unknown and complex
// fields remain strings so the Gateway can validate them authoritatively.
func coerceParamValue(key, raw string, schema json.RawMessage) any {
	property, exists := inputSchemaProperty(key, schema)
	if !exists {
		return raw
	}
	return coerceSchemaValue(raw, property)
}

func coerceSchemaValue(raw string, property json.RawMessage) any {
	switch schemaNodeType(property) {
	case "boolean":
		switch strings.ToLower(raw) {
		case "true", "yes", "1":
			return true
		case "false", "no", "0":
			return false
		}
	case "integer":
		if value, err := strconv.ParseInt(raw, 10, 64); err == nil {
			return value
		}
	case "number":
		if value, err := strconv.ParseFloat(raw, 64); err == nil {
			return value
		}
	}
	return raw
}

func inputSchemaPropertyType(key string, schema json.RawMessage) string {
	property, exists := inputSchemaProperty(key, schema)
	if !exists {
		return ""
	}
	return schemaNodeType(property)
}

func inputSchemaProperty(key string, schema json.RawMessage) (json.RawMessage, bool) {
	var definition struct {
		Properties map[string]json.RawMessage `json:"properties"`
	}
	if json.Unmarshal(schema, &definition) != nil {
		return nil, false
	}
	property, exists := definition.Properties[key]
	return property, exists
}

func schemaNodeType(property json.RawMessage) string {
	var definition struct {
		Type json.RawMessage `json:"type"`
	}
	if json.Unmarshal(property, &definition) != nil {
		return ""
	}
	var single string
	if json.Unmarshal(definition.Type, &single) == nil {
		return single
	}
	var multiple []string
	if json.Unmarshal(definition.Type, &multiple) != nil {
		return ""
	}
	for _, expected := range []string{"object", "array", "string", "boolean", "integer", "number"} {
		for _, candidate := range multiple {
			if candidate == expected {
				return candidate
			}
		}
	}
	return ""
}

// applySchemaParam serializes a --param value according to the discovered
// input schema. Dot paths address nested object fields. Object and array values
// use JSON so their structure is preserved rather than being sent as strings.
// This is encoding only: validation of value ranges and unknown top-level
// fields remains the Gateway's responsibility.
func applySchemaParam(input map[string]any, path, raw string, schema json.RawMessage) error {
	property, found := schemaPropertyAtPath(path, schema)
	if !found {
		if !strings.Contains(path, ".") {
			input[path] = raw
			return nil
		}
		return fmt.Errorf("嵌套字段 %q 未在模型 input_schema 中声明", path)
	}
	value, err := decodeSchemaParamValue(raw, property)
	if err != nil {
		return err
	}
	return setNestedSchemaValue(input, strings.Split(path, "."), value)
}

func schemaPropertyAtPath(path string, schema json.RawMessage) (json.RawMessage, bool) {
	current := schema
	for _, part := range strings.Split(path, ".") {
		var definition struct {
			Properties map[string]json.RawMessage `json:"properties"`
		}
		if json.Unmarshal(current, &definition) != nil {
			return nil, false
		}
		next, exists := definition.Properties[part]
		if !exists {
			return nil, false
		}
		current = next
	}
	return current, true
}

func decodeSchemaParamValue(raw string, property json.RawMessage) (any, error) {
	switch schemaNodeType(property) {
	case "object":
		var value map[string]any
		if err := json.Unmarshal([]byte(raw), &value); err != nil || value == nil {
			return nil, fmt.Errorf("对象字段必须使用 JSON 对象，例如 --param 'metadata={\"quality\":\"high\"}'")
		}
		return value, nil
	case "array":
		var value []any
		if err := json.Unmarshal([]byte(raw), &value); err != nil || value == nil {
			return nil, fmt.Errorf("数组字段必须使用 JSON 数组，例如 --param 'images=[\"https://example.com/image.png\"]'")
		}
		return value, nil
	default:
		return coerceSchemaValue(raw, property), nil
	}
}

func setNestedSchemaValue(input map[string]any, path []string, value any) error {
	current := input
	for _, part := range path[:len(path)-1] {
		existing, exists := current[part]
		if !exists {
			next := map[string]any{}
			current[part] = next
			current = next
			continue
		}
		next, ok := existing.(map[string]any)
		if !ok {
			return fmt.Errorf("字段 %q 不是对象，无法设置嵌套参数", part)
		}
		current = next
	}
	current[path[len(path)-1]] = value
	return nil
}

// applyRequiredSchemaDefaults copies only required fields with an explicit
// JSON Schema default into input. It deliberately does not validate the full
// schema or send optional defaults: the Gateway remains the source of truth
// for validation and optional-value fallback.
func applyRequiredSchemaDefaults(input map[string]any, schema json.RawMessage) {
	var definition struct {
		Required   []string                   `json:"required"`
		Properties map[string]json.RawMessage `json:"properties"`
	}
	if len(schema) == 0 || json.Unmarshal(schema, &definition) != nil {
		return
	}
	for _, key := range definition.Required {
		if _, exists := input[key]; exists {
			continue
		}
		var property struct {
			Default json.RawMessage `json:"default"`
		}
		if raw, exists := definition.Properties[key]; !exists || json.Unmarshal(raw, &property) != nil || len(property.Default) == 0 {
			continue
		}
		var value any
		if json.Unmarshal(property.Default, &value) == nil {
			input[key] = value
		}
	}
}

func outputDir() string {
	if configured := auth.GetOutputDir(); configured != "" {
		return configured
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, "ly-output")
}

func init() {
	imageCmd.Flags().StringVarP(&imageModelFlag, "model", "m", "", "模型 ID 或名称")
	imageCmd.Flags().StringArrayVarP(&imageInputFiles, "image", "i", nil, "输入图片 (可重复)")
	imageCmd.Flags().StringArrayVar(&imageParamFlags, "param", nil, "附加参数 key=value (可重复)")
	imageCmd.Flags().StringVarP(&imageOutPath, "output", "o", "", "输出文件路径")
	imageCmd.Flags().StringVarP(&imagePromptFlag, "prompt", "p", "", "提示词")
	imageCmd.Flags().BoolVar(&imageNoWait, "no-wait", false, "提交任务后立即返回 task_id")

	videoCmd.Flags().StringVarP(&videoModelFlag, "model", "m", "", "模型 ID 或名称")
	videoCmd.Flags().StringArrayVarP(&videoInputFiles, "image", "i", nil, "输入图片/视频 (可重复)")
	videoCmd.Flags().StringArrayVar(&videoParamFlags, "param", nil, "附加参数 key=value (可重复)")
	videoCmd.Flags().StringVarP(&videoOutPath, "output", "o", "", "输出文件路径")
	videoCmd.Flags().StringVarP(&videoPromptFlag, "prompt", "p", "", "提示词")
	videoCmd.Flags().BoolVar(&videoNoWait, "no-wait", false, "提交任务后立即返回 task_id")

	audioCmd.Flags().StringVarP(&audioModelFlag, "model", "m", "", "模型 ID 或名称")
	audioCmd.Flags().StringArrayVarP(&audioInputFiles, "audio", "i", nil, "输入音频 (可重复)")
	audioCmd.Flags().StringArrayVar(&audioParamFlags, "param", nil, "附加参数 key=value (可重复)")
	audioCmd.Flags().StringVarP(&audioOutPath, "output", "o", "", "输出文件路径")
	audioCmd.Flags().StringVarP(&audioPromptFlag, "prompt", "p", "", "提示词")
	audioCmd.Flags().BoolVar(&audioNoWait, "no-wait", false, "提交任务后立即返回 task_id")
}
