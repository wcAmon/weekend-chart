package handlers

import (
	"encoding/json"
	"log"
	"net/http"
	"time"
	"weekend-chart/server/claude"
	"weekend-chart/server/models"
	"weekend-chart/server/relay"

	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		return true // Allow all origins for now
	},
}

type WSMessage struct {
	Type string          `json:"type"`
	Data json.RawMessage `json:"data,omitempty"`
}

type AuthMessage struct {
	Token string `json:"token"`
}

type PairingCodeMessage struct {
	Code      string `json:"code"`
	ExpiresIn int    `json:"expires_in"`
}

type ConnectAgentMessage struct {
	AgentToken string `json:"agent_token"`
}

// Chat message types
type ChatMessageData struct {
	Message string `json:"message"`
}

type ChatResponse struct {
	Type       string       `json:"type"`
	Role       string       `json:"role"`
	Content    string       `json:"content"`
	Screenshot string       `json:"screenshot,omitempty"`
	Actions    []ActionInfo `json:"actions,omitempty"`
	IsError    bool         `json:"is_error,omitempty"`
}

type ActionInfo struct {
	Type        string `json:"type"`
	Description string `json:"description"`
	Success     bool   `json:"success"`
}

// ScreenshotData represents screenshot message from agent (flat structure)
type ScreenshotData struct {
	Type   string `json:"type"`
	Image  string `json:"image"`
	URL    string `json:"url"`
	Width  int    `json:"width"`
	Height int    `json:"height"`
}

// HandleAgentWS handles WebSocket connections from agents
func HandleAgentWS(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("Agent WS upgrade error: %v", err)
		return
	}

	// Wait for auth message
	conn.SetReadDeadline(time.Now().Add(10 * time.Second))
	_, msg, err := conn.ReadMessage()
	if err != nil {
		conn.Close()
		return
	}
	conn.SetReadDeadline(time.Time{})

	var wsMsg WSMessage
	if err := json.Unmarshal(msg, &wsMsg); err != nil {
		conn.Close()
		return
	}

	if wsMsg.Type != "auth" {
		conn.Close()
		return
	}

	var authMsg AuthMessage
	if err := json.Unmarshal(wsMsg.Data, &authMsg); err != nil || authMsg.Token == "" {
		conn.Close()
		return
	}

	// Register agent
	ac := relay.GlobalHub.RegisterAgent(authMsg.Token, conn)

	// Start read/write pumps
	go agentWritePump(ac)
	agentReadPump(ac)
}

func agentReadPump(ac *relay.AgentConn) {
	defer func() {
		relay.GlobalHub.UnregisterAgent(ac.Token)
		ac.Conn.Close()
	}()

	ac.Conn.SetReadLimit(10 * 1024 * 1024) // 10MB for screenshots
	ac.Conn.SetReadDeadline(time.Now().Add(60 * time.Second))
	ac.Conn.SetPongHandler(func(string) error {
		ac.Conn.SetReadDeadline(time.Now().Add(60 * time.Second))
		return nil
	})

	for {
		_, msg, err := ac.Conn.ReadMessage()
		if err != nil {
			break
		}

		var wsMsg WSMessage
		if err := json.Unmarshal(msg, &wsMsg); err != nil {
			continue
		}

		handleAgentMessage(ac, wsMsg, msg)
	}
}

