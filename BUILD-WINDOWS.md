# Windows 編譯說明

## 必要工具

### 1. Go 語言環境
- 下載：https://go.dev/dl/
- 選擇 `go1.24.x.windows-amd64.msi`
- 執行安裝程式

### 2. GCC 編譯器（二選一）

**方法 A：TDM-GCC（簡單）**
- 下載：https://jmeubank.github.io/tdm-gcc/
- 選擇 64-bit 版本
- 安裝時勾選 "Add to PATH"

**方法 B：MSYS2 + MinGW-w64**
1. 下載 MSYS2：https://www.msys2.org/
2. 安裝後開啟 MSYS2 終端機
3. 執行：`pacman -S mingw-w64-x86_64-gcc`
4. 將 `C:\msys64\mingw64\bin` 加入系統 PATH

## 編譯步驟

### 使用腳本（推薦）

**CMD 使用者：**
```
雙擊 build-windows.bat
```

**PowerShell 使用者：**
```powershell
powershell -ExecutionPolicy Bypass -File build-windows.ps1
```

### 手動編譯

```cmd
cd C:\path\to\weekend-chart
set CGO_ENABLED=1
go build -ldflags "-H=windowsgui -s -w" -o agent\build\weekend-chart-agent.exe .\agent
```

## 編譯參數說明

| 參數 | 說明 |
|------|------|
| `-H=windowsgui` | 啟動時不顯示黑色控制台視窗 |
| `-s` | 移除符號表，減小檔案大小 |
| `-w` | 移除 DWARF 除錯資訊，進一步減小檔案 |

## 輸出位置

```
weekend-chart/agent/build/weekend-chart-agent.exe
```

## 常見問題

### "gcc not found"
- 安裝 TDM-GCC 或 MinGW-w64
- 確認 GCC 已加入 PATH
- 重新開啟終端機

### "go: command not found"
- 確認 Go 已安裝
- 確認 Go 已加入 PATH
- 重新開啟終端機

### 程式啟動顯示黑色視窗
- 編譯時加上 `-ldflags "-H=windowsgui"`
