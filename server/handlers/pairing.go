package handlers

import (
	"encoding/json"
	"net/http"
	"weekend-chart/server/models"
	"weekend-chart/server/relay"
)

type PairRequest struct {
	Code string `json:"code"`
	Name string `json:"name,omitempty"`
}

type PairResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message,omitempty"`
}

type AgentInfo struct {
	ID       int64  `json:"id"`
	Token    string `json:"token"`
	Name     string `json:"name"`
	Online   bool   `json:"online"`
	LastSeen string `json:"last_seen,omitempty"`
}

func HandlePair(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	userID := GetUserID(r)
	if userID == 0 {
		sendJSON(w, PairResponse{Success: false, Message: "Unauthorized"})
		return
	}

	var req PairRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		sendJSON(w, PairResponse{Success: false, Message: "Invalid request"})
		return
	}

	// Validate pairing code
	agentToken, err := models.ValidatePairingCode(req.Code)
	if err != nil {
		sendJSON(w, PairResponse{Success: false, Message: "Invalid or expired pairing code"})
		return
	}

	// Set agent name
	name := req.Name
	if name == "" {
		name = "My Computer"
	}

	// Pair agent to user
	if err := models.PairAgent(userID, agentToken, name); err != nil {
		sendJSON(w, PairResponse{Success: false, Message: "Failed to pair agent"})
		return
	}

	// Delete used pairing code
	models.DeletePairingCode(req.Code)

	// Update agent's user ID in relay hub
	relay.GlobalHub.UpdateAgentUserID(agentToken, userID)

	// Notify agent that it's paired
	notifyMsg, _ := json.Marshal(map[string]interface{}{
		"type":    "paired",
		"user_id": userID,
	})
	relay.GlobalHub.SendToAgent(agentToken, notifyMsg)

	sendJSON(w, PairResponse{Success: true})
}

func HandleAgents(w http.ResponseWriter, r *http.Request) {
	userID := GetUserID(r)
	if userID == 0 {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	switch r.Method {
	case http.MethodGet:
		// List agents
		agents, err := models.GetUserAgents(userID)
		if err != nil {
			sendJSON(w, []AgentInfo{})
			return
		}

		var infos []AgentInfo
		for _, a := range agents {
			info := AgentInfo{
				ID:     a.ID,
				Token:  a.Token,
				Name:   a.Name,
				Online: relay.GlobalHub.IsAgentOnline(a.Token),
			}
			if !a.LastSeen.IsZero() {
				info.LastSeen = a.LastSeen.Format("2006-01-02 15:04:05")
			}
			infos = append(infos, info)
		}

		if infos == nil {
			infos = []AgentInfo{}
		}
		sendJSON(w, infos)

	case http.MethodDelete:
		// Delete agent
		var req struct {
			ID int64 `json:"id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			sendJSON(w, map[string]bool{"success": false})
			return
		}

		if err := models.DeleteAgent(userID, req.ID); err != nil {
			sendJSON(w, map[string]bool{"success": false})
			return
		}

		sendJSON(w, map[string]bool{"success": true})

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}
