package main

import (
	"log"
	"net/http"
	"os"
	"path/filepath"

	"weekend-chart/server/claude"
	"weekend-chart/server/handlers"
	"weekend-chart/server/models"
	"weekend-chart/server/relay"

	"github.com/joho/godotenv"
)

func main() {
	// Get working directory
	execPath, _ := os.Executable()
	workDir := filepath.Dir(execPath)

	// Load environment variables from .env file (in project root)
	envPath := filepath.Join(workDir, "..", ".env")
	if err := godotenv.Load(envPath); err != nil {
		// Try current directory
		if err := godotenv.Load(); err != nil {
			log.Printf("Warning: Could not load .env file: %v", err)
		}
	}

	// Verify Anthropic API key is set
	claudeClient := claude.NewClient()
	log.Printf("Anthropic API key status: %s", claudeClient.GetAPIKey())

	// Initialize database
	dbPath := filepath.Join(workDir, "..", "data", "weekend-chart.db")
	if err := os.MkdirAll(filepath.Dir(dbPath), 0755); err != nil {
		log.Fatalf("Failed to create data directory: %v", err)
	}

	if err := models.InitDB(dbPath); err != nil {
		log.Fatalf("Failed to initialize database: %v", err)
	}
	log.Printf("Database initialized at %s", dbPath)

	// Start heartbeat
	relay.GlobalHub.StartHeartbeat()

	// API routes
	http.HandleFunc("/api/login", handlers.HandleLogin)
	http.HandleFunc("/api/logout", handlers.HandleLogout)
	http.HandleFunc("/api/check-auth", handlers.HandleCheckAuth)
	http.HandleFunc("/api/pair", handlers.RequireAuth(handlers.HandlePair))
	http.HandleFunc("/api/agents", handlers.HandleAgents)

	// WebSocket routes
	http.HandleFunc("/ws/agent", handlers.HandleAgentWS)
	http.HandleFunc("/ws/user", handlers.HandleUserWS)

	// Static files
	staticDir := filepath.Join(workDir, "static")
	fs := http.FileServer(http.Dir(staticDir))
	http.Handle("/", fs)

	// Start server
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	log.Printf("Server starting on port %s", port)
	log.Printf("Static files served from %s", staticDir)

	if err := http.ListenAndServe(":"+port, nil); err != nil {
		log.Fatalf("Server failed: %v", err)
	}
}
