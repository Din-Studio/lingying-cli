// Copyright (c) 2026 Lingying. SPDX-License-Identifier: MIT
// ly image / video / audio — async media commands.
// All share: submit → poll → download.
package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
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
		inputData[parts[0]] = guessType(parts[1])
	}

	if dryRun {
		if len(inputFiles) > 0 {
			inputData[guessFieldName(matched, matched.ModelType)] = inputFiles
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
		if strings.HasPrefix(fi, "http://") || strings.HasPrefix(fi, "https://") {
			fieldName := guessFieldName(matched, matched.ModelType)
			if _, exists := inputData[fieldName]; !exists {
				inputData[fieldName] = []string{}
			}
			arr := inputData[fieldName].([]string)
			inputData[fieldName] = append(arr, fi)
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
			fieldName := guessFieldName(matched, matched.ModelType)
			if _, exists := inputData[fieldName]; !exists {
				inputData[fieldName] = []string{}
			}
			arr := inputData[fieldName].([]string)
			inputData[fieldName] = append(arr, url)
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
	if matched != nil && matched.FeatureTypes != nil {
		for _, feat := range matched.FeatureTypes {
			for _, mf := range feat.MatchFields {
				if mf == "images" || mf == "videos" || mf == "audios" {
					return mf
				}
			}
		}
	}
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
		p := getPromptFromArgs(args, &imageParamFlags)
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
		p := getPromptFromArgs(args, &videoParamFlags)
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
		p := getPromptFromArgs(args, &audioParamFlags)
		if p != "" {
			audioPromptFlag = p
		}
		return runMedia(cmd, map[string]bool{"audio": true, "audio_edit": true}, audioModelFlag, audioPromptFlag,
			audioInputFiles, audioParamFlags, audioOutPath, audioNoWait)
	},
}

// ── Helpers ──

func getPromptFromArgs(args []string, params *[]string) string {
	var parts []string
	for _, a := range args {
		if strings.Contains(a, "=") {
			*params = append(*params, a)
		} else {
			parts = append(parts, a)
		}
	}
	return strings.Join(parts, " ")
}

func guessType(raw string) any {
	switch strings.ToLower(raw) {
	case "true", "yes", "1":
		return true
	case "false", "no", "0":
		return false
	}
	var n int
	if _, err := fmt.Sscanf(raw, "%d", &n); err == nil {
		return n
	}
	var f float64
	if _, err := fmt.Sscanf(raw, "%f", &f); err == nil && strings.Contains(raw, ".") {
		return f
	}
	return raw
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
