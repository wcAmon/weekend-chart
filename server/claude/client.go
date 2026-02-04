package claude

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

const (
	apiEndpoint  = "https://api.openai.com/v1/chat/completions"
	defaultModel = "gpt-4o"
	maxTokens    = 4096
)

// Client is the Claude API client
type Client struct {
	apiKey     string
	model      string
	httpClient *http.Client
}

// NewClient creates a new Claude API client
func NewClient() *Client {
	apiKey := os.Getenv("OPENAI_API_KEY")
	model := os.Getenv("OPENAI_MODEL")
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

// Tool represents a tool definition
type Tool struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	InputSchema json.RawMessage `json:"input_schema"`
}

type openAIContentPart struct {
	Type     string           `json:"type"`
	Text     string           `json:"text,omitempty"`
	ImageURL *openAIImageURL  `json:"image_url,omitempty"`
}

type openAIImageURL struct {
	URL    string `json:"url"`
	Detail string `json:"detail,omitempty"`
}

type openAIFunction struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Parameters  json.RawMessage `json:"parameters,omitempty"`
}

type openAITool struct {
	Type     string         `json:"type"`
	Function openAIFunction `json:"function"`
}

type openAIToolCall struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

type openAIMessage struct {
	Role       string           `json:"role"`
	Content    interface{}      `json:"content,omitempty"`
	ToolCalls  []openAIToolCall `json:"tool_calls,omitempty"`
	ToolCallID string           `json:"tool_call_id,omitempty"`
}

// openAIChatRequest represents a request to the OpenAI Chat Completions API
type openAIChatRequest struct {
	Model     string         `json:"model"`
	Messages  []openAIMessage `json:"messages"`
	Tools     []openAITool    `json:"tools,omitempty"`
	MaxTokens int            `json:"max_tokens,omitempty"`
}

// openAIChatResponse represents a response from the OpenAI Chat Completions API
type openAIChatResponse struct {
	ID      string `json:"id"`
	Model   string `json:"model"`
	Choices []struct {
		Message struct {
			Role      string           `json:"role"`
			Content   json.RawMessage  `json:"content"`
			ToolCalls []openAIToolCall `json:"tool_calls,omitempty"`
		} `json:"message"`
	} `json:"choices"`
	Usage struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
	} `json:"usage"`
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

【最重要規則 - 必須遵守】
type_text 只能輸入純文字，絕對不能包含 "Tab"、"Enter" 等按鍵名稱！
要切換欄位必須使用 press_key("Tab")，不是在文字中加入 Tab！

登入範例（account=20152, password=0538）：
✗ 錯誤: type_text("20152Tab0538") ← Tab 變成文字了！
✓ 正確:
  1. click 點擊帳號欄位
  2. type_text("20152")
  3. press_key("Tab")
  4. type_text("0538")
  5. press_key("Enter") 或 click 登入按鈕

可用工具：
- take_screenshot: 截取當前畫面
- click: 點擊指定座標 (x, y)
- type_text: 輸入純文字（不含任何按鍵！）
- press_key: 按下按鍵（Tab、Enter、Escape、Backspace 等）
- select_all: 全選當前輸入框內容
- navigate: 導航到網址
- scroll: 滾動頁面

清除輸入框：click 該欄位 → select_all → press_key("Backspace")

