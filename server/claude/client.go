package claude

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"
)

const (
	apiEndpoint = "https://api.anthropic.com/v1/messages"
	model       = "claude-sonnet-4-20250514"
	maxTokens   = 4096
)

// Client is the Claude API client
type Client struct {
	apiKey     string
	httpClient *http.Client
}

// NewClient creates a new Claude API client
func NewClient() *Client {
	// Try CLAUDE_API_KEY first, then fall back to ANTHROPIC_API_KEY
	apiKey := os.Getenv("CLAUDE_API_KEY")
	if apiKey == "" {
		apiKey = os.Getenv("ANTHROPIC_API_KEY")
	}
	return &Client{
		apiKey: apiKey,
		httpClient: &http.Client{
			Timeout: 120 * time.Second,
		},
	}
}

// ContentBlock represents a content block in a message
type ContentBlock struct {
	Type      string          `json:"type"`
	Text      string          `json:"text,omitempty"`
	Source    *ImageSource    `json:"source,omitempty"`
	ID        string          `json:"id,omitempty"`
	Name      string          `json:"name,omitempty"`
	Input     json.RawMessage `json:"input,omitempty"`
	ToolUseID string          `json:"tool_use_id,omitempty"`
	Content   string          `json:"content,omitempty"`
	IsError   bool            `json:"is_error,omitempty"`
}

// ImageSource represents the source of an image
type ImageSource struct {
	Type      string `json:"type"`
	MediaType string `json:"media_type"`
	Data      string `json:"data"`
}

// ConversationMessage represents a message in a conversation
type ConversationMessage struct {
	Role    string         `json:"role"`
	Content []ContentBlock `json:"content"`
}

// Tool represents a Claude tool definition
type Tool struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	InputSchema json.RawMessage `json:"input_schema"`
}

// APIRequest represents a request to the Claude API
type APIRequest struct {
	Model     string                `json:"model"`
	MaxTokens int                   `json:"max_tokens"`
	System    string                `json:"system,omitempty"`
	Messages  []ConversationMessage `json:"messages"`
	Tools     []Tool                `json:"tools,omitempty"`
}

// APIResponse represents a response from the Claude API
type APIResponse struct {
	ID           string         `json:"id"`
	Type         string         `json:"type"`
	Role         string         `json:"role"`
	Content      []ContentBlock `json:"content"`
	Model        string         `json:"model"`
	StopReason   string         `json:"stop_reason"`
	StopSequence string         `json:"stop_sequence,omitempty"`
	Usage        struct {
		InputTokens  int `json:"input_tokens"`
		OutputTokens int `json:"output_tokens"`
	} `json:"usage"`
}

// ToolCall represents a tool call from Claude
type ToolCall struct {
	ID    string          `json:"id"`
	Name  string          `json:"name"`
	Input json.RawMessage `json:"input"`
}

// ChatResponse represents the response from a chat
type ChatResponse struct {
	TextContent string
	ToolCalls   []ToolCall
	StopReason  string
	Usage       struct {
		InputTokens  int
		OutputTokens int
	}
}

// SystemPrompt is the default system prompt for the browser automation assistant
const SystemPrompt = `你是一個瀏覽器自動化助手。你可以看到用戶電腦上的瀏覽器截圖，並使用工具來控制瀏覽器。

可用工具：
- take_screenshot: 截取當前畫面
- click: 點擊指定座標
- type_text: 輸入文字
- press_key: 按下按鍵
- navigate: 導航到網址
- scroll: 滾動頁面

規則：
1. 執行動作前，先描述你看到了什麼以及你要做什麼
2. 點擊時，精確計算目標元素的中心座標
3. 執行動作後，截取新的截圖確認結果
4. 如果動作失敗，嘗試其他方法或詢問用戶
5. 座標系統：螢幕解析度 1920x1080

回應格式：
- 先用自然語言說明你的觀察和計劃
- 然後調用工具執行動作
- 最後確認結果`

