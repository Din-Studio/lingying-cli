// Copyright (c) 2026 Lingying. SPDX-License-Identifier: MIT
// ly text — chat with text models.
package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/Din-Studio/lingying-cli/internal/auth"
	"github.com/Din-Studio/lingying-cli/internal/client"
	"github.com/Din-Studio/lingying-cli/internal/output"
	"github.com/spf13/cobra"
)

var (
	textModel      string
	textMaxTokens  int
	textParamFlags []string
)

var textCmd = &cobra.Command{
	Use:   "text [prompt]",
	Short: "文本对话",
	Long: `与 AI 文本模型对话。

  ly text "解释量子计算"
  ly text --model <model-id> "写一段 Go 代码"
  ly text --max-tokens 4096 "写一篇文章"
  ly text --param temperature=0.7 "写一个更有创意的标题"`,
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
		// Discover the model to get api_format
		c := client.New(resolved.Value)
		ctx := context.Background()
		allModels, err := c.ListModels(ctx)
		if err != nil {
			return err
		}

		matched, err := selectTextModel(allModels, modelID)
		if err != nil {
			return err
		}
		modelID = matched.ID

		// Get prompt
		prompt := strings.Join(args, " ")

		if prompt == "" {
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

		messages := []client.ChatMessage{{Role: "user", Content: prompt}}
		params, err := buildTextParams(textParamFlags, matched.InputSchema, textMaxTokens)
		if err != nil {
			return err
		}

		if dryRun {
			request := map[string]any{"model": modelID, "messages": messages}
			for key, value := range params {
				request[key] = value
			}
			env := output.Envelope{OK: true, Data: map[string]any{
				"model":   modelID,
				"request": request,
				"dry_run": true,
			}}
			if jsonMode {
				return output.JSON(env)
			}
			fmt.Printf("[dry-run] model=%s request=%v\n", modelID, request)
			return nil
		}

		reply, err := c.Chat(ctx, modelID, matched.APIFormat, messages, params)
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
	textCmd.Flags().IntVar(&textMaxTokens, "max-tokens", 0, "最大输出 token 数")
	textCmd.Flags().StringArrayVar(&textParamFlags, "param", nil, "附加模型参数 key=value（可重复）")
}

// buildTextParams encodes dynamic top-level schema fields. --max-tokens is a
// backwards-compatible shortcut and takes precedence when explicitly set.
func buildTextParams(flags []string, schema json.RawMessage, maxTokens int) (map[string]any, error) {
	params := map[string]any{}
	for _, flag := range flags {
		parts := strings.SplitN(flag, "=", 2)
		if len(parts) != 2 || parts[0] == "" {
			return nil, fmt.Errorf("非法 --param %q，应为 key=value", flag)
		}
		if parts[0] == "messages" || parts[0] == "model" {
			return nil, fmt.Errorf("--param %q 由 text 命令自身生成，不能覆盖", parts[0])
		}
		if err := applySchemaParam(params, parts[0], parts[1], schema); err != nil {
			return nil, fmt.Errorf("非法 --param %q: %w", flag, err)
		}
	}
	if maxTokens > 0 {
		params["max_tokens"] = maxTokens
	} else if _, exists := params["max_tokens"]; !exists {
		// Anthropic schemas require this field but do not publish a default.
		params["max_tokens"] = 4096
	}
	return params, nil
}

func selectTextModel(models []client.GatewayModel, requested string) (*client.GatewayModel, error) {
	if requested == "" {
		return selectDynamicModel(models, map[string]bool{"text": true})
	}
	for i := range models {
		m := &models[i]
		if m.ID == requested || strings.EqualFold(m.DisplayName, requested) {
			if m.ModelType != "text" {
				return nil, fmt.Errorf("模型 %s 的类型 %s 不适用于文本命令", m.ID, m.ModelType)
			}
			return m, nil
		}
	}
	return nil, fmt.Errorf("未找到文本模型: %s", requested)
}
