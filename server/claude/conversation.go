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

// TrimToLastN keeps only the last N messages, ensuring tool_use/tool_result pairs are not broken
func (c *Conversation) TrimToLastN(n int) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if n >= len(c.Messages) {
		return
	}

	start := len(c.Messages) - n

	// Ensure we don't start with a message that has tool_result without preceding tool_use
	// Also ensure we don't cut off a tool_use without its tool_result
	for start > 0 && start < len(c.Messages) {
		msg := c.Messages[start]

		// If this message contains tool_result, we need to include the previous assistant message
		hasToolResult := false
		for _, block := range msg.Content {
			if block.Type == "tool_result" {
				hasToolResult = true
				break
			}
		}

		if hasToolResult && start > 0 {
			// Include the previous message (should be assistant with tool_use)
			start--
			continue
		}

		// If previous message has tool_use but this doesn't have tool_result, skip back
		if start > 0 {
			prevMsg := c.Messages[start-1]
			hasToolUse := false
			for _, block := range prevMsg.Content {
				if block.Type == "tool_use" {
					hasToolUse = true
					break
				}
			}
			if hasToolUse && !hasToolResult {
				start--
				continue
			}
		}

		break
	}

	c.Messages = c.Messages[start:]
	c.UpdatedAt = time.Now()
}

// MessageCount returns the number of messages
func (c *Conversation) MessageCount() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.Messages)
}

// ValidateAndClean ensures conversation is valid for Claude API
// Returns cleaned messages that maintain tool_use/tool_result pairs
func ValidateAndClean(messages []ConversationMessage) []ConversationMessage {
	if len(messages) == 0 {
		return messages
	}

	result := make([]ConversationMessage, 0, len(messages))

	for i := 0; i < len(messages); i++ {
		msg := messages[i]

		// Check if this message has tool_use
		hasToolUse := false
		for _, block := range msg.Content {
			if block.Type == "tool_use" {
				hasToolUse = true
				break
			}
		}

		// If has tool_use, ensure next message has tool_result
		if hasToolUse {
			if i+1 < len(messages) {
				nextMsg := messages[i+1]
				hasToolResult := false
				for _, block := range nextMsg.Content {
					if block.Type == "tool_result" {
						hasToolResult = true
						break
					}
				}
				if hasToolResult {
					// Both messages are valid
					result = append(result, msg)
				}
				// If no tool_result follows, skip this tool_use message
			}
			// If this is the last message and has tool_use, skip it
		} else {
			// Check if this message has tool_result
			hasToolResult := false
			for _, block := range msg.Content {
				if block.Type == "tool_result" {
					hasToolResult = true
					break
				}
			}

			if hasToolResult {
				// Only include if previous message in result has tool_use
				if len(result) > 0 {
					prevMsg := result[len(result)-1]
					prevHasToolUse := false
					for _, block := range prevMsg.Content {
						if block.Type == "tool_use" {
							prevHasToolUse = true
							break
						}
					}
					if prevHasToolUse {
						result = append(result, msg)
					}
					// Skip orphan tool_result
				}
			} else {
				// Regular message, just add it
				result = append(result, msg)
			}
		}
	}

	return result
}