// Chat sends a chat message to Claude with optional screenshot
func (c *Client) Chat(messages []ConversationMessage, tools []Tool) (*ChatResponse, error) {
	if c.apiKey == "" {
		return nil, fmt.Errorf("ANTHROPIC_API_KEY not set")
	}

	req := APIRequest{
		Model:     model,
		MaxTokens: maxTokens,
		System:    SystemPrompt,
		Messages:  messages,
		Tools:     tools,
	}

	jsonBody, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	httpReq, err := http.NewRequest("POST", apiEndpoint, bytes.NewBuffer(jsonBody))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("x-api-key", c.apiKey)
	httpReq.Header.Set("anthropic-version", "2023-06-01")

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API error (status %d): %s", resp.StatusCode, string(body))
	}

	var apiResp APIResponse
	if err := json.Unmarshal(body, &apiResp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	// Parse response content
	chatResp := &ChatResponse{
		StopReason: apiResp.StopReason,
	}
	chatResp.Usage.InputTokens = apiResp.Usage.InputTokens
	chatResp.Usage.OutputTokens = apiResp.Usage.OutputTokens

	for _, block := range apiResp.Content {
		switch block.Type {
		case "text":
			chatResp.TextContent += block.Text
		case "tool_use":
			chatResp.ToolCalls = append(chatResp.ToolCalls, ToolCall{
				ID:    block.ID,
				Name:  block.Name,
				Input: block.Input,
			})
		}
	}

	return chatResp, nil
}

// CreateTextMessage creates a simple text message
func CreateTextMessage(role, text string) ConversationMessage {
	return ConversationMessage{
		Role: role,
		Content: []ContentBlock{
			{Type: "text", Text: text},
		},
	}
}

// CreateImageMessage creates a message with text and image
func CreateImageMessage(role, text, base64Image string) ConversationMessage {
	content := []ContentBlock{}

	if base64Image != "" {
		// Parse data URI format: data:image/jpeg;base64,<data>
		mediaType := "image/png" // default
		imageData := base64Image

		if len(base64Image) > 5 && base64Image[:5] == "data:" {
			// Find the comma that separates metadata from data
			commaIdx := -1
			for i := 0; i < len(base64Image) && i < 100; i++ {
				if base64Image[i] == ',' {
					commaIdx = i
					break
				}
			}
			if commaIdx > 0 {
				// Extract media type from "data:image/jpeg;base64"
				metadata := base64Image[5:commaIdx]
				semicolonIdx := -1
				for i := 0; i < len(metadata); i++ {
					if metadata[i] == ';' {
						semicolonIdx = i
						break
					}
				}
				if semicolonIdx > 0 {
					mediaType = metadata[:semicolonIdx]
				} else {
					mediaType = metadata
				}
				imageData = base64Image[commaIdx+1:]
			}
		}

		content = append(content, ContentBlock{
			Type: "image",
			Source: &ImageSource{
				Type:      "base64",
				MediaType: mediaType,
				Data:      imageData,
			},
		})
	}

	if text != "" {
		content = append(content, ContentBlock{
			Type: "text",
			Text: text,
		})
	}

	return ConversationMessage{
		Role:    role,
		Content: content,
	}
}

// CreateToolResultMessage creates a tool result message
func CreateToolResultMessage(results []ToolResult) ConversationMessage {
	content := []ContentBlock{}
	for _, r := range results {
		content = append(content, ContentBlock{
			Type:      "tool_result",
			ToolUseID: r.ToolUseID,
			Content:   r.Content,
			IsError:   r.IsError,
		})
	}
	return ConversationMessage{
		Role:    "user",
		Content: content,
	}
}

// CreateAssistantToolUseMessage creates an assistant message with tool uses
func CreateAssistantToolUseMessage(textContent string, toolCalls []ToolCall) ConversationMessage {
	content := []ContentBlock{}

	if textContent != "" {
		content = append(content, ContentBlock{
			Type: "text",
			Text: textContent,
		})
	}

	for _, tc := range toolCalls {
		content = append(content, ContentBlock{
			Type:  "tool_use",
			ID:    tc.ID,
			Name:  tc.Name,
			Input: tc.Input,
		})
	}

	return ConversationMessage{
		Role:    "assistant",
		Content: content,
	}
}

// ToolResult represents the result of a tool execution
type ToolResult struct {
	ToolUseID string `json:"tool_use_id"`
	Content   string `json:"content"`
	IsError   bool   `json:"is_error,omitempty"`
}

// GetAPIKey returns the API key status (for debugging)
func (c *Client) GetAPIKey() string {
	if c.apiKey == "" {
		return "(not set)"
	}
	if len(c.apiKey) > 10 {
		return c.apiKey[:10] + "..."
	}
	return c.apiKey
}
