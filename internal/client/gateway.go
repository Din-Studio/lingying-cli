package client

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"
	"unicode/utf8"
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
		if err := validateUTF8JSONValue(body); err != nil {
			return nil, err
		}
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

// validateUTF8JSONValue rejects invalid UTF-8 before encoding JSON. The Go
// JSON encoder would otherwise replace invalid bytes with U+FFFD, silently
// changing prompts or schema parameters after the user has reviewed them.
func validateUTF8JSONValue(value any) error {
	switch typed := value.(type) {
	case string:
		if !utf8.ValidString(typed) {
			return fmt.Errorf("请求内容包含无效 UTF-8 字符")
		}
	case map[string]any:
		for key, child := range typed {
			if !utf8.ValidString(key) {
				return fmt.Errorf("请求字段名包含无效 UTF-8 字符")
			}
			if err := validateUTF8JSONValue(child); err != nil {
				return err
			}
		}
	case []any:
		for _, child := range typed {
			if err := validateUTF8JSONValue(child); err != nil {
				return err
			}
		}
	case []string:
		for _, child := range typed {
			if err := validateUTF8JSONValue(child); err != nil {
				return err
			}
		}
	case []ChatMessage:
		for _, message := range typed {
			if err := validateUTF8JSONValue(message.Role); err != nil {
				return err
			}
			if err := validateUTF8JSONValue(message.Content); err != nil {
				return err
			}
		}
	}
	return nil
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

func (c *Client) Chat(ctx context.Context, modelID string, apiFormat string, messages []ChatMessage, params map[string]any) (string, error) {
	endpoint := c.gateway("/v1/chat/completions")
	if apiFormat == "anthropic" {
		endpoint = c.gateway("/v1/messages")
	}

	payload := make(map[string]any, len(params)+2)
	for key, value := range params {
		payload[key] = value
	}
	payload["model"] = modelID
	payload["messages"] = messages

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

// ── File upload (OAuth only, presigned) ──

// 上传参数为包级变量（非 const），测试可调小以触发分片路径。
// 数值与 media-sync worker 的生产默认值一致。
var (
	multipartThreshold int64 = 50 * 1024 * 1024 // ≥ 此值走分片
	uploadPartSize     int64 = 5 * 1024 * 1024
	uploadConcurrency        = 4
)

type presignedInitResponse struct {
	FileID       string `json:"file_id"`
	UploadURL    string `json:"upload_url"`
	Status       string `json:"status"`
	Deduplicated bool   `json:"deduplicated"`
}

// UploadFile 通过 AssetHub 预签名流程上传本地文件并返回下载直链：
// init →（未命中去重时）PUT 预签名 URL → completion → link。
func (c *Client) UploadFile(ctx context.Context, name, localPath string) (string, error) {
	file, err := os.Open(localPath)
	if err != nil {
		return "", fmt.Errorf("打开上传文件失败: %w", err)
	}
	defer file.Close()

	info, err := file.Stat()
	if err != nil {
		return "", fmt.Errorf("读取上传文件信息失败: %w", err)
	}
	size := info.Size()
	if size > MaxUploadBytes {
		return "", fmt.Errorf("文件超过上传上限 %d MiB", MaxUploadBytes/(1024*1024))
	}

	contentType, err := detectFileContentType(name, file)
	if err != nil {
		return "", fmt.Errorf("识别文件类型失败: %w", err)
	}

	var fileID string
	if size < multipartThreshold {
		fileID, err = c.uploadSmall(ctx, file, name, contentType, size)
	} else {
		fileID, err = c.uploadMultipart(ctx, localPath, name, contentType, size)
	}
	if err != nil {
		return "", err
	}
	return c.fileLink(ctx, fileID)
}

// uploadSmall 单次预签名 PUT。带 sha256 哈希以启用服务端内容去重。
func (c *Client) uploadSmall(ctx context.Context, file *os.File, name, contentType string, size int64) (string, error) {
	h := sha256.New()
	if _, err := io.Copy(h, file); err != nil {
		return "", fmt.Errorf("计算文件哈希失败: %w", err)
	}
	var init presignedInitResponse
	err := c.fileAPI(ctx, http.MethodPost, "/api/v1/files/presigned", map[string]any{
		"name":         name,
		"size":         size,
		"content_type": contentType,
		"hash":         hex.EncodeToString(h.Sum(nil)),
	}, &init)
	if err != nil {
		return "", err
	}
	// 去重短路：服务端已有相同内容，无需再传字节。
	if init.Deduplicated || (init.Status == "completed" && init.UploadURL == "") {
		return init.FileID, nil
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return "", fmt.Errorf("读取上传文件失败: %w", err)
	}
	if _, err := c.putPresigned(ctx, init.UploadURL, contentType, file, size); err != nil {
		return "", err
	}
	if err := c.fileAPI(ctx, http.MethodPost, "/api/v1/files/"+init.FileID+"/completion", nil, nil); err != nil {
		return "", err
	}
	return init.FileID, nil
}

// uploadMultipart 在后续任务中实现分片路径。
func (c *Client) uploadMultipart(ctx context.Context, localPath, name, contentType string, size int64) (string, error) {
	return "", fmt.Errorf("分片上传未实现")
}

// putPresigned 把 body PUT 到预签名 URL。不带应用鉴权头（URL 自鉴权）。
// Content-Type 参与签名，必须与 init 时一致。返回响应头 ETag（分片上传需要）。
func (c *Client) putPresigned(ctx context.Context, url, contentType string, body io.Reader, size int64) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, url, body)
	if err != nil {
		return "", err
	}
	req.ContentLength = size
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return "", fmt.Errorf("上传失败: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
		return "", fmt.Errorf("上传失败: 预签名 PUT HTTP %d", resp.StatusCode)
	}
	return resp.Header.Get("ETag"), nil
}