func agentWritePump(ac *relay.AgentConn) {
	ticker := time.NewTicker(30 * time.Second)
	defer func() {
		ticker.Stop()
		ac.Conn.Close()
	}()

	for {
		select {
		case msg, ok := <-ac.Send:
			if !ok {
				ac.Conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}
			ac.Conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if err := ac.Conn.WriteMessage(websocket.TextMessage, msg); err != nil {
				return
			}
		case <-ticker.C:
			ac.Conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if err := ac.Conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

func handleAgentMessage(ac *relay.AgentConn, wsMsg WSMessage, rawMsg []byte) {
	switch wsMsg.Type {
	case "request_pairing_code":
		// Generate and store pairing code
		code := generatePairingCode()
		if err := models.CreatePairingCode(code, ac.Token); err != nil {
			log.Printf("Failed to create pairing code: %v", err)
			return
		}

		// Send code back to agent
		resp, _ := json.Marshal(WSMessage{
			Type: "pairing_code",
			Data: mustMarshal(PairingCodeMessage{
				Code:      code,
				ExpiresIn: 300,
			}),
		})
		ac.Send <- resp

	case "screenshot":
		// Cache the screenshot - agent sends flat structure, not nested in "data"
		var screenshotData ScreenshotData
		if err := json.Unmarshal(rawMsg, &screenshotData); err == nil && screenshotData.Image != "" {
			relay.GlobalHub.UpdateScreenshotCache(ac.Token, screenshotData.Image)
			log.Printf("Screenshot cached for agent %s (size: %d)", ac.Token[:10], len(screenshotData.Image))
		} else {
			log.Printf("Failed to parse screenshot from agent %s: %v", ac.Token[:10], err)
		}
		// Forward to connected user
		relay.GlobalHub.BroadcastToAgentUsers(ac.Token, rawMsg)

	case "dom_update":
		// Forward to connected user
		relay.GlobalHub.BroadcastToAgentUsers(ac.Token, rawMsg)

	case "page_state":
		// Cache the page state
		var pageStateMsg struct {
			Type  string          `json:"type"`
			State json.RawMessage `json:"state"`
		}
		if err := json.Unmarshal(rawMsg, &pageStateMsg); err == nil && pageStateMsg.State != nil {
			relay.GlobalHub.UpdatePageStateCache(ac.Token, pageStateMsg.State)
			log.Printf("Page state cached for agent %s", ac.Token[:10])
		}
	}
}

// HandleUserWS handles WebSocket connections from users
func HandleUserWS(w http.ResponseWriter, r *http.Request) {
	userID := GetUserID(r)
	if userID == 0 {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("User WS upgrade error: %v", err)
		return
	}

	uc := relay.GlobalHub.RegisterUser(userID, conn)

	go userWritePump(uc)
	userReadPump(uc)
}

func userReadPump(uc *relay.UserConn) {
	defer func() {
		relay.GlobalHub.UnregisterUser(uc)
		uc.Conn.Close()
	}()

	uc.Conn.SetReadLimit(64 * 1024)
	uc.Conn.SetReadDeadline(time.Now().Add(60 * time.Second))
	uc.Conn.SetPongHandler(func(string) error {
		uc.Conn.SetReadDeadline(time.Now().Add(60 * time.Second))
		return nil
	})

	for {
		_, msg, err := uc.Conn.ReadMessage()
		if err != nil {
			break
		}

		var wsMsg WSMessage
		if err := json.Unmarshal(msg, &wsMsg); err != nil {
			continue
		}

		handleUserMessage(uc, wsMsg, msg)
	}
}

func userWritePump(uc *relay.UserConn) {
	ticker := time.NewTicker(30 * time.Second)
	defer func() {
		ticker.Stop()
		uc.Conn.Close()
	}()

	for {
		select {
		case msg, ok := <-uc.Send:
			if !ok {
				uc.Conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}
			uc.Conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if err := uc.Conn.WriteMessage(websocket.TextMessage, msg); err != nil {
				return
			}
		case <-ticker.C:
			uc.Conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if err := uc.Conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

func handleUserMessage(uc *relay.UserConn, wsMsg WSMessage, rawMsg []byte) {
	switch wsMsg.Type {
	case "connect_agent":
		// User wants to connect to an agent
		var cam ConnectAgentMessage
		if err := json.Unmarshal(wsMsg.Data, &cam); err != nil {
			return
		}

		// Verify user owns this agent
		agent, err := models.GetAgentByToken(cam.AgentToken)
		if err != nil || agent.UserID != uc.UserID {
			sendError(uc, "Agent not found")
			return
		}

		// Set which agent user is viewing
		relay.GlobalHub.SetUserViewingAgent(uc.UserID, cam.AgentToken)

		// Check if agent is online
		online := relay.GlobalHub.IsAgentOnline(cam.AgentToken)
		resp, _ := json.Marshal(map[string]interface{}{
			"type":   "agent_status",
			"online": online,
		})
		uc.Send <- resp

	case "navigate", "click", "click_xy", "input", "key", "request_screenshot":
		// Forward to agent
		agentToken := relay.GlobalHub.GetUserViewingAgent(uc.UserID)
		if agentToken == "" {
			log.Printf("User %d: No agent selected", uc.UserID)
			return
		}
		log.Printf("User %d -> Agent %s: %s", uc.UserID, agentToken[:10], wsMsg.Type)
		if !relay.GlobalHub.SendToAgent(agentToken, rawMsg) {
			log.Printf("Failed to send to agent %s", agentToken[:10])
		}

	case "chat_message":
		// Handle chat message with Claude
		var chatData ChatMessageData
		if err := json.Unmarshal(wsMsg.Data, &chatData); err != nil {
			sendError(uc, "Invalid chat message format")
			return
		}

		agentToken := relay.GlobalHub.GetUserViewingAgent(uc.UserID)
		if agentToken == "" {
			sendChatError(uc, "請先選擇一個 Agent")
			return
		}

		if !relay.GlobalHub.IsAgentOnline(agentToken) {
			sendChatError(uc, "Agent 離線中")
			return
		}

		// Process chat message in a goroutine to avoid blocking
		go handleChatMessage(uc, agentToken, chatData.Message)

	case "clear_conversation":
		// Clear conversation history
		agentToken := relay.GlobalHub.GetUserViewingAgent(uc.UserID)
		if agentToken != "" {
			claude.GlobalConversationManager.Delete(uc.UserID, agentToken)
			sendChatResponse(uc, "system", "對話已清除", "", nil)
		}
	}
}

func sendError(uc *relay.UserConn, msg string) {
	resp, _ := json.Marshal(map[string]string{
		"type":  "error",
		"error": msg,
	})
	uc.Send <- resp
}

func generatePairingCode() string {
	// Generate 6-digit code
	code := ""
	for i := 0; i < 6; i++ {
		code += string('0' + byte(time.Now().UnixNano()%10))
		time.Sleep(1 * time.Nanosecond)
	}
	return code
}

func mustMarshal(v interface{}) json.RawMessage {
	b, _ := json.Marshal(v)
	return b
}

// Chat message handling

func sendChatResponse(uc *relay.UserConn, role, content, screenshot string, actions []ActionInfo) {
	resp, _ := json.Marshal(ChatResponse{
		Type:       "chat_response",
		Role:       role,
		Content:    content,
		Screenshot: screenshot,
		Actions:    actions,
	})
	uc.Send <- resp
}

func sendChatError(uc *relay.UserConn, message string) {
	resp, _ := json.Marshal(ChatResponse{
		Type:    "chat_response",
		Role:    "system",
		Content: message,
		IsError: true,
	})
	uc.Send <- resp
}

// AgentProxy implements claude.AgentInterface for tool execution
type AgentProxy struct {
	agentToken string
	userConn   *relay.UserConn
}

func (ap *AgentProxy) RequestScreenshot() (string, error) {
	return relay.GlobalHub.RequestScreenshotSync(ap.agentToken, 15*time.Second)
}

func (ap *AgentProxy) RequestPageState() (string, error) {
	data, err := relay.GlobalHub.RequestPageStateSync(ap.agentToken, 10*time.Second)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func (ap *AgentProxy) SendAction(action claude.BrowserAction) error {
	// Build message in the format agent expects (flat structure)
	var msg []byte
	var err error

	switch action.Type {
	case "navigate":
		msg, err = json.Marshal(map[string]interface{}{
			"type": "navigate",
			"url":  action.URL,
		})
	case "click_xy":
		msg, err = json.Marshal(map[string]interface{}{
			"type": "click_xy",
			"x":    action.X,
			"y":    action.Y,
		})
	case "input":
		msg, err = json.Marshal(map[string]interface{}{
			"type":  "input",
			"value": action.Value,
		})
	case "key":
		msg, err = json.Marshal(map[string]interface{}{
			"type": "key",
			"key":  action.Key,
		})
	case "scroll":
		msg, err = json.Marshal(map[string]interface{}{
			"type":      "scroll",
			"direction": action.Direction,
			"amount":    action.Amount,
		})
	default:
		msg, err = json.Marshal(map[string]interface{}{
			"type": action.Type,
		})
	}

	if err != nil {
		return err
	}

	if !relay.GlobalHub.SendToAgent(ap.agentToken, msg) {
		return errAgentNotConnected
	}

	// Wait for the action to complete (increased for network latency)
	time.Sleep(1000 * time.Millisecond)
	return nil
}

var errAgentNotConnected = &agentError{"agent not connected"}

type agentError struct {
	msg string
}

func (e *agentError) Error() string {
	return e.msg
}

func handleChatMessage(uc *relay.UserConn, agentToken, message string) {
	log.Printf("Chat message from user %d: %s", uc.UserID, message)

	// Get or create conversation
	conv := claude.GlobalConversationManager.GetOrCreate(uc.UserID, agentToken)

	// Get current screenshot
	screenshot, _, hasScreenshot := relay.GlobalHub.GetCachedScreenshot(agentToken)
	if !hasScreenshot {
		// Try to request a fresh screenshot
		var err error
		screenshot, err = relay.GlobalHub.RequestScreenshotSync(agentToken, 5*time.Second)
		if err != nil {
			log.Printf("Failed to get screenshot: %v", err)
		}
	}

	// Create user message with screenshot
	var userMsg claude.ConversationMessage
	if screenshot != "" {
		userMsg = claude.CreateImageMessage("user", message, screenshot)
	} else {
		userMsg = claude.CreateTextMessage("user", message)
	}
	conv.AddMessage(userMsg)

	// Send screenshot to frontend
	if screenshot != "" {
		sendChatResponse(uc, "system", "", screenshot, nil)
	}

	// Create OpenAI client and call API
	client := claude.NewClient()
	tools := claude.GetBrowserTools()

	// Create agent proxy for tool execution
	agentProxy := &AgentProxy{
		agentToken: agentToken,
		userConn:   uc,
	}
	toolExecutor := claude.NewToolExecutor(agentProxy)

	// Limit conversation history to last 20 messages to avoid context overflow
	if conv.MessageCount() > 20 {
		conv.TrimToLastN(20)
	}

	// Loop until no more tool calls
	maxIterations := 10
	for i := 0; i < maxIterations; i++ {
		messages := conv.GetMessages()

		// Validate and clean messages to ensure tool_use/tool_result pairs are intact
		messages = claude.ValidateAndClean(messages)

		resp, err := client.Chat(messages, tools)
		if err != nil {
			log.Printf("OpenAI API error: %v", err)
			sendChatError(uc, "AI 服務發生錯誤: "+err.Error())
			return
		}

		// Send text response to user
		if resp.TextContent != "" {
			sendChatResponse(uc, "assistant", resp.TextContent, "", nil)
		}

		// Check if there are tool calls
		if len(resp.ToolCalls) == 0 {
			// No more tool calls, add assistant response and exit
			if resp.TextContent != "" {
				conv.AddMessage(claude.CreateTextMessage("assistant", resp.TextContent))
			}
			break
		}

		// Add assistant message with tool calls to conversation
		conv.AddMessage(claude.CreateAssistantToolUseMessage(resp.TextContent, resp.ToolCalls))

		// Execute tool calls
		results, actionDescs, newScreenshot, err := toolExecutor.ExecuteToolCalls(resp.ToolCalls)
		if err != nil {
			log.Printf("Tool execution error: %v", err)
			sendChatError(uc, "工具執行失敗: "+err.Error())
			return
		}

		// Convert action descriptions to ActionInfo
		var actions []ActionInfo
		for _, desc := range actionDescs {
			actions = append(actions, ActionInfo{
				Type:        "action",
				Description: desc,
				Success:     true,
			})
		}

		// Send action info to user
		if len(actions) > 0 {
			sendChatResponse(uc, "system", "", "", actions)
		}

		// If we got a new screenshot, send it to the user
		if newScreenshot != "" {
			sendChatResponse(uc, "system", "", newScreenshot, nil)
			// Also update the screenshot cache
			relay.GlobalHub.UpdateScreenshotCache(agentToken, newScreenshot)
		}

		// Add tool results to conversation
		conv.AddMessage(claude.CreateToolResultMessage(results))

		// If we executed actions, request a new screenshot to see the result
		hasNonScreenshotAction := false
		for _, tc := range resp.ToolCalls {
			if tc.Name != "take_screenshot" {
				hasNonScreenshotAction = true
				break
			}
		}

		// Get screenshot after actions and include it in the conversation for the model to see
		var screenshotForClaude string
		if hasNonScreenshotAction {
			time.Sleep(1000 * time.Millisecond) // Wait longer for action to complete visually
			screenshotForClaude, _ = agentProxy.RequestScreenshot()
		} else if newScreenshot != "" {
			// Use the screenshot from take_screenshot tool
			screenshotForClaude = newScreenshot
		}

		if screenshotForClaude != "" {
			sendChatResponse(uc, "system", "", screenshotForClaude, nil)
			relay.GlobalHub.UpdateScreenshotCache(agentToken, screenshotForClaude)
			// Add screenshot to conversation so Claude can see the result
			conv.AddMessage(claude.CreateImageMessage("user", "這是執行操作後的截圖", screenshotForClaude))
		}
	}

	log.Printf("Chat completed for user %d", uc.UserID)
}
