package claude

import (
	"fmt"
	"sync"
	"time"
)

// Conversation represents a chat conversation
type Conversation struct {
	ID         string
	UserID     int64
	AgentToken string
	Messages   []ConversationMessage
	CreatedAt  time.Time
	UpdatedAt  time.Time
	mu         sync.Mutex
}

// ConversationManager manages multiple conversations
type ConversationManager struct {
	conversations map[string]*Conversation
	mu            sync.RWMutex
}

// NewConversationManager creates a new conversation manager
func NewConversationManager() *ConversationManager {
	return &ConversationManager{
		conversations: make(map[string]*Conversation),
	}
}

// GlobalConversationManager is the global conversation manager
var GlobalConversationManager = NewConversationManager()

// getConversationID generates a unique conversation ID
func getConversationID(userID int64, agentToken string) string {
	return fmt.Sprintf("%d:%s", userID, agentToken)
}

// GetOrCreate gets an existing conversation or creates a new one
func (m *ConversationManager) GetOrCreate(userID int64, agentToken string) *Conversation {
	id := getConversationID(userID, agentToken)

	m.mu.RLock()
	if conv, ok := m.conversations[id]; ok {
		m.mu.RUnlock()
		return conv
	}
	m.mu.RUnlock()

	// Create new conversation
	m.mu.Lock()
	defer m.mu.Unlock()

	// Double-check after acquiring write lock
	if conv, ok := m.conversations[id]; ok {
		return conv
	}

	conv := &Conversation{
		ID:         id,
		UserID:     userID,
		AgentToken: agentToken,
		Messages:   []ConversationMessage{},
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
	}
	m.conversations[id] = conv
	return conv
}

// Get retrieves a conversation by ID
func (m *ConversationManager) Get(userID int64, agentToken string) *Conversation {
	id := getConversationID(userID, agentToken)

	m.mu.RLock()
	defer m.mu.RUnlock()

	return m.conversations[id]
}

// Delete removes a conversation
func (m *ConversationManager) Delete(userID int64, agentToken string) {
	id := getConversationID(userID, agentToken)

	m.mu.Lock()
	defer m.mu.Unlock()

	delete(m.conversations, id)
}

// Clear removes all conversations for a user
func (m *ConversationManager) ClearForUser(userID int64) {
	m.mu.Lock()
	defer m.mu.Unlock()

	for id, conv := range m.conversations {
		if conv.UserID == userID {
			delete(m.conversations, id)
		}
	}
}

// AddMessage adds a message to a conversation
func (c *Conversation) AddMessage(msg ConversationMessage) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.Messages = append(c.Messages, msg)
	c.UpdatedAt = time.Now()
}

// GetMessages returns all messages in the conversation
func (c *Conversation) GetMessages() []ConversationMessage {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Return a copy to avoid race conditions
	messages := make([]ConversationMessage, len(c.Messages))
	copy(messages, c.Messages)
	return messages
}

// GetLastN returns the last N messages
func (c *Conversation) GetLastN(n int) []ConversationMessage {
	c.mu.Lock()
	defer c.mu.Unlock()

	if n >= len(c.Messages) {
		messages := make([]ConversationMessage, len(c.Messages))
		copy(messages, c.Messages)
		return messages
	}

	start := len(c.Messages) - n
	messages := make([]ConversationMessage, n)
	copy(messages, c.Messages[start:])
	return messages
}

// Clear removes all messages from the conversation
func (c *Conversation) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.Messages = []ConversationMessage{}
	c.UpdatedAt = time.Now()
}

// TrimToLastN keeps only the last N messages
func (c *Conversation) TrimToLastN(n int) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if n >= len(c.Messages) {
		return
	}

	start := len(c.Messages) - n
	c.Messages = c.Messages[start:]
	c.UpdatedAt = time.Now()
}

// MessageCount returns the number of messages
func (c *Conversation) MessageCount() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.Messages)
}
