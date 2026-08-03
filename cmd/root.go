// Copyright (c) 2026 Lingying. SPDX-License-Identifier: MIT
// Root command and subcommand registration.
package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/Din-Studio/lingying-cli/internal/client"
	"github.com/Din-Studio/lingying-cli/internal/output"
	"github.com/spf13/cobra"
)

var Version = "0.1.0"
var configFile string

var rootCmd = &cobra.Command{
	Use:   "ly",
	Short: "Lingying Gateway CLI — AI 模型网关",
	Long: `ly — Lingying Gateway CLI

  快捷命令:
    ly text       文本对话
    ly image      图片生成
    ly video      视频生成
    ly audio      音频生成

  模型发现:
    ly model list
    ly model info <id>
    ly model search <词>

  鉴权:
    ly auth login          OAuth 登录
    ly auth set-key <key>  API Key
    ly auth show

  使用 ly <命令> --help 查看详细用法。`,
	SilenceUsage:  true,
	SilenceErrors: true,
	Version:       Version,
	PersistentPreRun: func(cmd *cobra.Command, args []string) {
		if configFile != "" {
			_ = os.Setenv("LY_CONFIG_FILE", configFile)
		}
	},
	CompletionOptions: cobra.CompletionOptions{DisableDefaultCmd: true},
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		if output.IsEmittedError(err) {
			os.Exit(1)
		}
		if hasJSONFlag(os.Args[1:]) {
			detail := client.ErrorDetails(err)
			extra := map[string]any{}
			if detail.RequestID != "" {
				extra["request_id"] = detail.RequestID
			}
			if jsonErr := output.JSON(output.Failure(detail.Code, detail.Message, extra)); jsonErr != nil && !output.IsEmittedError(jsonErr) {
				fmt.Fprintln(os.Stderr, "Error:", jsonErr)
			}
			os.Exit(1)
		}
		fmt.Fprintln(os.Stderr, "Error:", err)
		os.Exit(1)
	}
}

func hasJSONFlag(args []string) bool {
	for _, arg := range args {
		if arg == "--json" || strings.HasPrefix(arg, "--json=") {
			return true
		}
	}
	return false
}

func init() {
	rootCmd.PersistentFlags().Bool("json", false, "JSON 输出")
	rootCmd.PersistentFlags().Bool("dry-run", false, "预览不执行")
	rootCmd.PersistentFlags().BoolP("verbose", "v", false, "详细输出")
	rootCmd.PersistentFlags().StringVar(&configFile, "config", "", "配置文件路径（覆盖默认位置）")

	rootCmd.AddCommand(checkCmd)
	rootCmd.AddCommand(authCmd)
	rootCmd.AddCommand(textCmd)
	rootCmd.AddCommand(imageCmd)
	rootCmd.AddCommand(videoCmd)
	rootCmd.AddCommand(audioCmd)
	rootCmd.AddCommand(modelCmd)

	authCmd.AddCommand(authLoginCmd)
	authCmd.AddCommand(authSetKeyCmd)
	authCmd.AddCommand(authShowCmd)
	authCmd.AddCommand(authPathCmd)
	authCmd.AddCommand(authLogoutCmd)
	authCmd.AddCommand(authSetOutputDirCmd)

	modelCmd.AddCommand(modelListCmd)
	modelCmd.AddCommand(modelInfoCmd)
	modelCmd.AddCommand(modelSearchCmd)
}

// helpers

func isJSON(cmd *cobra.Command) bool {
	v, _ := cmd.Flags().GetBool("json")
	return v
}

func isDryRun(cmd *cobra.Command) bool {
	v, _ := cmd.Flags().GetBool("dry-run")
	return v
}