一般規則：
1. 執行動作前，先描述你看到了什麼以及你要做什麼
2. 點擊時，精確計算目標元素的中心座標
3. 執行動作後，截取新的截圖確認結果
4. 座標系統：螢幕解析度 1920x1080`

func toOpenAITools(tools []Tool) []openAITool {
	if len(tools) == 0 {
		return nil
	}

	out := make([]openAITool, 0, len(tools))
	for _, t := range tools {
		out = append(out, openAITool{
			Type: "function",
			Function: openAIFunction{
				Name:        t.Name,
				Description: t.Description,
				Parameters:  t.InputSchema,
			},
		})
	}
	return out
}

func toOpenAIMessages(messages []ConversationMessage) []openAIMessage {
	out := make([]openAIMessage, 0, len(messages)+1)
	out = append(out, openAIMessage{
		Role:    "system",
		Content: SystemPrompt,
	})

	for _, msg := range messages {
		hasToolUse := false
		hasToolResult := false
		hasImage := false
		for _, block := range msg.Content {
			switch block.Type {
			case "tool_use":
				hasToolUse = true
			case "tool_result":
				hasToolResult = true
			case "image":
				hasImage = true
			}
		}

		if hasToolResult && !hasToolUse {
			for _, block := range msg.Content {
				if block.Type != "tool_result" {
					continue
				}
				out = append(out, openAIMessage{
					Role:       "tool",
					ToolCallID: block.ToolUseID,
					Content:    block.Content,
				})
			}
			continue
		}

		if hasToolUse {
			var toolCalls []openAIToolCall
			var textParts []string
			for _, block := range msg.Content {
				switch block.Type {
				case "text":
					textParts = append(textParts, block.Text)
				case "tool_use":
					tc := openAIToolCall{
						ID:   block.ID,
						Type: "function",
					}
					tc.Function.Name = block.Name
					tc.Function.Arguments = string(block.Input)
					toolCalls = append(toolCalls, tc)
				}
			}

			var content interface{}
			if len(textParts) > 0 {
				content = strings.Join(textParts, "")
			}

			out = append(out, openAIMessage{
				Role:      "assistant",
				Content:   content,
				ToolCalls: toolCalls,
			})
			continue
		}

		if hasImage {
			parts := make([]openAIContentPart, 0, len(msg.Content))
			for _, block := range msg.Content {
				switch block.Type {
				case "text":
					parts = append(parts, openAIContentPart{
						Type: "text",
						Text: block.Text,
					})
				case "image":
					if block.Source == nil {
						continue
					}
					url := "data:" + block.Source.MediaType + ";base64," + block.Source.Data
					parts = append(parts, openAIContentPart{
						Type: "image_url",
						ImageURL: &openAIImageURL{
							URL:    url,
							Detail: "auto",
						},
					})
				}
			}
			out = append(out, openAIMessage{
				Role:    msg.Role,
				Content: parts,
			})
			continue
		}

		var textParts []string
		for _, block := range msg.Content {
			if block.Type == "text" {
				textParts = append(textParts, block.Text)
			}
		}
		out = append(out, openAIMessage{
			Role:    msg.Role,
			Content: strings.Join(textParts, ""),
		})
	}

	return out
}

func parseContentText(raw json.RawMessage) string {
	if len(raw) == 0 || string(raw) == "null" {
		return ""
	}

	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s
	}

	var parts []openAIContentPart
	if err := json.Unmarshal(raw, &parts); err == nil {
		var sb strings.Builder
		for _, p := range parts {
			if p.Type == "text" {
				sb.WriteString(p.Text)
			}
		}
		return sb.String()
	}

	return ""
}

// Chat sends a chat message to OpenAI with optional screenshot
func (c *Client) Chat(messages []ConversationMessage, tools []Tool) (*ChatResponse, error) {
	if c.apiKey == "" {
		return nil, fmt.Errorf("OPENAI_API_KEY not set")
	}

	req := openAIChatRequest{
		Model:     c.model,
		MaxTokens: maxTokens,
		Messages:  toOpenAIMessages(messages),
		Tools:     toOpenAITools(tools),
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
	httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)

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

	var apiResp openAIChatResponse
	if err := json.Unmarshal(body, &apiResp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	// Parse response content
	chatResp := &ChatResponse{
	}
	chatResp.Usage.InputTokens = apiResp.Usage.PromptTokens
	chatResp.Usage.OutputTokens = apiResp.Usage.CompletionTokens

	if len(apiResp.Choices) > 0 {
		msg := apiResp.Choices[0].Message
		chatResp.TextContent = parseContentText(msg.Content)
		for _, tc := range msg.ToolCalls {
			input := json.RawMessage(tc.Function.Arguments)
			chatResp.ToolCalls = append(chatResp.ToolCalls, ToolCall{
				ID:    tc.ID,
				Name:  tc.Function.Name,
				Input: input,
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
