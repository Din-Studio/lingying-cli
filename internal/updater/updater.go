// Copyright (c) 2026 Lingying. SPDX-License-Identifier: MIT
// Self-update: resolve the latest release, download it, replace this binary.
//
// The update path is deliberately identical for every install method. Whether
// ly arrived via install.sh, npm or go install, we download the GitHub Release
// archive for this platform and overwrite the running executable in place.
package updater

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
)

const (
	defaultManifestURL = "https://raw.githubusercontent.com/Din-Studio/lingying-cli/main/scripts/version.json"
	defaultReleaseAPI  = "https://api.github.com/repos/Din-Studio/lingying-cli/releases/latest"
	defaultReleaseBase = "https://github.com/Din-Studio/lingying-cli/releases/download"

	// devVersion marks a binary built without the release ldflags. It carries
	// no comparable version, so any published release counts as newer.
	devVersion = "dev"
)

// Updater holds everything the update flow needs. The URL and platform fields
// are configurable so tests can point at a local server.
type Updater struct {
	Current     string
	ExecPath    string
	HTTP        *http.Client
	ManifestURL string
	ReleaseAPI  string
	ReleaseBase string
	GOOS        string
	GOARCH      string
}

// New resolves the running executable — following symlinks so that a
// ~/.local/bin/ly symlink updates the real file, not the link.
func New(current string) (*Updater, error) {
	exe, err := os.Executable()
	if err != nil {
		return nil, fmt.Errorf("无法定位当前可执行文件: %w", err)
	}
	if resolved, err := filepath.EvalSymlinks(exe); err == nil {
		exe = resolved
	}
	return &Updater{
		Current:     current,
		ExecPath:    exe,
		HTTP:        &http.Client{Timeout: 120 * time.Second},
		ManifestURL: defaultManifestURL,
		ReleaseAPI:  defaultReleaseAPI,
		ReleaseBase: defaultReleaseBase,
		GOOS:        runtime.GOOS,
		GOARCH:      runtime.GOARCH,
	}, nil
}

// LatestVersion prefers scripts/version.json — the same manifest install.sh
// reads, so both paths agree on what "latest" means — and falls back to the
// GitHub Releases API when the manifest is unreachable or malformed.
func (u *Updater) LatestVersion(ctx context.Context) (string, error) {
	if version, err := u.versionFromManifest(ctx); err == nil {
		return version, nil
	}
	version, err := u.versionFromReleaseAPI(ctx)
	if err != nil {
		return "", fmt.Errorf("无法获取最新版本: %w", err)
	}
	return version, nil
}

func (u *Updater) versionFromManifest(ctx context.Context) (string, error) {
	body, err := u.fetch(ctx, u.ManifestURL)
	if err != nil {
		return "", err
	}
	var manifest struct {
		Version string `json:"version"`
	}
	if err := json.Unmarshal(body, &manifest); err != nil {
		return "", err
	}
	return validVersion(manifest.Version)
}

func (u *Updater) versionFromReleaseAPI(ctx context.Context) (string, error) {
	body, err := u.fetch(ctx, u.ReleaseAPI)
	if err != nil {
		return "", err
	}
	var release struct {
		TagName string `json:"tag_name"`
	}
	if err := json.Unmarshal(body, &release); err != nil {
		return "", err
	}
	return validVersion(release.TagName)
}

// NeedsUpdate reports whether latest is worth installing over the running
// build. A dev build has no comparable version, so it always updates.
func (u *Updater) NeedsUpdate(latest string) bool {
	current, err := validVersion(u.Current)
	if err != nil {
		return true
	}
	return compareVersions(latest, current) > 0
}

// Apply downloads the release archive for `version`, verifies its SHA-256
// against checksums.txt, and replaces the running executable with it.
func (u *Updater) Apply(ctx context.Context, version string) error {
	version = strings.TrimPrefix(version, "v")
	dir := filepath.Dir(u.ExecPath)

	// Creating the staging file doubles as the writability probe: if we can't
	// write next to the executable, we can't atomically replace it either.
	staged, err := os.CreateTemp(dir, ".ly-update-*")
	if err != nil {
		return fmt.Errorf("无法写入 %s: %w", dir, err)
	}
	stagedPath := staged.Name()
	staged.Close()
	defer os.Remove(stagedPath)

	asset := u.assetName(version)
	base := fmt.Sprintf("%s/v%s", u.ReleaseBase, version)

	sums, err := u.fetch(ctx, base+"/checksums.txt")
	if err != nil {
		return fmt.Errorf("下载校验和失败: %w", err)
	}
	expected, err := expectedChecksum(sums, asset)
	if err != nil {
		return err
	}

	archive, err := u.fetch(ctx, base+"/"+asset)
	if err != nil {
		return fmt.Errorf("下载 %s 失败: %w", asset, err)
	}
	actual := sha256.Sum256(archive)
	if hex.EncodeToString(actual[:]) != expected {
		return fmt.Errorf("校验和不匹配，已中止更新\n  期望: %s\n  实际: %s", expected, hex.EncodeToString(actual[:]))
	}

	if err := extractBinary(archive, asset, u.binaryName(), stagedPath); err != nil {
		return err
	}
	if err := os.Chmod(stagedPath, 0o755); err != nil {
		return err
	}
	return replaceExecutable(stagedPath, u.ExecPath, u.GOOS == "windows")
}

