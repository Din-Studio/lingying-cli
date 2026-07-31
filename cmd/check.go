// Copyright (c) 2026 Lingying. SPDX-License-Identifier: MIT
// ly check — verify auth and connectivity.
package cmd

import (
	"context"
	"fmt"

	"github.com/Din-Studio/lingying-cli/internal/client"
	"github.com/Din-Studio/lingying-cli/internal/output"
	"github.com/spf13/cobra"
)

var checkCmd = &cobra.Command{
	Use:   "check",
	Short: "检查连通性和可用模型",
	RunE: func(cmd *cobra.Command, args []string) error {
		jsonMode := isJSON(cmd)
		resolved := authFactory(cmd)
		if resolved.Value == "" {
			env := output.Envelope{
				OK: false,
				Data: map[string]any{
					"status":  "no_auth",
					"message": "未配置鉴权，请先 ly auth login 或 ly auth set-key",
				},
			}
			if jsonMode {
				return output.JSON(env)
			}
			fmt.Println("❌ 未配置鉴权。请先 ly auth login 或 ly auth set-key")
			return nil
		}

		c := client.New(resolved.Value)
		models, err := c.ListModels(context.Background())
		if err != nil {
			env := output.Envelope{
				OK: false,
				Data: map[string]any{
					"status":  "error",
					"message": err.Error(),
				},
			}
			if jsonMode {
				return output.JSON(env)
			}
			fmt.Printf("❌ 连接失败: %v\n", err)
			return nil
		}

		typeCounts := make(map[string]int)
		for _, m := range models {
			typeCounts[m.ModelType]++
		}

		env := output.Envelope{
			OK: true,
			Data: map[string]any{
				"status":    "ready",
				"auth_type": resolved.Type,
				"models":    typeCounts,
			},
		}
		if jsonMode {
			return output.JSON(env)
		}
		fmt.Printf("✅ Lingying Gateway 已就绪\n   鉴权: %s\n   模型: ", resolved.Display())
		first := true
		for t, n := range typeCounts {
			if !first {
				fmt.Print(", ")
			}
			fmt.Printf("%s (%d)", t, n)
			first = false
		}
		fmt.Println()
		return nil
	},
}
