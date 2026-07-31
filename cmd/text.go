// Copyright (c) 2026 Lingying. SPDX-License-Identifier: MIT
// ly text — chat with text models.
package cmd

import (
	"context"
	"fmt"
	"strings"

	"github.com/Din-Studio/lingying-cli/internal/auth"
	"github.com/Din-Studio/lingying-cli/internal/client"
	"github.com/Din-Studio/lingying-cli/internal/output"
	"github.com/spf13/cobra"
)

var (
	textModel    string
	textFile     string
	textMaxTokens int
)

var textCmd = &cobra.Command{
	Use:   "text [prompt]",
	Short: "文本对话",
	Long: `与 AI 文本模型对话。

  ly text "解释量子计算"
  ly text --model claude-sonnet-4.6 "写一段 Go 代码"
  ly text --model deepseek-v4-pro --file ./doc.pdf "总结这个文档"`,
	Args: cobra.MinimumNArgs(0),
	RunE: func(cmd *cobra.Command, args []string) error {
		jsonMode := isJSON(cmd)
		dryRun := isDryRun(cmd)

		// Resolve auth
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

		// Pick model
		modelID := textModel
		if modelID == "" {
			modelID = auth.GetDefaultModel()
		}
		if modelID == "" {
			modelID = "claude-sonnet-4.6" // reasonable default
		}

		// Discover the model to get api_format
		c := client.New(resolved.Value)
		ctx := context.Background()
		allModels, err := c.ListModels(ctx)
		if err != nil {
			return err
		}

		var matched *client.GatewayModel
		for i := range allModels {
			if strings.EqualFold(allModels[i].ID, modelID) ||
				strings.Contains(strings.ToLower(allModels[i].DisplayName), strings.ToLower(modelID)) ||
				strings.Contains(strings.ToLower(allModels[i].ID), strings.ToLower(modelID)) {
				matched = &allModels[i]
				modelID = allModels[i].ID // use canonical ID
				break
			}
		}

		// Get prompt
		prompt := strings.Join(args, " ")

		if prompt == "" && textFile == "" {
			// Interactive: ask for prompt
			fmt.Printf("使用模型: %s\n", modelID)
			fmt.Print("输入: ")
			fmt.Scanln(&prompt)
			if prompt == "" {
				env := output.Envelope{OK: false, Data: map[string]any{
					"message": "未输入 prompt",
				}}
				if jsonMode {
					return output.JSON(env)
				}
				return nil
			}
		}

		if dryRun {
			env := output.Envelope{OK: true, Data: map[string]any{
				"model":  modelID,
				"prompt": prompt,
				"file":   textFile,
				"dry_run": true,
			}}
			if jsonMode {
				return output.JSON(env)
			}
			fmt.Printf("[dry-run] model=%s prompt=%s\n", modelID, prompt)
			return nil
		}

		apiFormat := ""
		if matched != nil {
			apiFormat = matched.APIFormat
		}

		messages := []client.ChatMessage{{Role: "user", Content: prompt}}

		// Default max_tokens to avoid validation errors
		mt := textMaxTokens
		if mt == 0 {
			mt = 4096
		}

		reply, err := c.Chat(ctx, modelID, apiFormat, messages, mt)
		if err != nil {
			env := output.Envelope{OK: false, Data: map[string]any{"message": err.Error()}}
			if jsonMode {
				return output.JSON(env)
			}
			fmt.Printf("❌ %v\n", err)
			return nil
		}

		env := output.Envelope{OK: true, Data: map[string]any{
			"model": modelID,
			"reply": reply,
		}}
		if jsonMode {
			return output.JSON(env)
		}
		fmt.Printf("使用模型: %s\n\n%s\n", modelID, reply)
		return nil
	},
}

func init() {
	textCmd.Flags().StringVarP(&textModel, "model", "m", "", "模型 ID 或 display_name")
	textCmd.Flags().StringVarP(&textFile, "file", "f", "", "附加文件路径")
	textCmd.Flags().IntVar(&textMaxTokens, "max-tokens", 0, "最大输出 token 数")
}
