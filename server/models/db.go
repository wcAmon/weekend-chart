package models

import (
	"database/sql"
	"log"
	"time"

	_ "github.com/mattn/go-sqlite3"
	"golang.org/x/crypto/bcrypt"
)

var DB *sql.DB

func InitDB(dbPath string) error {
	var err error
	DB, err = sql.Open("sqlite3", dbPath)
	if err != nil {
		return err
	}

	// Create tables
	schema := `
	CREATE TABLE IF NOT EXISTS users (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		username TEXT UNIQUE NOT NULL,
		password_hash TEXT NOT NULL,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS agents (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		user_id INTEGER NOT NULL,
		agent_token TEXT UNIQUE NOT NULL,
		name TEXT DEFAULT 'My Computer',
		last_seen DATETIME,
		paired_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		FOREIGN KEY (user_id) REFERENCES users(id)
	);

	CREATE TABLE IF NOT EXISTS pairing_codes (
		code TEXT PRIMARY KEY,
		agent_token TEXT UNIQUE NOT NULL,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		expires_at DATETIME
	);

	CREATE TABLE IF NOT EXISTS sessions (
		token TEXT PRIMARY KEY,
		user_id INTEGER NOT NULL,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		expires_at DATETIME,
		FOREIGN KEY (user_id) REFERENCES users(id)
	);
	`

	_, err = DB.Exec(schema)
	if err != nil {
		return err
	}

	// Create default user if not exists
	err = createDefaultUser("wake", "721225")
	if err != nil {
		log.Printf("Note: %v", err)
	}

	// Clean up expired pairing codes
	go cleanupExpiredCodes()

	return nil
}

func createDefaultUser(username, password string) error {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), 14)
	if err != nil {
		return err
	}

	_, err = DB.Exec(
		"INSERT OR IGNORE INTO users (username, password_hash) VALUES (?, ?)",
		username, string(hash),
	)
	return err
}

func cleanupExpiredCodes() {
	ticker := time.NewTicker(1 * time.Minute)
	for range ticker.C {
		DB.Exec("DELETE FROM pairing_codes WHERE expires_at < datetime('now')")
	}
}

// User functions
func ValidateUser(username, password string) (int64, error) {
	var id int64
	var hash string
	err := DB.QueryRow(
		"SELECT id, password_hash FROM users WHERE username = ?",
		username,
	).Scan(&id, &hash)
	if err != nil {
		return 0, err
	}

	err = bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
	if err != nil {
		return 0, err
	}

	return id, nil
}

// Session functions
func CreateSession(userID int64, token string) error {
	expiresAt := time.Now().Add(7 * 24 * time.Hour)
	_, err := DB.Exec(
		"INSERT INTO sessions (token, user_id, expires_at) VALUES (?, ?, ?)",
		token, userID, expiresAt,
	)
	return err
}

func ValidateSession(token string) (int64, error) {
	var userID int64
	err := DB.QueryRow(
		"SELECT user_id FROM sessions WHERE token = ? AND expires_at > datetime('now')",
		token,
	).Scan(&userID)
	return userID, err
}

func DeleteSession(token string) error {
	_, err := DB.Exec("DELETE FROM sessions WHERE token = ?", token)
	return err
}

// Agent functions
func CreatePairingCode(code, agentToken string) error {
	expiresAt := time.Now().Add(5 * time.Minute)
	_, err := DB.Exec(
		"INSERT OR REPLACE INTO pairing_codes (code, agent_token, expires_at) VALUES (?, ?, ?)",
		code, agentToken, expiresAt,
	)
	return err
}

func ValidatePairingCode(code string) (string, error) {
	var agentToken string
	err := DB.QueryRow(
		"SELECT agent_token FROM pairing_codes WHERE code = ? AND expires_at > datetime('now')",
		code,
	).Scan(&agentToken)
	return agentToken, err
}

func DeletePairingCode(code string) error {
	_, err := DB.Exec("DELETE FROM pairing_codes WHERE code = ?", code)
	return err
}

func PairAgent(userID int64, agentToken, name string) error {
	_, err := DB.Exec(
		"INSERT INTO agents (user_id, agent_token, name, last_seen) VALUES (?, ?, ?, datetime('now'))",
		userID, agentToken, name,
	)
	return err
}

func GetUserAgents(userID int64) ([]Agent, error) {
	rows, err := DB.Query(
		"SELECT id, agent_token, name, last_seen FROM agents WHERE user_id = ?",
		userID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var agents []Agent
	for rows.Next() {
		var a Agent
		var lastSeen sql.NullTime
		err := rows.Scan(&a.ID, &a.Token, &a.Name, &lastSeen)
		if err != nil {
			continue
		}
		if lastSeen.Valid {
			a.LastSeen = lastSeen.Time
		}
		agents = append(agents, a)
	}
	return agents, nil
}

func GetAgentByToken(token string) (*Agent, error) {
	var a Agent
	var lastSeen sql.NullTime
	var userID int64
	err := DB.QueryRow(
		"SELECT id, user_id, agent_token, name, last_seen FROM agents WHERE agent_token = ?",
		token,
	).Scan(&a.ID, &userID, &a.Token, &a.Name, &lastSeen)
	if err != nil {
		return nil, err
	}
	a.UserID = userID
	if lastSeen.Valid {
		a.LastSeen = lastSeen.Time
	}
	return &a, nil
}

func UpdateAgentLastSeen(token string) error {
	_, err := DB.Exec(
		"UPDATE agents SET last_seen = datetime('now') WHERE agent_token = ?",
		token,
	)
	return err
}

func DeleteAgent(userID int64, agentID int64) error {
	_, err := DB.Exec(
		"DELETE FROM agents WHERE id = ? AND user_id = ?",
		agentID, userID,
	)
	return err
}

type Agent struct {
	ID       int64
	UserID   int64
	Token    string
	Name     string
	LastSeen time.Time
}
