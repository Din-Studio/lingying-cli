// Copyright (c) 2026 Lingying. SPDX-License-Identifier: MIT
// ly model list / info / search — model discovery.
package cmd

import (
	"context"
	"fmt"
	"strings"

	"github.com/Din-Studio/lingying-cli/internal/auth"
	"github.com/Din-Studio/lingying-cli/internal/client"
	"github.com/Din-Studio/lingying-cli/internal/output"
	"github.com/Din-Studio/lingying-cli/internal/registry"
	"github.com/spf13/cobra"
)

var (
	modelListType string
	modelListSize int
)

var modelCmd = &cobra.Command{
	Use:   "model",
	Short: "模型发现",
	Long:  "浏览、搜索和查看可用 AI 模型。",
}

var modelListCmd = &cobra.Command{
	Use:   "list",
	Short: "列出可用模型",
	RunE: func(cmd *cobra.Command, args []string) error {
		jsonMode := isJSON(cmd)
		resolved := auth.Resolve()
		if resolved.Value == "" {
			return commandFailure(jsonMode, "no_auth", "未配置鉴权，请先 ly auth login 或 ly auth set-key", nil)
		}

		c := client.New(resolved.Value).WithProjectID(resolveProjectID(cmd))
		models, err := c.ListModels(context.Background())
		if err != nil {
			return err
		}

		// Filter by type
		var filtered []client.GatewayModel
		for _, m := range models {
			if modelListType == "" || m.ModelType == modelListType {
				filtered = append(filtered, m)
			}
		}
		filtered = limitModels(filtered, modelListSize)

		if jsonMode {
			return output.JSON(output.Envelope{
				OK: true,
				Data: map[string]any{
					"models": filtered,
					"count":  len(filtered),
				},
			})
		}

		// Group by model_type for human display
		groups := make(map[string][]client.GatewayModel)
		for _, m := range filtered {
			groups[m.ModelType] = append(groups[m.ModelType], m)
		}
		for _, t := range registry.AllTypes() {
			list, ok := groups[t]
			if !ok {
				continue
			}
			fmt.Printf("\n%s (%d)\n", t, len(list))
			for _, m := range list {
				entry, _ := registry.Lookup(t)
				mode := entry.Mode
				fmt.Printf("  %-30s  %s (%s)\n", m.ID, m.DisplayName, mode)
			}
		}
		fmt.Println()
		return nil
	},
}

var modelInfoCmd = &cobra.Command{
	Use:   "info <id>",
	Short: "查看模型详情",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		jsonMode := isJSON(cmd)
		resolved := auth.Resolve()
		if resolved.Value == "" {
			return commandFailure(jsonMode, "no_auth", "未配置鉴权，请先 ly auth login 或 ly auth set-key", nil)
		}

		query := args[0]
		c := client.New(resolved.Value).WithProjectID(resolveProjectID(cmd))
		models, err := c.ListModels(context.Background())
		if err != nil {
			return err
		}

		matched := selectModelInfo(models, query)

		if matched == nil {
			return commandFailure(jsonMode, "model_not_found", fmt.Sprintf("未找到模型: %s", query), nil)
		}

		entry, _ := registry.Lookup(matched.ModelType)

		if jsonMode {
			return output.JSON(output.Envelope{
				OK: true,
				Data: map[string]any{
					"id":            matched.ID,
					"model_id":      matched.ModelID,
					"display_name":  matched.DisplayName,
					"model_type":    matched.ModelType,
					"api_format":    matched.APIFormat,
					"mode":          entry.Mode,
					"endpoint":      entry.Endpoint,
					"output_type":   entry.OutputType,
					"feature_types": matched.FeatureTypes,
					"input_schema":  matched.InputSchema,
				},
			})
		}

		fmt.Printf("ID:           %s\n", matched.ID)
		fmt.Printf("Model ID:     %s\n", matched.ModelID)
		fmt.Printf("Display Name: %s\n", matched.DisplayName)
		fmt.Printf("Type:         %s\n", matched.ModelType)
		fmt.Printf("Mode:         %s\n", entry.Mode)
		fmt.Printf("Endpoint:     %s\n", entry.Endpoint)
		if matched.APIFormat != "" {
			fmt.Printf("API Format:    %s\n", matched.APIFormat)
		}
		fmt.Printf("Output:       %s (.%s)\n", entry.OutputType, entry.OutputExt)
		fmt.Printf("\nInput Schema:\n%s\n", string(matched.InputSchema))
		return nil
	},
}

// selectModelInfo preserves convenient partial lookup while ensuring that a
// model ID or display name supplied exactly returns that model's schema.
func selectModelInfo(models []client.GatewayModel, query string) *client.GatewayModel {
	for i := range models {
		model := &models[i]
		if model.ID == query || strings.EqualFold(model.DisplayName, query) {
			return model
		}
	}
	needle := strings.ToLower(query)
	for i := range models {
		model := &models[i]
		if strings.Contains(strings.ToLower(model.DisplayName), needle) ||
			strings.Contains(strings.ToLower(model.ID), needle) {
			return model
		}
	}
	return nil
}

func limitModels(models []client.GatewayModel, size int) []client.GatewayModel {
	if size > 0 && len(models) > size {
		return models[:size]
	}
	return models
}

var modelSearchCmd = &cobra.Command{
	Use:   "search <keyword>",
	Short: "搜索模型",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		jsonMode := isJSON(cmd)
		resolved := auth.Resolve()
		if resolved.Value == "" {
			return commandFailure(jsonMode, "no_auth", "未配置鉴权，请先 ly auth login 或 ly auth set-key", nil)
		}

		keyword := strings.ToLower(args[0])
		c := client.New(resolved.Value).WithProjectID(resolveProjectID(cmd))
		models, err := c.ListModels(context.Background())
		if err != nil {
			return err
		}

		var results []client.GatewayModel
		for _, m := range models {
			if strings.Contains(strings.ToLower(m.DisplayName), keyword) ||
				strings.Contains(strings.ToLower(m.ID), keyword) ||
				strings.Contains(strings.ToLower(m.ModelType), keyword) {
				results = append(results, m)
			}
		}

		if jsonMode {
			return output.JSON(output.Envelope{
				OK:   true,
				Data: map[string]any{"results": results, "count": len(results)},
				Meta: &output.Meta{Count: len(results)},
			})
		}

		if len(results) == 0 {
			fmt.Printf("未找到匹配 '%s' 的模型\n", keyword)
			return nil
		}
		fmt.Printf("找到 %d 个匹配 '%s' 的模型:\n\n", len(results), keyword)
		for _, m := range results {
			entry, _ := registry.Lookup(m.ModelType)
			fmt.Printf("  %-30s  %s  (%s)\n", m.ID, m.DisplayName, entry.Mode)
		}
		return nil
	},
}

func init() {
	modelListCmd.Flags().StringVar(&modelListType, "type", "", "按模型类型过滤 (text/image/video/audio_edit)")
	modelListCmd.Flags().IntVar(&modelListSize, "size", 0, "限制显示数量")
}
