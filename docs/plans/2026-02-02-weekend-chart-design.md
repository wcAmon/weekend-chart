# Weekend Chart 設計文件

> 遠端瀏覽器控制 Web App - 從手機控制辦公室電腦的瀏覽器

## 概述

Weekend Chart 讓你從手機遠端控制辦公室 Windows 電腦上的瀏覽器，透過 bookbark.io 伺服器中繼連接。

**核心功能：**
- 在辦公室電腦運行 Agent，啟動 Headless Chrome
- 手機透過網頁介面遠端瀏覽和操作
- 支援 DOM 同步 + 截圖混合模式
- 配對碼機制綁定裝置

---

## 系統架構

```
┌─────────────────────────────────────────────────────────────┐
│                    bookbark.io 伺服器                        │
│  ┌─────────────┐  ┌─────────────┐  ┌─────────────────────┐  │
│  │  Web 服務   │  │  WebSocket  │  │  SQLite 資料庫      │  │
│  │  (靜態頁面) │  │  中繼服務   │  │  - users 帳號       │  │
│  │             │  │             │  │  - agents 配對資訊  │  │
│  └─────────────┘  └─────────────┘  └─────────────────────┘  │
└─────────────────────────────────────────────────────────────┘
         ↑                   ↑
         │ HTTPS             │ WSS
         ↓                   ↓
┌─────────────────┐   ┌─────────────────────────────────────┐
│   手機瀏覽器     │   │      辦公室 Windows 電腦            │
│   - 登入介面     │   │  ┌─────────────────────────────┐   │
│   - 輸入配對碼   │   │  │  weekend-chart-agent.exe   │   │
│   - 遠端瀏覽器   │   │  │  - Headless Chrome         │   │
│     控制介面     │   │  │  - 產生配對碼               │   │
└─────────────────┘   │  │  - 截圖 + DOM 擷取          │   │
                      │  └─────────────────────────────┘   │
                      └─────────────────────────────────────┘
```

---

## 資料庫結構

```sql
-- 用戶帳號
CREATE TABLE users (
    id INTEGER PRIMARY KEY,
    username TEXT UNIQUE NOT NULL,
    password_hash TEXT NOT NULL,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

-- 已配對的 Agent
CREATE TABLE agents (
    id INTEGER PRIMARY KEY,
    user_id INTEGER NOT NULL,
    agent_token TEXT UNIQUE NOT NULL,
    name TEXT DEFAULT 'My Computer',
    last_seen DATETIME,
    paired_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (user_id) REFERENCES users(id)
);

-- 配對碼（臨時）
CREATE TABLE pairing_codes (
    code TEXT PRIMARY KEY,
    agent_token TEXT UNIQUE NOT NULL,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    expires_at DATETIME
);
```

**預設帳號：** wake / 721225

---

## 通訊協議

### Agent → 伺服器

```json
// 連接認證
{ "type": "auth", "token": "agent_xxx..." }

// 請求配對碼
{ "type": "request_pairing_code" }

// DOM 更新
{ "type": "dom_update",
  "url": "https://example.com",
  "title": "網頁標題",
  "html": "<html>...</html>",
  "inputs": [{"selector": "#email", "value": "..."}]
}

// 截圖
{ "type": "screenshot",
  "url": "https://example.com",
  "image": "data:image/jpeg;base64,...",
  "width": 1920, "height": 1080
}
```

### 伺服器 → Agent

```json
{ "type": "pairing_code", "code": "482916", "expires_in": 300 }
```

### 手機 → Agent（經伺服器中繼）

```json
// 導航
{ "type": "navigate", "url": "https://google.com" }

// 點擊（DOM 模式）
{ "type": "click", "selector": "#submit-btn" }

// 點擊（截圖模式）
{ "type": "click_xy", "x": 150, "y": 320 }

// 輸入
{ "type": "input", "selector": "#search", "value": "hello" }

// 按鍵
{ "type": "key", "key": "Enter" }

// 請求截圖
{ "type": "request_screenshot" }
```

---

## 專案結構

