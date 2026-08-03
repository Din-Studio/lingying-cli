package client

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"
)

const (
	GatewayBase = "https://console.echojoy.cn/gateway"
	FileService = "https://file.echojoy.cn"
	// MaxUploadBytes matches the guaranteed single-file capacity of the
	// pre-signed cloud-storage upload path.
	MaxUploadBytes int64 = 1024 * 1024 * 1024
)

// ── Model types from GET /v1/models ──

type GatewayModel struct {
	ID           string             `json:"id"`
	Object       string             `json:"object"`
	ModelType    string             `json:"model_type"`
	ModelID      string             `json:"model_id"`
	DisplayName  string             `json:"display_name"`
	APIFormat    string             `json:"api_format,omitempty"`
	InputSchema  json.RawMessage    `json:"input_schema,omitempty"`
	FeatureTypes map[string]Feature `json:"feature_types,omitempty"`
}

type Feature struct {
	MatchFields []string `json:"match_fields"`
}

type ModelListResponse struct {
	Object  string         `json:"object"`
	Data    []GatewayModel `json:"data"`
	HasMore bool           `json:"has_more"`
}

// ── HTTP client ──

type Client struct {
	token          string
	baseURL        string
	fileServiceURL string
	http           *http.Client
}

type ErrorDetail struct {
	Code      string
	Message   string
	RequestID string
}

type GatewayError struct {
	Status int
	ErrorDetail
}

func (e *GatewayError) Error() string {
	if e.Code != "" {
		return fmt.Sprintf("%s: %s", e.Code, e.Message)
	}
	return fmt.Sprintf("HTTP %d: %s", e.Status, e.Message)
}

// ErrorDetails exposes only safe, user-actionable upstream error fields.
func ErrorDetails(err error) ErrorDetail {
	var gatewayErr *GatewayError
	if errors.As(err, &gatewayErr) {
		return gatewayErr.ErrorDetail
	}
	return ErrorDetail{Code: "command_error", Message: err.Error()}
}

func New(token string) *Client {
	return NewWithBaseURL(token, GatewayBase)
}

// NewWithBaseURL creates a client for an alternate Gateway endpoint. It keeps
// HTTP integration tests isolated and is also useful for self-hosted gateways.
func NewWithBaseURL(token, baseURL string) *Client {
	return NewWithEndpoints(token, baseURL, FileService)
}

// NewWithEndpoints creates a client with explicit Gateway and file-service
// URLs. It exists for self-hosted deployments and HTTP integration tests.
func NewWithEndpoints(token, baseURL, fileServiceURL string) *Client {
	return &Client{
		token:          token,
		baseURL:        strings.TrimRight(baseURL, "/"),
		fileServiceURL: strings.TrimRight(fileServiceURL, "/"),
		http:           &http.Client{Timeout: 120 * time.Second},
	}
}

func (c *Client) gateway(path string) string { return c.baseURL + path }

func (c *Client) MediaEndpoint(path string) string { return c.gateway(path) }

func (c *Client) doJSON(ctx context.Context, method, url string, body any) ([]byte, error) {
	var r io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("marshal: %w", err)
		}
		r = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, url, r)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	if method != "GET" {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("请求失败: %s", maskSecret(err.Error()))
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("读取响应失败: %w", err)
	}
	if resp.StatusCode >= 400 {
		return nil, parseGatewayError(resp.StatusCode, respBody)
	}
	return respBody, nil
}

func parseGatewayError(status int, body []byte) error {
	var payload struct {
		Code      string `json:"code"`
		ErrorCode string `json:"error_code"`
		Message   string `json:"message"`
		RequestID string `json:"request_id"`
		Error     struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	_ = json.Unmarshal(body, &payload)
	detail := ErrorDetail{Code: payload.ErrorCode, Message: payload.Message, RequestID: payload.RequestID}
	if detail.Code == "" {
		detail.Code = payload.Code
	}
	if detail.Code == "" {
		detail.Code = payload.Error.Code
	}
	if detail.Message == "" {
		detail.Message = payload.Error.Message
	}
	if detail.Code == "" && status == http.StatusPaymentRequired {
		detail.Code = "INSUFFICIENT_BALANCE"
	}
	if detail.Message == "" {
		detail.Message = string(body[:min(len(body), 300)])
	}
	return &GatewayError{Status: status, ErrorDetail: detail}
}

// ── Model discovery ──

func (c *Client) ListModels(ctx context.Context) ([]GatewayModel, error) {
	body, err := c.doJSON(ctx, "GET", c.gateway("/v1/models"), nil)
	if err != nil {
		return nil, err
	}
	var resp ModelListResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("解析模型列表失败: %w", err)
	}
	return resp.Data, nil
}

// ── Text ──

type ChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

