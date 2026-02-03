package handlers

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"time"
	"weekend-chart/server/models"
)

type LoginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type LoginResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message,omitempty"`
}

func HandleLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req LoginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		sendJSON(w, LoginResponse{Success: false, Message: "Invalid request"})
		return
	}

	userID, err := models.ValidateUser(req.Username, req.Password)
	if err != nil {
		sendJSON(w, LoginResponse{Success: false, Message: "Invalid username or password"})
		return
	}

	// Generate session token
	token := generateToken(32)
	if err := models.CreateSession(userID, token); err != nil {
		sendJSON(w, LoginResponse{Success: false, Message: "Failed to create session"})
		return
	}

	// Set cookie
	http.SetCookie(w, &http.Cookie{
		Name:     "session",
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteStrictMode,
		MaxAge:   7 * 24 * 60 * 60, // 7 days
	})

	sendJSON(w, LoginResponse{Success: true})
}

func HandleLogout(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie("session")
	if err == nil {
		models.DeleteSession(cookie.Value)
	}

	http.SetCookie(w, &http.Cookie{
		Name:     "session",
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		Secure:   true,
		MaxAge:   -1,
	})

	sendJSON(w, map[string]bool{"success": true})
}

func HandleCheckAuth(w http.ResponseWriter, r *http.Request) {
	userID := GetUserID(r)
	sendJSON(w, map[string]interface{}{
		"authenticated": userID > 0,
	})
}

func GetUserID(r *http.Request) int64 {
	cookie, err := r.Cookie("session")
	if err != nil {
		return 0
	}

	userID, err := models.ValidateSession(cookie.Value)
	if err != nil {
		return 0
	}

	return userID
}

func RequireAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if GetUserID(r) == 0 {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		next(w, r)
	}
}

func generateToken(length int) string {
	bytes := make([]byte, length)
	rand.Read(bytes)
	return hex.EncodeToString(bytes)
}

func sendJSON(w http.ResponseWriter, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(data)
}

func init() {
	// Prevent unused import error for time
	_ = time.Now
}