func (u *Updater) assetName(version string) string {
	ext := ".tar.gz"
	if u.GOOS == "windows" {
		ext = ".zip"
	}
	return fmt.Sprintf("ly-%s-%s-%s%s", version, u.GOOS, u.GOARCH, ext)
}

func (u *Updater) binaryName() string {
	if u.GOOS == "windows" {
		return "ly.exe"
	}
	return "ly"
}

func (u *Updater) fetch(ctx context.Context, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "ly-cli-updater")
	resp, err := u.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, url)
	}
	return io.ReadAll(resp.Body)
}

// expectedChecksum pulls the hash for `asset` out of goreleaser's
// checksums.txt, whose lines are "<sha256>  <filename>".
func expectedChecksum(sums []byte, asset string) (string, error) {
	for _, line := range strings.Split(string(sums), "\n") {
		fields := strings.Fields(line)
		if len(fields) != 2 || fields[1] != asset {
			continue
		}
		sum := strings.ToLower(fields[0])
		if len(sum) != 64 {
			return "", fmt.Errorf("checksums.txt 中 %s 的校验和格式无效: %q", asset, fields[0])
		}
		if _, err := hex.DecodeString(sum); err != nil {
			return "", fmt.Errorf("checksums.txt 中 %s 的校验和格式无效: %q", asset, fields[0])
		}
		return sum, nil
	}
	return "", fmt.Errorf("checksums.txt 中未找到 %s", asset)
}

// extractBinary writes the single archive entry named `binary` to dest.
// Archive paths are only ever compared, never used as an output path, so a
// crafted entry name cannot escape the destination.
func extractBinary(archive []byte, asset, binary, dest string) error {
	if strings.HasSuffix(asset, ".zip") {
		return extractFromZip(archive, binary, dest)
	}
	return extractFromTarGz(archive, binary, dest)
}

func extractFromTarGz(archive []byte, binary, dest string) error {
	gz, err := gzip.NewReader(bytes.NewReader(archive))
	if err != nil {
		return fmt.Errorf("解压归档失败: %w", err)
	}
	defer gz.Close()

	reader := tar.NewReader(gz)
	for {
		header, err := reader.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return fmt.Errorf("读取归档失败: %w", err)
		}
		if header.Typeflag != tar.TypeReg || filepath.Base(header.Name) != binary {
			continue
		}
		return writeFile(dest, reader)
	}
	return fmt.Errorf("归档中未找到 %s", binary)
}

func extractFromZip(archive []byte, binary, dest string) error {
	reader, err := zip.NewReader(bytes.NewReader(archive), int64(len(archive)))
	if err != nil {
		return fmt.Errorf("解压归档失败: %w", err)
	}
	for _, entry := range reader.File {
		if entry.FileInfo().IsDir() || filepath.Base(entry.Name) != binary {
			continue
		}
		src, err := entry.Open()
		if err != nil {
			return fmt.Errorf("读取归档失败: %w", err)
		}
		defer src.Close()
		return writeFile(dest, src)
	}
	return fmt.Errorf("归档中未找到 %s", binary)
}

func writeFile(dest string, src io.Reader) error {
	out, err := os.OpenFile(dest, os.O_WRONLY|os.O_TRUNC, 0o755)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, src)
	return err
}

// replaceExecutable swaps staged into place. Windows refuses to overwrite a
// running executable but does allow renaming it, so the old binary is moved
// aside first and cleaned up on a later run.
func replaceExecutable(staged, target string, windows bool) error {
	if !windows {
		return os.Rename(staged, target)
	}
	backup := target + ".old"
	_ = os.Remove(backup)
	if err := os.Rename(target, backup); err != nil {
		return err
	}
	if err := os.Rename(staged, target); err != nil {
		_ = os.Rename(backup, target)
		return err
	}
	_ = os.Remove(backup)
	return nil
}

// validVersion accepts dotted numeric versions with an optional "v" prefix and
// an optional pre-release suffix ("v1.2.3", "1.2.3-rc1"), returning the
// normalized "1.2.3" form.
func validVersion(raw string) (string, error) {
	version := strings.TrimPrefix(strings.TrimSpace(raw), "v")
	if base, _, found := strings.Cut(version, "-"); found {
		version = base
	}
	if version == "" || version == devVersion {
		return "", fmt.Errorf("版本号无效: %q", raw)
	}
	for _, segment := range strings.Split(version, ".") {
		if _, err := strconv.Atoi(segment); err != nil {
			return "", fmt.Errorf("版本号无效: %q", raw)
		}
	}
	return version, nil
}

// compareVersions returns >0 when a is newer than b, 0 when equal, <0 when
// older. Inputs are assumed to have passed validVersion.
func compareVersions(a, b string) int {
	as := strings.Split(strings.TrimPrefix(a, "v"), ".")
	bs := strings.Split(strings.TrimPrefix(b, "v"), ".")
	for i := 0; i < len(as) || i < len(bs); i++ {
		if diff := segment(as, i) - segment(bs, i); diff != 0 {
			return diff
		}
	}
	return 0
}

func segment(parts []string, i int) int {
	if i >= len(parts) {
		return 0
	}
	n, err := strconv.Atoi(parts[i])
	if err != nil {
		return 0
	}
	return n
}
