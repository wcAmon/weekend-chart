package claude

import (
	"encoding/json"
	"fmt"
)

// GetBrowserTools returns the tool definitions for browser control
func GetBrowserTools() []Tool {
	return []Tool{
		{
			Name:        "take_screenshot",
			Description: "截取當前瀏覽器畫面",
			InputSchema: json.RawMessage(`{
				"type": "object",
				"properties": {},
				"required": []
			}`),
		},
		{
			Name:        "click",
			Description: "點擊螢幕上的指定位置",
			InputSchema: json.RawMessage(`{
				"type": "object",
				"properties": {
					"x": {
						"type": "integer",
						"description": "X 座標 (0-1920)"
					},
					"y": {
						"type": "integer",
						"description": "Y 座標 (0-1080)"
					},
					"description": {
						"type": "string",
						"description": "點擊目標的描述"
					}
				},
				"required": ["x", "y", "description"]
			}`),
		},
		{
			Name:        "type_text",
			Description: "在當前焦點位置輸入文字",
			InputSchema: json.RawMessage(`{
				"type": "object",
				"properties": {
					"text": {
						"type": "string",
						"description": "要輸入的文字"
					}
				},
				"required": ["text"]
			}`),
		},
		{
			Name:        "press_key",
			Description: "按下鍵盤按鍵",
			InputSchema: json.RawMessage(`{
				"type": "object",
				"properties": {
					"key": {
						"type": "string",
						"description": "按鍵名稱 (Enter, Tab, Escape 等)"
					}
				},
				"required": ["key"]
			}`),
		},
		{
			Name:        "navigate",
			Description: "導航到指定網址",
			InputSchema: json.RawMessage(`{
				"type": "object",
				"properties": {
					"url": {
						"type": "string",
						"description": "目標網址"
					}
				},
				"required": ["url"]
			}`),
		},
		{
			Name:        "scroll",
			Description: "滾動頁面",
			InputSchema: json.RawMessage(`{
				"type": "object",
				"properties": {
					"direction": {
						"type": "string",
						"enum": ["up", "down"],
						"description": "滾動方向"
					},
					"amount": {
						"type": "integer",
						"description": "滾動像素數 (預設 500)"
					}
				},
				"required": ["direction"]
			}`),
		},
		{
			Name:        "select_all",
			Description: "全選當前焦點輸入框的內容 (Ctrl+A)，常用於清除輸入框：先 select_all 再 press_key Backspace",
			InputSchema: json.RawMessage(`{
				"type": "object",
				"properties": {},
				"required": []
			}`),
		},
		{
			Name:        "get_page_state",
			Description: "取得頁面狀態（表單欄位、按鈕、連結等），比截圖更快更準確。會返回所有輸入框的目前值、座標、focus狀態",
			InputSchema: json.RawMessage(`{
				"type": "object",
				"properties": {},
				"required": []
			}`),
		},
	}
}

// ClickInput represents the input for a click action
type ClickInput struct {
	X           int    `json:"x"`
	Y           int    `json:"y"`
	Description string `json:"description"`
}

// TypeTextInput represents the input for a type_text action
type TypeTextInput struct {
	Text string `json:"text"`
}

// PressKeyInput represents the input for a press_key action
type PressKeyInput struct {
	Key string `json:"key"`
}

// NavigateInput represents the input for a navigate action
type NavigateInput struct {
	URL string `json:"url"`
}

// ScrollInput represents the input for a scroll action
type ScrollInput struct {
	Direction string `json:"direction"`
	Amount    int    `json:"amount"`
}

// BrowserAction represents an action to be sent to the agent
type BrowserAction struct {
	Type        string `json:"type"`
	Description string `json:"description,omitempty"`

	// For click_xy
	X int `json:"x,omitempty"`
	Y int `json:"y,omitempty"`

	// For input
	Value string `json:"value,omitempty"`

	// For key
	Key string `json:"key,omitempty"`

	// For navigate
	URL string `json:"url,omitempty"`

	// For scroll
	Direction string `json:"direction,omitempty"`
	Amount    int    `json:"amount,omitempty"`
}

// AgentInterface defines the interface for interacting with the agent
type AgentInterface interface {
	RequestScreenshot() (string, error)
	RequestPageState() (string, error)
	SendAction(action BrowserAction) error
}

// ToolExecutor handles the execution of Claude tools
type ToolExecutor struct {
	agent AgentInterface
}

// NewToolExecutor creates a new tool executor
func NewToolExecutor(agent AgentInterface) *ToolExecutor {
	return &ToolExecutor{agent: agent}
}

