// Copyright (c) 2026 Lingying. SPDX-License-Identifier: MIT
// Updater — 带重试、Range 续传与停滞检测的下载器。
package updater

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"
)

// downloader 下载 Release 资产。
//
// 超时判据是「停滞」而非总时长：只要仍有字节进入就继续等，连续 stallWindow
// 内一个字节都没有才判定失败。总时长上限会在慢链路上把大文件硬砍断——那正是
// 旧实现的 downloadTimeout 与 install.sh 的 --max-time 120 的缺陷。
type downloader struct {
	client      *http.Client
	attempts    int
	stallWindow time.Duration
	retryDelay  time.Duration
}

func newDownloader() *downloader {
	return &downloader{
		client: &http.Client{
			// 不设 Client.Timeout：总时长上限正是要避免的东西。
			Transport: &http.Transport{
				TLSHandshakeTimeout:   15 * time.Second,
				ResponseHeaderTimeout: 30 * time.Second,
			},
		},
		attempts:    5,
		stallWindow: 30 * time.Second,
		retryDelay:  2 * time.Second,
	}
}

// stallReader 每读到字节就重置计时器；计时器到期则取消请求上下文。
type stallReader struct {
	inner io.Reader
	timer *time.Timer
	win   time.Duration
}

func (s *stallReader) Read(p []byte) (int, error) {
	n, err := s.inner.Read(p)
	if n > 0 {
		s.timer.Reset(s.win)
	}
	return n, err
}

// fatalStatusError 标记不该重试的确定性失败（如 404）。
type fatalStatusError struct{ code int }

func (e *fatalStatusError) Error() string { return fmt.Sprintf("HTTP %d", e.code) }

// fetch 下载 url 到 path。重试时以 Range 头从已有字节处续传，而非从头重来。
func (d *downloader) fetch(ctx context.Context, url, path string) error {
	var lastErr error
	for attempt := 1; attempt <= d.attempts; attempt++ {
		if attempt > 1 && d.retryDelay > 0 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(d.retryDelay):
			}
		}
		err := d.fetchOnce(ctx, url, path)
		if err == nil {
			return nil
		}
		var fatal *fatalStatusError
		if errors.As(err, &fatal) {
			return fmt.Errorf("下载 %s 失败: %w", url, err)
		}
		lastErr = err
	}
	return fmt.Errorf("下载 %s 失败（已重试 %d 次）: %w", url, d.attempts, lastErr)
}

func (d *downloader) fetchOnce(ctx context.Context, url, path string) error {
	var offset int64
	if info, err := os.Stat(path); err == nil {
		offset = info.Size()
	}

	reqCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "ly-cli-updater")
	if offset > 0 {
		req.Header.Set("Range", fmt.Sprintf("bytes=%d-", offset))
	}

	resp, err := d.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusOK:
		offset = 0 // 服务端忽略了 Range，从头写
	case http.StatusPartialContent:
		// 续传生效，保留 offset
	case http.StatusRequestedRangeNotSatisfiable:
		_ = os.Remove(path)
		return errors.New("服务端拒绝续传区间，已重置本地分片")
	case http.StatusNotFound, http.StatusUnauthorized, http.StatusForbidden, http.StatusGone:
		return &fatalStatusError{code: resp.StatusCode}
	default:
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	flag := os.O_CREATE | os.O_WRONLY
	if offset > 0 {
		flag |= os.O_APPEND
	} else {
		flag |= os.O_TRUNC
	}
	f, err := os.OpenFile(path, flag, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()

	timer := time.AfterFunc(d.stallWindow, cancel)
	defer timer.Stop()

	if _, err := io.Copy(f, &stallReader{inner: resp.Body, timer: timer, win: d.stallWindow}); err != nil {
		if reqCtx.Err() != nil && ctx.Err() == nil {
			return fmt.Errorf("传输停滞超过 %s", d.stallWindow)
		}
		return err
	}
	return nil
}

// bytes 把 url 的内容读进内存，用于 checksums.txt 这类小文件。
func (d *downloader) bytes(ctx context.Context, url string) ([]byte, error) {
	f, err := os.CreateTemp("", "ly-fetch-*")
	if err != nil {
		return nil, err
	}
	name := f.Name()
	f.Close()
	defer os.Remove(name)

	// CreateTemp 建出的是 0 字节空文件，fetch 会把它当作空分片从头下载。
	if err := d.fetch(ctx, url, name); err != nil {
		return nil, err
	}
	return os.ReadFile(name)
}
