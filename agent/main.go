package main

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"sync"
	"syscall"
	"time"

	"weekend-chart/agent/browser"
	"weekend-chart/agent/config"
	"weekend-chart/agent/tray"

	"github.com/gorilla/websocket"
)

func init() {
	// Setup log file for debugging crashes
	exePath, _ := os.Executable()
	logPath := filepath.Join(filepath.Dir(exePath), "agent-error.log")
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err == nil {
		log.SetOutput(logFile)
		log.SetFlags(log.LstdFlags | log.Lshortfile)
		log.Println("=== Agent Started ===")
	}
}

const (
	ServerURL = "wss://wake.loader.land/ws/agent"
)

type Message struct {
	Type string `json:"type"`
	// Flat fields for different message types
	URL      string `json:"url,omitempty"`
	Selector string `json:"selector,omitempty"`
	X        int    `json:"x,omitempty"`
	Y        int    `json:"y,omitempty"`
	Value    string `json:"value,omitempty"`
	Key      string `json:"key,omitempty"`
	// For responses from server
	Data json.RawMessage `json:"data,omitempty"`
}

type AuthData struct {
	Token string `json:"token"`
}

var (
	cfg      *config.Config
	conn     *websocket.Conn
	connMu   sync.Mutex // Protects WebSocket writes
	chrome   *browser.Browser
	paired   bool
)

func main() {
	// Catch panics
	defer func() {
		if r := recover(); r != nil {
			tray.ShowConsole()
			fmt.Println()
			fmt.Println("╔═══════════════════════════════════════════╗")
			fmt.Println("║           程式發生錯誤                     ║")
			fmt.Println("╚═══════════════════════════════════════════╝")
			fmt.Printf("錯誤: %v\n", r)
			fmt.Println()
			fmt.Println("按 Enter 鍵關閉...")
			fmt.Scanln()
		}
	}()

	fmt.Println("╔═══════════════════════════════════════════╗")
	fmt.Println("║        Weekend Chart Agent                ║")
	fmt.Println("╚═══════════════════════════════════════════╝")
	fmt.Println()

	// Load or create config
	var err error
	cfg, err = config.Load()
	if err != nil {
		fmt.Println("首次執行，建立新設定...")
		// First run, generate new token
		cfg = &config.Config{
			ServerURL:  ServerURL,
			AgentToken: generateToken(),
			AgentName:  "My Computer",
		}
		if err := config.Save(cfg); err != nil {
			fmt.Printf("警告: 無法儲存設定: %v\n", err)
		}
		paired = false
	} else {
		// Always use current server URL
		cfg.ServerURL = ServerURL
		paired = true
		fmt.Printf("已載入設定，Token: %s\n", cfg.AgentToken[:10]+"...")
	}
	fmt.Println()

	// Start Chrome
	fmt.Println("正在啟動瀏覽器...")
	fmt.Println("(需要安裝 Google Chrome 或 Chromium)")
	fmt.Println()
	chrome, err = browser.New()
	if err != nil {
		fmt.Println("╔═══════════════════════════════════════════╗")
		fmt.Println("║              錯誤                          ║")
		fmt.Println("╠═══════════════════════════════════════════╣")
		fmt.Println("║  無法啟動瀏覽器！                          ║")
		fmt.Println("║                                           ║")
		fmt.Println("║  請確認已安裝 Google Chrome                ║")
		fmt.Println("║  下載: https://www.google.com/chrome       ║")
		fmt.Println("╚═══════════════════════════════════════════╝")
		fmt.Println()
		fmt.Printf("錯誤詳情: %v\n", err)
		fmt.Println()
		fmt.Println("按 Enter 鍵關閉...")
		fmt.Scanln()
		os.Exit(1)
	}
	fmt.Println("瀏覽器已啟動")
	fmt.Println()

	// Handle shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigChan
		cleanup()
		os.Exit(0)
	}()

	// On Windows, use system tray
	if runtime.GOOS == "windows" {
		fmt.Println("程式將縮小到系統列...")
		fmt.Println()
		time.Sleep(2 * time.Second)

		// Start system tray (this blocks on the main goroutine)
		tray.Start(onTrayReady, cleanup)
	} else {
		// On other platforms, run directly
		runAgent()
	}
}

