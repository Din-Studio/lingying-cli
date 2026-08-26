// Copyright (c) 2026 Lingying. SPDX-License-Identifier: MIT
// ly update — replace this binary with the latest release.
package cmd

import (
	"context"
	"fmt"

	"github.com/Din-Studio/lingying-cli/internal/output"
	"github.com/Din-Studio/lingying-cli/internal/updater"
	"github.com/spf13/cobra"
)

var updateCheckOnly bool

var updateCmd = &cobra.Command{
	Use:   "update",
	Short: "更新 ly 到最新版本",
	Long: `ly update — 更新到最新版本

从 GitHub Releases 下载当前平台的发布包，校验 SHA-256 后替换正在运行的
可执行文件。无论 ly 是通过安装脚本、npm 还是 go install 安装的，更新方式
都相同。

  ly update            更新到最新版本
  ly update --check    只检查是否有新版本，不做任何改动`,
	RunE: func(cmd *cobra.Command, args []string) error {
		jsonMode := isJSON(cmd)

		up, err := updater.New(Version)
		if err != nil {
			return commandFailure(jsonMode, "update_failed", err.Error(), nil)
		}

		ctx := context.Background()
		latest, err := up.LatestVersion(ctx)
		if err != nil {
			return commandFailure(jsonMode, "update_check_failed", err.Error(), nil)
		}

		if !up.NeedsUpdate(latest) {
			return reportUpdateStatus(jsonMode, "up_to_date", up.Current, latest,
				fmt.Sprintf("✅ 已是最新版本 %s", latest))
		}

		if updateCheckOnly || isDryRun(cmd) {
			return reportUpdateStatus(jsonMode, "update_available", up.Current, latest,
				fmt.Sprintf("发现新版本 %s（当前 %s）\n   执行 ly update 更新", latest, up.Current))
		}

		if !jsonMode {
			fmt.Printf("下载 ly %s ...\n", latest)
		}
		if err := up.Apply(ctx, latest); err != nil {
			return commandFailure(jsonMode, "update_failed", err.Error(), nil)
		}
		return reportUpdateStatus(jsonMode, "updated", up.Current, latest,
			fmt.Sprintf("✅ 已更新到 %s", latest))
	},
}

func reportUpdateStatus(jsonMode bool, status, current, latest, message string) error {
	if jsonMode {
		return output.JSON(output.Envelope{OK: true, Data: map[string]any{
			"status":  status,
			"current": current,
			"latest":  latest,
		}})
	}
	fmt.Println(message)
	return nil
}

func init() {
	updateCmd.Flags().BoolVar(&updateCheckOnly, "check", false, "只检查新版本，不更新")
}
