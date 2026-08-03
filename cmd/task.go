package cmd

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/Din-Studio/lingying-cli/internal/auth"
	"github.com/Din-Studio/lingying-cli/internal/client"
	"github.com/Din-Studio/lingying-cli/internal/output"
	"github.com/spf13/cobra"
)

var taskCmd = &cobra.Command{
	Use:   "task",
	Short: "查询异步任务",
}

var taskGetCmd = &cobra.Command{
	Use:   "get <task-id>",
	Short: "查询任务状态和结果",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		jsonMode := isJSON(cmd)
		resolved := auth.Resolve()
		if resolved.Value == "" {
			env := output.Failure("no_auth", "未配置鉴权，请先 ly auth login 或 ly auth set-key", nil)
			if jsonMode {
				return output.JSON(env)
			}
			fmt.Println("❌ 未配置鉴权。请先 ly auth login 或 ly auth set-key")
			return nil
		}

		body, err := client.New(resolved.Value).GetTask(context.Background(), args[0])
		if err != nil {
			return err
		}
		var response map[string]any
		if err := json.Unmarshal(body, &response); err != nil {
			return fmt.Errorf("解析任务响应失败: %w", err)
		}
		data := response["data"]
		if data == nil {
			data = response
		}
		if jsonMode {
			return output.JSON(output.Envelope{OK: true, Data: data, Meta: &output.Meta{TaskID: args[0]}})
		}
		pretty, _ := json.MarshalIndent(data, "", "  ")
		fmt.Println(string(pretty))
		return nil
	},
}

func init() {
	taskCmd.AddCommand(taskGetCmd)
	rootCmd.AddCommand(taskCmd)
}
