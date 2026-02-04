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
	apiEndpoint     = "https://api.anthropic.com/v1/messages"
	defaultModel    = "claude-sonnet-4-20250514"
	anthropicVersion = "2023-06-01"
	maxTokens       = 4096
)

// Client is the Claude API client
type Client struct {
	apiKey     string
	model      string
	httpClient *http.Client
}

// NewClient creates a new Claude API client
func NewClient() *Client {
	apiKey := os.Getenv("ANTHROPIC_API_KEY")
	model := os.Getenv("ANTHROPIC_MODEL")
	if model == "" {
		model = defaultModel
	}
	return &Client{
		apiKey: apiKey,
		model:  model,
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

// Tool represents a tool definition for Anthropic API
type Tool struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	InputSchema json.RawMessage `json:"input_schema"`
}

// ToolCall represents a tool call from the model
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

【絕對禁止 - 違反會導致失敗】
type_text 的參數只能是「要顯示在畫面上的文字」！
按鍵操作必須用 press_key！

絕對錯誤的用法（會導致失敗）：
✗ type_text("Tab") ← 錯！會打出 "Tab" 三個字
✗ type_text("Enter") ← 錯！會打出 "Enter" 五個字
✗ type_text("Backspace") ← 錯！會打出 "Backspace" 九個字
✗ type_text("20152Tab0538") ← 錯！Tab 變成文字

正確用法：
✓ press_key("Tab") ← 按下 Tab 鍵切換欄位
✓ press_key("Enter") ← 按下 Enter 鍵
✓ press_key("Backspace") ← 按下退格鍵刪除

登入範例（account=20152, password=0538）：
  1. click 點擊帳號欄位
  2. type_text("20152") ← 只輸入數字
  3. press_key("Tab") ← 用 press_key 切換欄位！
  4. type_text("0538") ← 只輸入數字
  5. press_key("Enter")

可用工具：
- get_page_state: 【推薦】取得頁面狀態（輸入框的值、座標、focus狀態），比截圖更快更準確
- take_screenshot: 截取當前畫面（需要看視覺內容時使用）
- click: 點擊指定座標 (x, y)
- type_text: 輸入純文字（不含任何按鍵！）
- press_key: 按下按鍵（Tab、Enter、Escape、Backspace 等）
- select_all: 全選當前輸入框內容
- navigate: 導航到網址
- scroll: 滾動頁面

清除輸入框：click 該欄位 → select_all → press_key("Backspace")

操作流程建議：
1. 先用 get_page_state 了解頁面有哪些輸入框和按鈕
2. 根據返回的座標執行 click 和 type_text
3. 操作後再用 get_page_state 確認結果（檢查輸入框的 value 是否正確）
4. 只在需要看視覺內容時才用 take_screenshot

一般規則：
1. 執行動作前，先描述你看到了什麼以及你要做什麼
2. 點擊時，使用 get_page_state 返回的座標
3. 座標系統：螢幕解析度 1920x1080`

// Anthropic API request/response types
type anthropicRequest struct {
	Model     string                `json:"model"`
	MaxTokens int                   `json:"max_tokens"`
	System    string                `json:"system,omitempty"`
	Messages  []anthropicMessage    `json:"messages"`
	Tools     []Tool                `json:"tools,omitempty"`
}

type anthropicMessage struct {
	Role    string        `json:"role"`
	Content []interface{} `json:"content"`
}

type anthropicResponse struct {
	ID           string `json:"id"`
	Type         string `json:"type"`
	Role         string `json:"role"`
	Content      []anthropicContentBlock `json:"content"`
	Model        string `json:"model"`
	StopReason   string `json:"stop_reason"`
	StopSequence string `json:"stop_sequence"`
	Usage        struct {
		InputTokens  int `json:"input_tokens"`
		OutputTokens int `json:"output_tokens"`
	} `json:"usage"`
}

type anthropicContentBlock struct {
	Type  string          `json:"type"`
	Text  string          `json:"text,omitempty"`
	ID    string          `json:"id,omitempty"`
	Name  string          `json:"name,omitempty"`
	Input json.RawMessage `json:"input,omitempty"`
}

type anthropicTextContent struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type anthropicImageContent struct {
	Type   string `json:"type"`
	Source struct {
		Type      string `json:"type"`
		MediaType string `json:"media_type"`
		Data      string `json:"data"`
	} `json:"source"`
}

type anthropicToolUseContent struct {
	Type  string          `json:"type"`
	ID    string          `json:"id"`
	Name  string          `json:"name"`
	Input json.RawMessage `json:"input"`
}

type anthropicToolResultContent struct {
	Type      string `json:"type"`
	ToolUseID string `json:"tool_use_id"`
	Content   string `json:"content"`
	IsError   bool   `json:"is_error,omitempty"`
}

func toAnthropicMessages(messages []ConversationMessage) []anthropicMessage {
	out := make([]anthropicMessage, 0, len(messages))

	for _, msg := range messages {
		content := make([]interface{}, 0, len(msg.Content))

		for _, block := range msg.Content {
			switch block.Type {
			case "text":
				content = append(content, anthropicTextContent{
					Type: "text",
					Text: block.Text,
				})
			case "image":
				if block.Source != nil {
					img := anthropicImageContent{Type: "image"}
					img.Source.Type = block.Source.Type
					img.Source.MediaType = block.Source.MediaType
					img.Source.Data = block.Source.Data
					content = append(content, img)
				}
			case "tool_use":
				content = append(content, anthropicToolUseContent{
					Type:  "tool_use",
					ID:    block.ID,
					Name:  block.Name,
					Input: block.Input,
				})
			case "tool_result":
				content = append(content, anthropicToolResultContent{
					Type:      "tool_result",
					ToolUseID: block.ToolUseID,
					Content:   block.Content,
					IsError:   block.IsError,
				})
			}
		}

		if len(content) > 0 {
			out = append(out, anthropicMessage{
				Role:    msg.Role,
				Content: content,
			})
		}
	}

	return out
}

// Chat sends a chat message to Anthropic Claude API
func (c *Client) Chat(messages []ConversationMessage, tools []Tool) (*ChatResponse, error) {
	if c.apiKey == "" {
		return nil, fmt.Errorf("ANTHROPIC_API_KEY not set")
	}

	req := anthropicRequest{
		Model:     c.model,
		MaxTokens: maxTokens,
		System:    SystemPrompt,
		Messages:  toAnthropicMessages(messages),
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
	httpReq.Header.Set("anthropic-version", anthropicVersion)

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

	var apiResp anthropicResponse
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
			if chatResp.TextContent != "" {
				chatResp.TextContent += "\n"
			}
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

// GetAPIKey returns the API key status (redacted for safety).
func (c *Client) GetAPIKey() string {
	if c.apiKey == "" {
		return "(not set)"
	}
	return "(redacted)"
}
