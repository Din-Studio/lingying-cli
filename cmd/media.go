// Copyright (c) 2026 Lingying. SPDX-License-Identifier: MIT
// ly image / video / audio — async media commands.
// All share: submit → poll → download.
package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/lingying/ly-cli/internal/auth"
	"github.com/lingying/ly-cli/internal/client"
	"github.com/lingying/ly-cli/internal/output"
	"github.com/lingying/ly-cli/internal/registry"
	"github.com/spf13/cobra"
)

// ── Shared async runner ──

func runMedia(
	cmd *cobra.Command,
	modelType string,
	defaultModel string,
	modelFlag string,
	prompt string,
	inputFiles []string,
	extraParams []string,
	outputPath string,
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

	entry, ok := registry.Lookup(modelType)
	if !ok {
		return fmt.Errorf("未知的模型类型: %s", modelType)
	}

	c := client.New(resolved.Value)
	ctx := context.Background()
	allModels, err := c.ListModels(ctx)
	if err != nil {
		return err
	}

	var candidates []client.GatewayModel
	for _, m := range allModels {
		if m.ModelType == modelType {
			candidates = append(candidates, m)
		}
	}
	if len(candidates) == 0 {
		return fmt.Errorf("没有找到 %s 类型的模型", modelType)
	}

	// Pick model
	modelID := modelFlag
	if modelID == "" {
		modelID = defaultModel
	}
	var matched *client.GatewayModel
	for i := range candidates {
		m := &candidates[i]
		if m.ID == modelID || strings.EqualFold(m.DisplayName, modelID) ||
			strings.Contains(strings.ToLower(m.DisplayName), strings.ToLower(modelID)) ||
			strings.Contains(strings.ToLower(m.ID), strings.ToLower(modelID)) {
			matched = m
			if m.ModelID != "" {
				modelID = m.ModelID
			}
			break
		}
	}
	if matched == nil {
		matched = &candidates[0]
		modelID = candidates[0].ModelID
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

	for _, fi := range inputFiles {
		if strings.HasPrefix(fi, "http://") || strings.HasPrefix(fi, "https://") {
			fieldName := guessFieldName(matched, modelType)
			if _, exists := inputData[fieldName]; !exists {
				inputData[fieldName] = []string{}
			}
			arr := inputData[fieldName].([]string)
			inputData[fieldName] = append(arr, fi)
		} else {
			name := filepath.Base(fi)
			fmt.Printf("上传 %s... ", name)
			url, err := c.UploadFile(ctx, name, fi)
			if err != nil {
				return err
			}
			fmt.Println("完成")
			fieldName := guessFieldName(matched, modelType)
			if _, exists := inputData[fieldName]; !exists {
				inputData[fieldName] = []string{}
			}
			arr := inputData[fieldName].([]string)
			inputData[fieldName] = append(arr, url)
		}
	}

	for _, p := range extraParams {
		parts := strings.SplitN(p, "=", 2)
		if len(parts) == 2 {
			inputData[parts[0]] = guessType(parts[1])
		}
	}

	if dryRun {
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

	fmt.Printf("使用模型: %s\n提交任务", matched.DisplayName)
	taskID, err := c.SubmitTask(ctx, client.GatewayBase+entry.Endpoint, modelID, inputData)
	if err != nil {
		return fmt.Errorf("❌ 提交失败: %v", err)
	}
	fmt.Println(" - 完成")

	start := time.Now()
	fmt.Print("轮询中")
	_, err = c.PollTask(ctx, taskID, 3, 1200)
	if err != nil {
		return fmt.Errorf("❌ %v", err)
	}
	elapsed := int64(time.Since(start).Seconds())
	fmt.Printf(" - 完成 (%ds)\n", elapsed)

	outDir := outputDir()
	os.MkdirAll(outDir, 0755)
	outFile := filepath.Join(outDir, fmt.Sprintf("result.%s", entry.OutputExt))
	if outputPath != "" {
		outFile = outputPath
	}

	env := output.Envelope{
		OK: true,
		Data: map[string]any{
			"files":    []string{outFile},
			"task_id":  taskID,
			"duration": elapsed,
			"model":    matched.DisplayName,
		},
		Meta: &output.Meta{TaskID: taskID, Duration: elapsed},
	}
	if jsonMode {
		return output.JSON(env)
	}
	fmt.Printf("OUTPUT_FILE: %s\n", outFile)
	return nil
}

func showModelList(candidates []client.GatewayModel) {
	for i, m := range candidates {
		fmt.Printf("  %2d. %-30s %s\n", i+1, m.ID, m.DisplayName)
	}
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
)

var imageCmd = &cobra.Command{
	Use:   "image",
	Short: "图片生成和编辑",
	RunE: func(cmd *cobra.Command, args []string) error {
		p := getPromptFromArgs(args, &imageParamFlags)
		if p != "" {
			imagePromptFlag = p
		}
		return runMedia(cmd, "image", "image-2", imageModelFlag, imagePromptFlag,
			imageInputFiles, imageParamFlags, imageOutPath)
	},
}

// ── Video ──

var (
	videoModelFlag  string
	videoInputFiles []string
	videoParamFlags []string
	videoOutPath    string
	videoPromptFlag string
)

var videoCmd = &cobra.Command{
	Use:   "video",
	Short: "视频生成和编辑",
	RunE: func(cmd *cobra.Command, args []string) error {
		p := getPromptFromArgs(args, &videoParamFlags)
		if p != "" {
			videoPromptFlag = p
		}
		return runMedia(cmd, "video", "seedance2.0", videoModelFlag, videoPromptFlag,
			videoInputFiles, videoParamFlags, videoOutPath)
	},
}

// ── Audio ──

var (
	audioModelFlag  string
	audioParamFlags []string
	audioOutPath    string
	audioPromptFlag string
)

var audioCmd = &cobra.Command{
	Use:   "audio",
	Short: "音频生成",
	RunE: func(cmd *cobra.Command, args []string) error {
		p := getPromptFromArgs(args, &audioParamFlags)
		if p != "" {
			audioPromptFlag = p
		}
		return runMedia(cmd, "audio_edit", "音频智能设计", audioModelFlag, audioPromptFlag,
			nil, audioParamFlags, audioOutPath)
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
	home, _ := os.UserHomeDir()
	return filepath.Join(home, "ly-output")
}

func init() {
	imageCmd.Flags().StringVarP(&imageModelFlag, "model", "m", "", "模型 ID 或名称")
	imageCmd.Flags().StringArrayVarP(&imageInputFiles, "image", "i", nil, "输入图片 (可重复)")
	imageCmd.Flags().StringArrayVar(&imageParamFlags, "param", nil, "附加参数 key=value (可重复)")
	imageCmd.Flags().StringVarP(&imageOutPath, "output", "o", "", "输出文件路径")
	imageCmd.Flags().StringVarP(&imagePromptFlag, "prompt", "p", "", "提示词")

	videoCmd.Flags().StringVarP(&videoModelFlag, "model", "m", "", "模型 ID 或名称")
	videoCmd.Flags().StringArrayVarP(&videoInputFiles, "image", "i", nil, "输入图片/视频 (可重复)")
	videoCmd.Flags().StringArrayVar(&videoParamFlags, "param", nil, "附加参数 key=value (可重复)")
	videoCmd.Flags().StringVarP(&videoOutPath, "output", "o", "", "输出文件路径")
	videoCmd.Flags().StringVarP(&videoPromptFlag, "prompt", "p", "", "提示词")

	audioCmd.Flags().StringVarP(&audioModelFlag, "model", "m", "", "模型 ID 或名称")
	audioCmd.Flags().StringArrayVar(&audioParamFlags, "param", nil, "附加参数 key=value (可重复)")
	audioCmd.Flags().StringVarP(&audioOutPath, "output", "o", "", "输出文件路径")
	audioCmd.Flags().StringVarP(&audioPromptFlag, "prompt", "p", "", "提示词")
}