func onTrayReady() {
	// Hide console window on Windows
	tray.HideConsole()
	tray.SetStatus("狀態: 連線中...")

	// Run agent in a goroutine
	go runAgent()
}

func cleanup() {
	fmt.Println("\n正在關閉...")
	if chrome != nil {
		chrome.Close()
	}
	if conn != nil {
		conn.Close()
	}
}

func runAgent() {
	// Connect to server with retry
	for {
		if err := connect(); err != nil {
			log.Printf("連線失敗: %v", err)
			tray.SetStatus("狀態: 連線失敗")
			fmt.Println("5 秒後重試...")
			time.Sleep(5 * time.Second)
			continue
		}
		handleMessages()
		tray.SetStatus("狀態: 已斷線")
		fmt.Println("連線中斷，重新連線...")
		time.Sleep(3 * time.Second)
	}
}

func connect() error {
	var err error
	conn, _, err = websocket.DefaultDialer.Dial(cfg.ServerURL, nil)
	if err != nil {
		return err
	}

	// Send auth
	authData, _ := json.Marshal(AuthData{Token: cfg.AgentToken})
	authMsg, _ := json.Marshal(Message{
		Type: "auth",
		Data: authData,
	})

	if err := conn.WriteMessage(websocket.TextMessage, authMsg); err != nil {
		return err
	}

	fmt.Println("已連接到伺服器")

	// If not paired, request pairing code
	if !paired {
		tray.SetStatus("狀態: 等待配對...")
		requestPairingCode()
	} else {
		tray.SetStatus("狀態: 已連線 ✓")
		fmt.Println("狀態：已配對 ✓")
		fmt.Println()
		fmt.Println("等待手機連線...")
	}

	return nil
}

func requestPairingCode() {
	msg, _ := json.Marshal(Message{Type: "request_pairing_code"})
	safeWriteMessage(websocket.TextMessage, msg)
}

func handleMessages() {
	// Start DOM watcher
	chrome.WatchDOMChanges(func(state *browser.PageState) {
		sendDOMUpdate(state)
	})

	for {
		_, data, err := conn.ReadMessage()
		if err != nil {
			return
		}

		var msg Message
		if err := json.Unmarshal(data, &msg); err != nil {
			continue
		}

		handleMessage(msg)
	}
}

func handleMessage(msg Message) {
	switch msg.Type {
	case "pairing_code":
		var code struct {
			Code      string `json:"code"`
			ExpiresIn int    `json:"expires_in"`
		}
		json.Unmarshal(msg.Data, &code)

		fmt.Println("╔═══════════════════════════════════════════╗")
		fmt.Println("║               配對碼                       ║")
		fmt.Println("╠═══════════════════════════════════════════╣")
		fmt.Printf("║                                           ║\n")
		fmt.Printf("║             %s                      ║\n", code.Code)
		fmt.Printf("║                                           ║\n")
		fmt.Println("║   請在手機上輸入此配對碼 (5 分鐘內有效)    ║")
		fmt.Println("╚═══════════════════════════════════════════╝")
		fmt.Println()

	case "paired":
		paired = true
		config.Save(cfg)
		fmt.Println("✓ 配對成功！")
		fmt.Println()

	case "navigate":
		log.Printf("導航至: %s", msg.URL)
		if msg.URL == "" {
			log.Printf("導航失敗: URL 為空")
			return
		}
		if err := chrome.Navigate(msg.URL); err != nil {
			log.Printf("導航失敗: %v", err)
		} else {
			log.Printf("導航成功，發送狀態...")
			sendCurrentState()
			log.Printf("狀態已發送")
		}

	case "click":
		log.Printf("點擊: %s", msg.Selector)
		if err := chrome.Click(msg.Selector); err != nil {
			log.Printf("點擊失敗: %v", err)
		}
		time.Sleep(500 * time.Millisecond)
		sendCurrentState()

	case "click_xy":
		log.Printf("點擊座標: (%d, %d)", msg.X, msg.Y)
		if err := chrome.ClickXY(msg.X, msg.Y); err != nil {
			log.Printf("點擊失敗: %v", err)
		}
		time.Sleep(500 * time.Millisecond)
		sendCurrentState()

	case "input":
		log.Printf("輸入: %s", msg.Value)
		var err error
		if msg.Selector != "" {
			err = chrome.Input(msg.Selector, msg.Value)
		} else {
			err = chrome.InputToFocused(msg.Value)
		}
		if err != nil {
			log.Printf("輸入失敗: %v", err)
		} else {
			log.Printf("輸入成功")
		}
		sendCurrentState()

	case "key":
		log.Printf("按鍵: %s", msg.Key)
		chrome.PressKey(msg.Key)
		time.Sleep(300 * time.Millisecond)
		sendCurrentState()

	case "select_all":
		log.Printf("全選")
		if err := chrome.SelectAll(); err != nil {
			log.Printf("全選失敗: %v", err)
		}
		time.Sleep(200 * time.Millisecond)
		sendCurrentState()

	case "get_page_state":
		log.Printf("取得頁面狀態")
		sendPageState()

	case "request_screenshot":
		sendCurrentState()
	}
}

