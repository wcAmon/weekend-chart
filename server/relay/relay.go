package relay

import (
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"time"
	"weekend-chart/server/models"

	"github.com/gorilla/websocket"
)

type Message struct {
	Type string          `json:"type"`
	Data json.RawMessage `json:"data,omitempty"`

	// For internal routing
	AgentToken string `json:"-"`
	UserID     int64  `json:"-"`
}

type Hub struct {
	// Agent connections (key: agent_token)
	agents map[string]*AgentConn

	// User connections (key: user_id, value: map of connections)
	users map[int64]map[*UserConn]bool

	// Which agent each user is viewing
	userViewingAgent map[int64]string

	// Screenshot cache (key: agent_token)
	screenshotCache map[string]*ScreenshotCache

	// Screenshot request channels (key: request_id)
	screenshotRequests map[string]chan string

	mu sync.RWMutex
}

// ScreenshotCache stores the latest screenshot for an agent
type ScreenshotCache struct {
	Data      string
	UpdatedAt time.Time
}

type AgentConn struct {
	Token  string
	UserID int64
	Conn   *websocket.Conn
	Send   chan []byte
}

type UserConn struct {
	UserID int64
	Conn   *websocket.Conn
	Send   chan []byte
}

var GlobalHub = &Hub{
	agents:             make(map[string]*AgentConn),
	users:              make(map[int64]map[*UserConn]bool),
	userViewingAgent:   make(map[int64]string),
	screenshotCache:    make(map[string]*ScreenshotCache),
	screenshotRequests: make(map[string]chan string),
}

// Agent methods
func (h *Hub) RegisterAgent(token string, conn *websocket.Conn) *AgentConn {
	h.mu.Lock()
	defer h.mu.Unlock()

	// Check if agent is already paired
	agent, err := models.GetAgentByToken(token)
	var userID int64
	if err == nil && agent != nil {
		userID = agent.UserID
	}

	ac := &AgentConn{
		Token:  token,
		UserID: userID,
		Conn:   conn,
		Send:   make(chan []byte, 256),
	}
	h.agents[token] = ac

	log.Printf("Agent registered: %s (user: %d)", token, userID)
	return ac
}

func (h *Hub) UnregisterAgent(token string) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if ac, ok := h.agents[token]; ok {
		close(ac.Send)
		delete(h.agents, token)
		log.Printf("Agent unregistered: %s", token)
	}
}

func (h *Hub) IsAgentOnline(token string) bool {
	h.mu.RLock()
	defer h.mu.RUnlock()
	_, ok := h.agents[token]
	return ok
}

func (h *Hub) UpdateAgentUserID(token string, userID int64) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if ac, ok := h.agents[token]; ok {
		ac.UserID = userID
	}
}

// User methods
func (h *Hub) RegisterUser(userID int64, conn *websocket.Conn) *UserConn {
	h.mu.Lock()
	defer h.mu.Unlock()

	uc := &UserConn{
		UserID: userID,
		Conn:   conn,
		Send:   make(chan []byte, 256),
	}

	if h.users[userID] == nil {
		h.users[userID] = make(map[*UserConn]bool)
	}
	h.users[userID][uc] = true

	log.Printf("User connected: %d", userID)
	return uc
}

func (h *Hub) UnregisterUser(uc *UserConn) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if conns, ok := h.users[uc.UserID]; ok {
		if _, ok := conns[uc]; ok {
			close(uc.Send)
			delete(conns, uc)
			if len(conns) == 0 {
				delete(h.users, uc.UserID)
				delete(h.userViewingAgent, uc.UserID)
			}
		}
	}
	log.Printf("User disconnected: %d", uc.UserID)
}

func (h *Hub) SetUserViewingAgent(userID int64, agentToken string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.userViewingAgent[userID] = agentToken
}

func (h *Hub) GetUserViewingAgent(userID int64) string {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.userViewingAgent[userID]
}

