// Copyright (c) 2026 Lingying. SPDX-License-Identifier: MIT
// Updater — 候选下载源与 Release 资产 URL 拼装。
package updater

import (
	"fmt"
	"os"
	"strings"
)

// GitHubBase 是所有下载地址的根。公共代理的用法是把完整的 GitHub URL 接在自己
// 后面，因此代理源的 Base 形如 "https://ghfast.top/https://github.com"。
const GitHubBase = "https://github.com"

// Source 是一个候选下载来源。
// Trusted 标记该来源是否可信——只有可信来源取回的校验和才能防篡改。
type Source struct {
	Name    string
	Base    string
	Trusted bool
}

// Sources 按优先级返回候选源：直连优先，其后是 LY_MIRROR 指定的镜像，
// 最后是内置公共代理。公共代理只作回退，随时可能失效。
func Sources() []Source {
	list := []Source{{Name: "GitHub 直连", Base: GitHubBase, Trusted: true}}
	if mirror := strings.TrimRight(os.Getenv("LY_MIRROR"), "/"); mirror != "" {
		list = append(list, Source{Name: "LY_MIRROR", Base: mirror + "/" + GitHubBase})
	}
	return append(list,
		Source{Name: "ghfast.top", Base: "https://ghfast.top/" + GitHubBase},
		Source{Name: "gh-proxy.com", Base: "https://gh-proxy.com/" + GitHubBase},
	)
}

// LatestTagURL 指向 releases/latest。该地址返回 302，Location 形如
// .../releases/tag/v0.1.5——一次请求即可拿到版本号，无需 api.github.com。
func (s Source) LatestTagURL(repo string) string {
	return fmt.Sprintf("%s/%s/releases/latest", s.Base, repo)
}

// AssetURL 拼出某个 Release 资产的下载地址。version 为空表示取最新。
func (s Source) AssetURL(repo, version, asset string) string {
	if version == "" {
		return fmt.Sprintf("%s/%s/releases/latest/download/%s", s.Base, repo, asset)
	}
	return fmt.Sprintf("%s/%s/releases/download/v%s/%s", s.Base, repo, strings.TrimPrefix(version, "v"), asset)
}