// fileLink 获取文件下载直链（去重/单次/分片三条路径统一出口）。
func (c *Client) fileLink(ctx context.Context, fileID string) (string, error) {
	var out struct {
		DownloadURL string `json:"download_url"`
	}
	if err := c.fileAPI(ctx, http.MethodGet, "/api/v1/files/"+fileID+"/link?url_format=direct", nil, &out); err != nil {
		return "", err
	}
	if out.DownloadURL == "" {
		return "", fmt.Errorf("上传成功但下载链接为空")
	}
	return out.DownloadURL, nil
}

// fileAPI 调用 AssetHub JSON 接口并解开 {code, message, data} 信封。
func (c *Client) fileAPI(ctx context.Context, method, path string, body, out any) error {
	var reader io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.fileServiceURL+path, reader)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("文件服务请求失败: %w", err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return fmt.Errorf("读取文件服务响应失败: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("文件服务 HTTP %d: %s", resp.StatusCode, string(data[:min(len(data), 300)]))
	}
	var env struct {
		Code    int             `json:"code"`
		Message string          `json:"message"`
		Data    json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(data, &env); err != nil {
		return fmt.Errorf("解析文件服务响应失败: %s", string(data[:min(len(data), 200)]))
	}
	if env.Code != 0 {
		return fmt.Errorf("文件服务错误: %s", env.Message)
	}
	if out != nil {
		if err := json.Unmarshal(env.Data, out); err != nil {
			return fmt.Errorf("解析文件服务响应失败: %s", string(data[:min(len(data), 200)]))
		}
	}
	return nil
}

// detectFileContentType 先按扩展名推断，取不到再嗅探前 512 字节。
// 返回前把读取偏移复位到文件开头。
func detectFileContentType(name string, file *os.File) (string, error) {
	if ct := mime.TypeByExtension(strings.ToLower(filepath.Ext(name))); ct != "" {
		return ct, nil
	}
	buf := make([]byte, 512)
	n, err := file.Read(buf)
	if err != nil && !errors.Is(err, io.EOF) {
		return "", err
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return "", err
	}
	return http.DetectContentType(buf[:n]), nil
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