```
/home/wake/weekend-chart/
├── server/
│   ├── main.go
│   ├── handlers/
│   │   ├── auth.go
│   │   ├── websocket.go
│   │   └── pairing.go
│   ├── models/
│   │   └── db.go
│   ├── relay/
│   │   └── relay.go
│   └── static/
│       ├── index.html
│       ├── dashboard.html
│       ├── remote.html
│       ├── download.html
│       ├── css/
│       │   └── style.css
│       ├── js/
│       │   ├── auth.js
│       │   ├── remote.js
│       │   └── websocket.js
│       └── downloads/
│           └── weekend-chart-agent.exe
│
├── agent/
│   ├── main.go
│   ├── browser/
│   │   └── chrome.go
│   ├── capture/
│   │   ├── dom.go
│   │   └── screenshot.go
│   ├── config/
│   │   └── config.go
│   └── build/
│
├── data/
│   └── weekend-chart.db
│
├── docs/
│   └── plans/
│
└── go.mod
```

---

## 手機介面

### 登入頁 (index.html)
- 帳號/密碼輸入
- 登入按鈕

### 控制台 (dashboard.html)
- 已配對 Agent 列表（顯示在線狀態）
- 配對新電腦（輸入 6 位數配對碼）

### 遠端瀏覽器 (remote.html)
- 網址列 + 導航按鈕
- 網頁內容區（可點擊）
- DOM/截圖模式切換
- 文字輸入列

### 下載頁 (download.html)
- Agent 下載連結
- 安裝說明

---

## Agent 介面（Windows）

### 首次啟動
- 顯示 6 位數配對碼
- 等待配對狀態

### 日常運行
- 已連線狀態
- 活動日誌
- 最小化到系統托盤

---

## 安全機制

1. **傳輸安全** - HTTPS/WSS（Caddy 自動 TLS）
2. **密碼儲存** - bcrypt 雜湊
3. **Session** - HttpOnly Cookie，7 天有效
4. **登入保護** - 失敗 5 次鎖定 15 分鐘
5. **Agent Token** - 32 字元隨機字串，永久有效
6. **配對碼** - 6 位數，5 分鐘過期

---

## Caddy 配置

```
bookbark.io {
    handle /weekend-chart/* {
        uri strip_prefix /weekend-chart
        reverse_proxy localhost:8080
    }
}
```

---

## 技術依賴

**Go 套件：**
- github.com/gorilla/websocket
- github.com/chromedp/chromedp
- github.com/mattn/go-sqlite3
- golang.org/x/crypto/bcrypt
- github.com/lxn/walk（Windows GUI）

**伺服器：**
- Caddy（HTTPS 反向代理）

---

## 實作階段

### 階段一：基礎架構
- 建立專案目錄結構
- 初始化 Go module
- 設定 SQLite 資料庫 + 建立帳號
- 安裝並配置 Caddy

### 階段二：伺服器核心
- HTTP 伺服器 + 靜態檔案服務
- 登入 API（Session 管理）
- WebSocket 連線管理
- 訊息中繼邏輯
- 配對碼產生與驗證

### 階段三：Agent 核心
- WebSocket 連接伺服器
- Headless Chrome 啟動
- DOM 擷取與傳送
- 截圖擷取與傳送
- 接收並執行操作指令
- 配對碼顯示視窗

### 階段四：手機前端
- 登入頁面
- 控制台
- 遠端瀏覽器介面
- DOM 渲染 + 截圖顯示
- 點擊與輸入操作

### 階段五：下載與部署
- 下載頁面
- Agent 交叉編譯
- Systemd 服務配置
- 測試完整流程

---

## 編譯指令

```bash
# 伺服器（Linux）
go build -o server/weekend-chart-server ./server

# Agent（Windows）
GOOS=windows GOARCH=amd64 go build -o agent/build/weekend-chart-agent.exe ./agent
```

---

## 存取路徑

```
https://bookbark.io/weekend-chart/          → 登入頁
https://bookbark.io/weekend-chart/dashboard → 控制台
https://bookbark.io/weekend-chart/remote    → 遠端瀏覽器
https://bookbark.io/weekend-chart/download  → Agent 下載
wss://bookbark.io/weekend-chart/ws          → WebSocket
```