// ExecuteTool executes a single tool call and returns the result
func (te *ToolExecutor) ExecuteTool(toolCall ToolCall) (ToolResult, string, error) {
	result := ToolResult{
		ToolUseID: toolCall.ID,
	}

	var actionDescription string

	switch toolCall.Name {
	case "take_screenshot":
		screenshot, err := te.agent.RequestScreenshot()
		if err != nil {
			result.Content = fmt.Sprintf("截圖失敗: %v", err)
			result.IsError = true
		} else {
			result.Content = "截圖成功"
			// Note: The screenshot will be included as an image in the next message
			actionDescription = "截圖"
		}
		// Return screenshot as extra data
		return result, screenshot, nil

	case "click":
		var input ClickInput
		if err := json.Unmarshal(toolCall.Input, &input); err != nil {
			result.Content = fmt.Sprintf("解析點擊參數失敗: %v", err)
			result.IsError = true
			return result, "", nil
		}

		action := BrowserAction{
			Type:        "click_xy",
			X:           input.X,
			Y:           input.Y,
			Description: input.Description,
		}
		if err := te.agent.SendAction(action); err != nil {
			result.Content = fmt.Sprintf("點擊失敗: %v", err)
			result.IsError = true
		} else {
			result.Content = fmt.Sprintf("已點擊位置 (%d, %d): %s", input.X, input.Y, input.Description)
			actionDescription = fmt.Sprintf("點擊 %s", input.Description)
		}

	case "type_text":
		var input TypeTextInput
		if err := json.Unmarshal(toolCall.Input, &input); err != nil {
			result.Content = fmt.Sprintf("解析輸入參數失敗: %v", err)
			result.IsError = true
			return result, "", nil
		}

		// Safety: detect if AI mistakenly tried to type a key name
		keyNames := map[string]bool{
			"Tab": true, "tab": true, "TAB": true,
			"Enter": true, "enter": true, "ENTER": true,
			"Backspace": true, "backspace": true, "BACKSPACE": true,
			"Escape": true, "escape": true, "ESC": true, "esc": true,
			"Delete": true, "delete": true, "DEL": true, "del": true,
			"ArrowUp": true, "ArrowDown": true, "ArrowLeft": true, "ArrowRight": true,
		}

		if keyNames[input.Text] {
			// Convert to press_key action instead
			action := BrowserAction{
				Type: "key",
				Key:  input.Text,
			}
			if err := te.agent.SendAction(action); err != nil {
				result.Content = fmt.Sprintf("按鍵失敗: %v", err)
				result.IsError = true
			} else {
				result.Content = fmt.Sprintf("已按下按鍵: %s (自動轉換)", input.Text)
				actionDescription = fmt.Sprintf("按下 %s", input.Text)
			}
		} else {
			action := BrowserAction{
				Type:  "input",
				Value: input.Text,
			}
			if err := te.agent.SendAction(action); err != nil {
				result.Content = fmt.Sprintf("輸入失敗: %v", err)
				result.IsError = true
			} else {
				result.Content = fmt.Sprintf("已輸入文字: %s", input.Text)
				actionDescription = fmt.Sprintf("輸入 \"%s\"", input.Text)
			}
		}

	case "press_key":
		var input PressKeyInput
		if err := json.Unmarshal(toolCall.Input, &input); err != nil {
			result.Content = fmt.Sprintf("解析按鍵參數失敗: %v", err)
			result.IsError = true
			return result, "", nil
		}

		action := BrowserAction{
			Type: "key",
			Key:  input.Key,
		}
		if err := te.agent.SendAction(action); err != nil {
			result.Content = fmt.Sprintf("按鍵失敗: %v", err)
			result.IsError = true
		} else {
			result.Content = fmt.Sprintf("已按下按鍵: %s", input.Key)
			actionDescription = fmt.Sprintf("按下 %s", input.Key)
		}

	case "navigate":
		var input NavigateInput
		if err := json.Unmarshal(toolCall.Input, &input); err != nil {
			result.Content = fmt.Sprintf("解析導航參數失敗: %v", err)
			result.IsError = true
			return result, "", nil
		}

		action := BrowserAction{
			Type: "navigate",
			URL:  input.URL,
		}
		if err := te.agent.SendAction(action); err != nil {
			result.Content = fmt.Sprintf("導航失敗: %v", err)
			result.IsError = true
		} else {
			result.Content = fmt.Sprintf("已導航到: %s", input.URL)
			actionDescription = fmt.Sprintf("導航到 %s", input.URL)
		}

	case "scroll":
		var input ScrollInput
		if err := json.Unmarshal(toolCall.Input, &input); err != nil {
			result.Content = fmt.Sprintf("解析滾動參數失敗: %v", err)
			result.IsError = true
			return result, "", nil
		}

		if input.Amount == 0 {
			input.Amount = 500
		}

		action := BrowserAction{
			Type:      "scroll",
			Direction: input.Direction,
			Amount:    input.Amount,
		}
		if err := te.agent.SendAction(action); err != nil {
			result.Content = fmt.Sprintf("滾動失敗: %v", err)
			result.IsError = true
		} else {
			result.Content = fmt.Sprintf("已向%s滾動 %d 像素", input.Direction, input.Amount)
			actionDescription = fmt.Sprintf("向%s滾動", input.Direction)
		}

	case "select_all":
		action := BrowserAction{
			Type: "select_all",
		}
		if err := te.agent.SendAction(action); err != nil {
			result.Content = fmt.Sprintf("全選失敗: %v", err)
			result.IsError = true
		} else {
			result.Content = "已全選輸入框內容"
			actionDescription = "全選"
		}

	case "get_page_state":
		pageState, err := te.agent.RequestPageState()
		if err != nil {
			result.Content = fmt.Sprintf("取得頁面狀態失敗: %v", err)
			result.IsError = true
		} else {
			result.Content = pageState
			actionDescription = "取得頁面狀態"
		}

	default:
		result.Content = fmt.Sprintf("未知的工具: %s", toolCall.Name)
		result.IsError = true
	}

	return result, actionDescription, nil
}

// ExecuteToolCalls executes all tool calls and returns results
func (te *ToolExecutor) ExecuteToolCalls(toolCalls []ToolCall) ([]ToolResult, []string, string, error) {
	var results []ToolResult
	var actionDescriptions []string
	var lastScreenshot string

	for _, tc := range toolCalls {
		result, screenshot, err := te.ExecuteTool(tc)
		if err != nil {
			return nil, nil, "", err
		}
		results = append(results, result)

		if tc.Name == "take_screenshot" && screenshot != "" {
			lastScreenshot = screenshot
		}

		// Collect action descriptions for UI
		if result.Content != "" && !result.IsError {
			actionDescriptions = append(actionDescriptions, result.Content)
		}
	}

	return results, actionDescriptions, lastScreenshot, nil
}
