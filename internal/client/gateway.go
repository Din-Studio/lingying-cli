package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"strings"
	"time"
)

const (
	GatewayBase = "https://console.echojoy.cn/gateway"
	FileService = "https://file.echojoy.cn"
)

// ── Model types from GET /v1/models ──

type GatewayModel struct {
	ID           string              `json:"id"`
	Object       string              `json:"object"`
	ModelType    string              `json:"model_type"`
	ModelID      string              `json:"model_id"`
	DisplayName  string              `json:"display_name"`
	APIFormat    string              `json:"api_format,omitempty"`
	InputSchema  json.RawMessage     `json:"input_schema,omitempty"`
	FeatureTypes map[string]Feature  `json:"feature_types,omitempty"`
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
	token string
	http  *http.Client
}

func New(token string) *Client {
	return &Client{
		token: token,
		http:  &http.Client{Timeout: 120 * time.Second},
	}
}

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
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(respBody[:min(len(respBody), 300)]))
	}
	return respBody, nil
}

// ── Model discovery ──

func (c *Client) ListModels(ctx context.Context) ([]GatewayModel, error) {
	body, err := c.doJSON(ctx, "GET", GatewayBase+"/v1/models", nil)
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
	endpoint := GatewayBase + "/v1/chat/completions"
	if apiFormat == "anthropic" {
		endpoint = GatewayBase + "/v1/messages"
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
		return "", err
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

		body, err := c.doJSON(ctx, "GET", fmt.Sprintf("%s/v1/tasks/%s", GatewayBase, taskID), nil)
		if err != nil {
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
				return body, fmt.Errorf("%s: %s", resp.Data.ErrorCode, resp.Data.Error)
			}
		}
	}
	return nil, fmt.Errorf("任务超时 (%ds)", maxSeconds)
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
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

// ── File upload (OAuth only) ──

func (c *Client) UploadFile(ctx context.Context, name, localPath string) (string, error) {
	data, err := os.ReadFile(localPath)
	if err != nil {
		return "", fmt.Errorf("读取文件失败: %w", err)
	}

	var b bytes.Buffer
	w := multipart.NewWriter(&b)
	fw, err := w.CreateFormFile("file", name)
	if err != nil {
		return "", err
	}
	fw.Write(data)
	w.WriteField("name", name)
	w.Close()

	req, err := http.NewRequestWithContext(ctx, "POST", FileService+"/api/v1/files", &b)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Content-Type", w.FormDataContentType())

	resp, err := c.http.Do(req)
	if err != nil {
		return "", fmt.Errorf("上传失败: %w", err)
	}
	defer resp.Body.Close()

	respData, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
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
	if result.Code != 0 {
		return "", fmt.Errorf("上传失败: %s", result.Message)
	}
	return result.Data.DownloadURL, nil
}

// ── Helpers ──

func maskSecret(s string) string {
	if i := strings.Index(s, "Bearer "); i >= 0 {
		return s[:i+7] + "****"
	}
	return s
}