func (c *Client) Chat(ctx context.Context, modelID string, apiFormat string, messages []ChatMessage, maxTokens int) (string, error) {
	endpoint := c.gateway("/v1/chat/completions")
	if apiFormat == "anthropic" {
		endpoint = c.gateway("/v1/messages")
	}

	payload := map[string]any{
		"model":    modelID,
		"messages": messages,
	}
	if maxTokens > 0 {
		payload["max_tokens"] = maxTokens
	}

	respBody, err := c.doJSON(ctx, "POST", endpoint, payload)
	if err != nil {
		return "", err
	}

	// Try OpenAI format
	var oai struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if json.Unmarshal(respBody, &oai) == nil && len(oai.Choices) > 0 {
		return oai.Choices[0].Message.Content, nil
	}

	// Try Anthropic format
	var ant struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
	}
	if json.Unmarshal(respBody, &ant) == nil && len(ant.Content) > 0 {
		var texts []string
		for _, b := range ant.Content {
			if b.Text != "" {
				texts = append(texts, b.Text)
			}
		}
		return strings.Join(texts, "\n"), nil
	}

	return "", fmt.Errorf("无法解析响应: %s", string(respBody)[:300])
}

// ── Async media ──

func (c *Client) SubmitTask(ctx context.Context, endpoint, modelID string, inputData map[string]any) (string, error) {
	payload := map[string]any{
		"model_id":   modelID,
		"input_data": inputData,
	}
	body, err := c.doJSON(ctx, "POST", endpoint, payload)
	if err != nil {
		return "", fmt.Errorf("提交任务: %w", err)
	}
	var resp struct {
		Data struct {
			TaskID string `json:"task_id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return "", fmt.Errorf("解析提交响应: %s", string(body)[:200])
	}
	if resp.Data.TaskID == "" {
		return "", fmt.Errorf("提交失败: %s", string(body)[:300])
	}
	return resp.Data.TaskID, nil
}

func (c *Client) PollTask(ctx context.Context, taskID string, interval, maxSeconds int) ([]byte, error) {
	deadline := time.Now().Add(time.Duration(maxSeconds) * time.Second)
	failures := 0

	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(time.Duration(interval) * time.Second):
		}

		body, err := c.doJSON(ctx, "GET", fmt.Sprintf("%s/v1/tasks/%s", c.baseURL, taskID), nil)
		if err != nil {
			var gatewayErr *GatewayError
			if errors.As(err, &gatewayErr) {
				return nil, gatewayErr
			}
			failures++
			if failures >= 5 {
				return nil, fmt.Errorf("连续 %d 次轮询失败", failures)
			}
			continue
		}
		failures = 0

		var resp struct {
			Data struct {
				Status    string `json:"status"`
				Result    any    `json:"result"`
				ErrorCode string `json:"error_code"`
				Error     string `json:"error"`
			} `json:"data"`
		}
		if json.Unmarshal(body, &resp) == nil {
			switch resp.Data.Status {
			case "success":
				return body, nil
			case "failed":
				return body, &GatewayError{ErrorDetail: ErrorDetail{
					Code:    resp.Data.ErrorCode,
					Message: resp.Data.Error,
				}}
			}
		}
	}
	return nil, fmt.Errorf("任务超时 (%ds)", maxSeconds)
}

// GetTask returns the Gateway task envelope without submitting new work.
func (c *Client) GetTask(ctx context.Context, taskID string) ([]byte, error) {
	return c.doJSON(ctx, "GET", fmt.Sprintf("%s/v1/tasks/%s", c.baseURL, taskID), nil)
}

// ExtractResultURLs returns media URLs from a Gateway task envelope. Gateway
// providers use slightly different result shapes, so URLs are collected from
// conventional URL-named fields recursively and de-duplicated in encounter order.
func ExtractResultURLs(body []byte) ([]string, error) {
	var value any
	if err := json.Unmarshal(body, &value); err != nil {
		return nil, fmt.Errorf("解析任务结果失败: %w", err)
	}
	seen := make(map[string]bool)
	var urls []string
	var visit func(any, string)
	visit = func(v any, key string) {
		switch typed := v.(type) {
		case map[string]any:
			for childKey, child := range typed {
				visit(child, childKey)
			}
		case []any:
			for _, child := range typed {
				visit(child, key)
			}
		case string:
			if (key == "url" || key == "download_url" || key == "file_url") &&
				(strings.HasPrefix(typed, "https://") || strings.HasPrefix(typed, "http://")) && !seen[typed] {
				seen[typed] = true
				urls = append(urls, typed)
			}
		}
	}
	visit(value, "")
	sort.Strings(urls)
	return urls, nil
}

func (c *Client) Download(ctx context.Context, url, path string) error {
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return fmt.Errorf("下载失败: HTTP %d", resp.StatusCode)
	}
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, "."+filepath.Base(path)+"-*.partial")
	if err != nil {
		return fmt.Errorf("创建下载临时文件失败: %w", err)
	}
	tmpPath := tmp.Name()
	committed := false
	defer func() {
		if !committed {
			_ = os.Remove(tmpPath)
		}
	}()
	if _, err := io.Copy(tmp, resp.Body); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("下载写入失败: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("关闭下载临时文件失败: %w", err)
	}
	if err := replaceFile(tmpPath, path); err != nil {
		return fmt.Errorf("保存下载结果失败: %w", err)
	}
	committed = true
	return nil
}

// ── File upload (OAuth only) ──

func (c *Client) UploadFile(ctx context.Context, name, localPath string) (string, error) {
	file, err := os.Open(localPath)
	if err != nil {
		return "", fmt.Errorf("打开上传文件失败: %w", err)
	}
	fileClosed := false
	defer func() {
		if !fileClosed {
			_ = file.Close()
		}
	}()
	if info, err := file.Stat(); err != nil {
		return "", fmt.Errorf("读取上传文件信息失败: %w", err)
	} else if info.Size() > MaxUploadBytes {
		return "", fmt.Errorf("文件超过上传上限 %d MiB", MaxUploadBytes/(1024*1024))
	}

	tmp, err := os.CreateTemp("", "ly-upload-*.multipart")
	if err != nil {
		return "", fmt.Errorf("创建上传临时文件失败: %w", err)
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	w := multipart.NewWriter(tmp)
	fw, err := w.CreateFormFile("file", name)
	if err != nil {
		_ = tmp.Close()
		return "", fmt.Errorf("创建上传表单失败: %w", err)
	}
	written, err := io.Copy(fw, io.LimitReader(file, MaxUploadBytes+1))
	closeErr := file.Close()
	fileClosed = true
	if err != nil {
		_ = tmp.Close()
		return "", fmt.Errorf("读取上传文件失败: %w", err)
	}
	if closeErr != nil {
		_ = tmp.Close()
		return "", fmt.Errorf("关闭上传文件失败: %w", closeErr)
	}
	if written > MaxUploadBytes {
		_ = tmp.Close()
		return "", fmt.Errorf("文件超过上传上限 %d MiB", MaxUploadBytes/(1024*1024))
	}
	if err := w.WriteField("name", name); err != nil {
		_ = tmp.Close()
		return "", fmt.Errorf("写入上传表单失败: %w", err)
	}
	contentType := w.FormDataContentType()
	if err := w.Close(); err != nil {
		_ = tmp.Close()
		return "", fmt.Errorf("完成上传表单失败: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return "", fmt.Errorf("关闭上传临时文件失败: %w", err)
	}
	body, err := os.Open(tmpPath)
	if err != nil {
		return "", fmt.Errorf("读取上传临时文件失败: %w", err)
	}
	defer body.Close()

	req, err := http.NewRequestWithContext(ctx, "POST", c.fileServiceURL+"/api/v1/files", body)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Content-Type", contentType)

	resp, err := c.http.Do(req)
	if err != nil {
		return "", fmt.Errorf("上传失败: %w", err)
	}
	defer resp.Body.Close()

	respData, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("读取上传响应失败: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("上传失败: HTTP %d: %s", resp.StatusCode, string(respData[:min(len(respData), 300)]))
	}

	var result struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
		Data    struct {
			DownloadURL string `json:"download_url"`
			FileID      string `json:"file_id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(respData, &result); err != nil {
		return "", fmt.Errorf("解析上传响应失败: %s", string(respData)[:200])
	}
	if result.Code != 0 || result.Data.DownloadURL == "" {
		return "", fmt.Errorf("上传失败: %s", result.Message)
	}
	return result.Data.DownloadURL, nil
}

func replaceFile(tempPath, destination string) error {
	if runtime.GOOS != "windows" {
		return os.Rename(tempPath, destination)
	}
	if _, err := os.Stat(destination); err != nil {
		if os.IsNotExist(err) {
			return os.Rename(tempPath, destination)
		}
		return err
	}
	backup, err := os.CreateTemp(filepath.Dir(destination), "."+filepath.Base(destination)+"-*.backup")
	if err != nil {
		return err
	}
	backupPath := backup.Name()
	if err := backup.Close(); err != nil {
		return err
	}
	if err := os.Remove(backupPath); err != nil {
		return err
	}
	if err := os.Rename(destination, backupPath); err != nil {
		return err
	}
	if err := os.Rename(tempPath, destination); err != nil {
		_ = os.Rename(backupPath, destination)
		return err
	}
	return os.Remove(backupPath)
}

// ── Helpers ──

func maskSecret(s string) string {
	if i := strings.Index(s, "Bearer "); i >= 0 {
		return s[:i+7] + "****"
	}
	return s
}