// Message routing
func (h *Hub) SendToAgent(agentToken string, msg []byte) bool {
	h.mu.RLock()
	defer h.mu.RUnlock()

	if ac, ok := h.agents[agentToken]; ok {
		select {
		case ac.Send <- msg:
			return true
		default:
			return false
		}
	}
	return false
}

func (h *Hub) SendToUser(userID int64, msg []byte) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	if conns, ok := h.users[userID]; ok {
		for uc := range conns {
			select {
			case uc.Send <- msg:
			default:
			}
		}
	}
}

func (h *Hub) BroadcastToAgentUsers(agentToken string, msg []byte) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	// Find which user owns this agent
	if ac, ok := h.agents[agentToken]; ok && ac.UserID > 0 {
		// Check if user is viewing this agent
		if viewingAgent := h.userViewingAgent[ac.UserID]; viewingAgent == agentToken {
			if conns, ok := h.users[ac.UserID]; ok {
				for uc := range conns {
					select {
					case uc.Send <- msg:
					default:
					}
				}
			}
		}
	}
}

// Heartbeat
func (h *Hub) StartHeartbeat() {
	ticker := time.NewTicker(30 * time.Second)
	go func() {
		for range ticker.C {
			h.mu.RLock()
			for token := range h.agents {
				models.UpdateAgentLastSeen(token)
			}
			h.mu.RUnlock()
		}
	}()
}

// Screenshot cache methods

// UpdateScreenshotCache updates the cached screenshot for an agent
func (h *Hub) UpdateScreenshotCache(agentToken string, data string) {
	h.mu.Lock()
	defer h.mu.Unlock()

	h.screenshotCache[agentToken] = &ScreenshotCache{
		Data:      data,
		UpdatedAt: time.Now(),
	}

	// Check if there's a pending request for this agent
	prefix := agentToken + ":"
	for reqID, ch := range h.screenshotRequests {
		// Request ID format: "agentToken:timestamp"
		if len(reqID) >= len(prefix) && reqID[:len(prefix)] == prefix {
			select {
			case ch <- data:
			default:
			}
		}
	}
}

// GetCachedScreenshot returns the cached screenshot for an agent
func (h *Hub) GetCachedScreenshot(agentToken string) (string, time.Time, bool) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	if cache, ok := h.screenshotCache[agentToken]; ok {
		return cache.Data, cache.UpdatedAt, true
	}
	return "", time.Time{}, false
}

// RequestScreenshotSync requests a screenshot and waits for the response
func (h *Hub) RequestScreenshotSync(agentToken string, timeout time.Duration) (string, error) {
	// First check if we have a recent cached screenshot (within 3 seconds)
	if data, updatedAt, ok := h.GetCachedScreenshot(agentToken); ok {
		if time.Since(updatedAt) < 3*time.Second {
			return data, nil
		}
	}

	// Create a unique request ID
	reqID := fmt.Sprintf("%s:%d", agentToken, time.Now().UnixNano())
	respChan := make(chan string, 1)

	// Register the request
	h.mu.Lock()
	h.screenshotRequests[reqID] = respChan
	h.mu.Unlock()

	// Cleanup on exit
	defer func() {
		h.mu.Lock()
		delete(h.screenshotRequests, reqID)
		h.mu.Unlock()
	}()

	// Send screenshot request to agent
	reqMsg, _ := json.Marshal(map[string]string{"type": "request_screenshot"})
	if !h.SendToAgent(agentToken, reqMsg) {
		return "", fmt.Errorf("agent not connected")
	}

	// Wait for response with timeout
	select {
	case screenshot := <-respChan:
		return screenshot, nil
	case <-time.After(timeout):
		// Try to return cached screenshot if available
		if data, _, ok := h.GetCachedScreenshot(agentToken); ok {
			return data, nil
		}
		return "", fmt.Errorf("screenshot request timed out")
	}
}

// ClearAgentScreenshotCache clears the screenshot cache for an agent
func (h *Hub) ClearAgentScreenshotCache(agentToken string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	delete(h.screenshotCache, agentToken)
}