func sendDOMUpdate(state *browser.PageState) {
	if conn == nil {
		return
	}

	// Send flat structure
	msg, _ := json.Marshal(map[string]interface{}{
		"type":  "dom_update",
		"url":   state.URL,
		"title": state.Title,
		"html":  state.HTML,
	})

	safeWriteMessage(websocket.TextMessage, msg)
}

func sendScreenshot() {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("sendScreenshot panic: %v", r)
		}
	}()

	if conn == nil || chrome == nil {
		log.Printf("sendScreenshot: conn 或 chrome 為 nil")
		return
	}

	log.Printf("sendScreenshot: 獲取截圖中...")
	ss, err := chrome.GetScreenshot()
	if err != nil {
		log.Printf("截圖失敗: %v", err)
		return
	}
	log.Printf("sendScreenshot: 截圖成功, URL=%s", ss.URL)

	// Send flat structure
	msg, err := json.Marshal(map[string]interface{}{
		"type":   "screenshot",
		"url":    ss.URL,
		"image":  ss.Image,
		"width":  ss.Width,
		"height": ss.Height,
	})
	if err != nil {
		log.Printf("JSON 序列化失敗: %v", err)
		return
	}

	log.Printf("sendScreenshot: 發送中... (size=%d)", len(msg))
	if err := safeWriteMessage(websocket.TextMessage, msg); err != nil {
		log.Printf("sendScreenshot: 發送失敗: %v", err)
	} else {
		log.Printf("sendScreenshot: 發送成功")
	}
}

func sendCurrentState() {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("sendCurrentState panic: %v", r)
		}
	}()

	log.Printf("sendCurrentState: 開始")
	// Send screenshot only (DOM can be too large)
	log.Printf("sendCurrentState: 準備截圖")
	sendScreenshot()
	log.Printf("sendCurrentState: 完成")
}

func sendPageState() {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("sendPageState panic: %v", r)
		}
	}()

	if conn == nil || chrome == nil {
		log.Printf("sendPageState: conn 或 chrome 為 nil")
		return
	}

	state, err := chrome.GetSimplifiedPageState()
	if err != nil {
		log.Printf("取得頁面狀態失敗: %v", err)
		return
	}

	msg, err := json.Marshal(map[string]interface{}{
		"type":  "page_state",
		"state": state,
	})
	if err != nil {
		log.Printf("JSON 序列化失敗: %v", err)
		return
	}

	log.Printf("sendPageState: 發送中... (size=%d)", len(msg))
	if err := safeWriteMessage(websocket.TextMessage, msg); err != nil {
		log.Printf("sendPageState: 發送失敗: %v", err)
	} else {
		log.Printf("sendPageState: 發送成功")
	}
}

func generateToken() string {
	b := make([]byte, 16)
	rand.Read(b)
	return "agent_" + hex.EncodeToString(b)
}

// safeWriteMessage writes to WebSocket with mutex protection
func safeWriteMessage(messageType int, data []byte) error {
	connMu.Lock()
	defer connMu.Unlock()
	if conn == nil {
		return fmt.Errorf("connection is nil")
	}
	return conn.WriteMessage(messageType, data)
}
