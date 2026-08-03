// Copyright (c) 2026 Lingying. SPDX-License-Identifier: MIT
// ly auth — OAuth login / API key / show.
package cmd

import (
	"fmt"

	"github.com/Din-Studio/lingying-cli/internal/auth"
	"github.com/Din-Studio/lingying-cli/internal/output"
	"github.com/spf13/cobra"
)

var authCmd = &cobra.Command{
	Use:   "auth",
	Short: "管理鉴权",
	Long:  "OAuth 登录、设置 API Key、查看当前鉴权状态。",
}

// authFactory resolves credentials and handles no-auth errors uniformly.
// Each RunE that needs auth calls this first.
func authFactory(cmd *cobra.Command) auth.Resolved {
	r := auth.Resolve()
	if r.Value == "" {
		return r
	}
	return r
}

var authLoginCmd = &cobra.Command{
	Use:   "login",
	Short: "OAuth 浏览器登录",
	Long: `打开浏览器完成 Lingying 登录授权，自动获取 Access Token。

  ly auth login`,
	RunE: func(cmd *cobra.Command, args []string) error {
		jsonMode := isJSON(cmd)
		if jsonMode {
			return commandFailure(true, "interactive_auth_required", "ly auth login 需要交互式终端；自动化场景请设置 LY_ACCESS_TOKEN", nil)
		}

		// OAuth Device Flow is not available yet; prompt for a pasted token.
		fmt.Print("请在浏览器中打开 https://console.echojoy.cn/console/login 完成登录。\n")
		fmt.Print("登录后将控制台中的 Access Token 粘贴到此处: ")

		token, err := readInteractivePrompt(cmd.InOrStdin())
		if err != nil {
			return commandFailure(false, "input_required", "未输入 Access Token", nil)
		}

		if err := auth.StoreOAuthToken(token, ""); err != nil {
			return err
		}
		env := output.Envelope{OK: true, Data: map[string]any{
			"message": "登录成功，Token 已保存",
		}}
		if jsonMode {
			return output.JSON(env)
		}
		fmt.Println("✅ 登录成功，Token 已保存")
		return nil
	},
}

var authSetKeyCmd = &cobra.Command{
	Use:   "set-key <key>",
	Short: "设置 API Key",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		jsonMode := isJSON(cmd)
		key := args[0]
		if err := auth.StoreAPIKey(key); err != nil {
			return err
		}
		env := output.Envelope{OK: true, Data: map[string]any{
			"message": "API Key 已保存",
		}}
		if jsonMode {
			return output.JSON(env)
		}
		fmt.Println("✅ API Key 已保存")
		return nil
	},
}

var authShowCmd = &cobra.Command{
	Use:   "show",
	Short: "查看当前鉴权",
	RunE: func(cmd *cobra.Command, args []string) error {
		jsonMode := isJSON(cmd)
		r := auth.Resolve()
		if r.Value == "" {
			return commandFailure(jsonMode, "no_auth", "未配置鉴权", nil)
		}
		env := output.Envelope{OK: true, Data: map[string]any{
			"auth_type":   r.Type,
			"source":      r.Source,
			"has_upload":  r.HasUpload(),
			"config_path": auth.ConfigPath(),
		}}
		if jsonMode {
			return output.JSON(env)
		}
		fmt.Printf("鉴权类型: %s\n来源: %s\n文件上传: %v\n配置文件: %s\n", r.Display(), r.Source, r.HasUpload(), auth.ConfigPath())
		return nil
	},
}

var authPathCmd = &cobra.Command{
	Use:   "path",
	Short: "显示配置文件路径",
	RunE: func(cmd *cobra.Command, args []string) error {
		if isJSON(cmd) {
			return output.JSON(output.Envelope{OK: true, Data: map[string]any{"config_path": auth.ConfigPath()}})
		}
		fmt.Println(auth.ConfigPath())
		return nil
	},
}

var authLogoutCmd = &cobra.Command{
	Use:   "logout",
	Short: "清除本地保存的凭据",
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := auth.ClearCredentials(); err != nil {
			return err
		}
		env := output.Envelope{OK: true, Data: map[string]any{"message": "本地凭据已清除"}}
		if isJSON(cmd) {
			return output.JSON(env)
		}
		fmt.Println("✅ 本地凭据已清除")
		return nil
	},
}

var authSetOutputDirCmd = &cobra.Command{
	Use:   "set-output-dir <path>",
	Short: "设置默认输出目录",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := auth.StoreOutputDir(args[0]); err != nil {
			return err
		}
		env := output.Envelope{OK: true, Data: map[string]any{"output_dir": args[0]}}
		if isJSON(cmd) {
			return output.JSON(env)
		}
		fmt.Printf("✅ 默认输出目录已设置为 %s\n", args[0])
		return nil
	},
}
