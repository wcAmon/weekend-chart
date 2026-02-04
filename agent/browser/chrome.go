package browser

import (
	"context"
	"encoding/base64"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/chromedp/cdproto/dom"
	"github.com/chromedp/cdproto/input"
	"github.com/chromedp/cdproto/page"
	"github.com/chromedp/chromedp"
	"github.com/chromedp/chromedp/kb"
)

type Browser struct {
	ctx        context.Context
	cancel     context.CancelFunc
	mu         sync.Mutex
	currentURL string
}

type PageState struct {
	URL    string `json:"url"`
	Title  string `json:"title"`
	HTML   string `json:"html"`
	Width  int    `json:"width"`
	Height int    `json:"height"`
}

// SimplifiedPageState contains essential page info for AI understanding
type SimplifiedPageState struct {
	URL          string      `json:"url"`
	Title        string      `json:"title"`
	FocusedElement string    `json:"focused_element,omitempty"`
	Inputs       []InputInfo `json:"inputs,omitempty"`
	Buttons      []ButtonInfo `json:"buttons,omitempty"`
	Links        []LinkInfo  `json:"links,omitempty"`
	Text         string      `json:"text,omitempty"` // Main visible text (truncated)
}

type InputInfo struct {
	Type        string `json:"type"`
	Name        string `json:"name,omitempty"`
	ID          string `json:"id,omitempty"`
	Placeholder string `json:"placeholder,omitempty"`
	Value       string `json:"value,omitempty"`
	Label       string `json:"label,omitempty"`
	Focused     bool   `json:"focused,omitempty"`
	X           int    `json:"x"`
	Y           int    `json:"y"`
}

type ButtonInfo struct {
	Text string `json:"text"`
	Type string `json:"type,omitempty"`
	X    int    `json:"x"`
	Y    int    `json:"y"`
}

type LinkInfo struct {
	Text string `json:"text"`
	Href string `json:"href,omitempty"`
	X    int    `json:"x"`
	Y    int    `json:"y"`
}

type Screenshot struct {
	URL    string `json:"url"`
	Image  string `json:"image"` // base64
	Width  int    `json:"width"`
	Height int    `json:"height"`
}

func New() (*Browser, error) {
	opts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.Flag("headless", true),
		chromedp.Flag("disable-gpu", true),
		chromedp.Flag("no-sandbox", true),
		chromedp.Flag("disable-dev-shm-usage", true),
		chromedp.WindowSize(1920, 1080),
	)

	allocCtx, _ := chromedp.NewExecAllocator(context.Background(), opts...)
	ctx, cancel := chromedp.NewContext(allocCtx)

	// Navigate to blank page to start
	if err := chromedp.Run(ctx, chromedp.Navigate("about:blank")); err != nil {
		cancel()
		return nil, err
	}

	return &Browser{
		ctx:    ctx,
		cancel: cancel,
	}, nil
}

func (b *Browser) Close() {
	b.cancel()
}

func (b *Browser) Navigate(url string) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	ctx, cancel := context.WithTimeout(b.ctx, 30*time.Second)
	defer cancel()

	err := chromedp.Run(ctx, chromedp.Navigate(url))
	if err != nil {
		return err
	}

	b.currentURL = url
	return nil
}

func (b *Browser) Click(selector string) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	ctx, cancel := context.WithTimeout(b.ctx, 10*time.Second)
	defer cancel()

	return chromedp.Run(ctx,
		chromedp.WaitVisible(selector, chromedp.ByQuery),
		chromedp.Click(selector, chromedp.ByQuery),
	)
}

func (b *Browser) ClickXY(x, y int) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	ctx, cancel := context.WithTimeout(b.ctx, 10*time.Second)
	defer cancel()

	// Simple mouse click - let the browser handle focus naturally
	return chromedp.Run(ctx,
		chromedp.MouseClickXY(float64(x), float64(y)),
	)
}

func (b *Browser) Input(selector, value string) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	ctx, cancel := context.WithTimeout(b.ctx, 10*time.Second)
	defer cancel()

	// If no selector, send keys to focused element
	if selector == "" {
		return chromedp.Run(ctx,
			chromedp.KeyEvent(value),
		)
	}

	return chromedp.Run(ctx,
		chromedp.WaitVisible(selector, chromedp.ByQuery),
		chromedp.Clear(selector, chromedp.ByQuery),
		chromedp.SendKeys(selector, value, chromedp.ByQuery),
	)
}

func (b *Browser) InputToFocused(value string) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	ctx, cancel := context.WithTimeout(b.ctx, 10*time.Second)
	defer cancel()

	// Use CDP's insertText to type text directly - works like real keyboard input
	return chromedp.Run(ctx,
		chromedp.ActionFunc(func(ctx context.Context) error {
			return input.InsertText(value).Do(ctx)
		}),
	)
}

func (b *Browser) PressKey(key string) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	ctx, cancel := context.WithTimeout(b.ctx, 5*time.Second)
	defer cancel()

	// Map key names to chromedp keyboard keys
	keyMap := map[string]string{
		"Backspace": kb.Backspace,
		"backspace": kb.Backspace,
		"Delete":    kb.Delete,
		"delete":    kb.Delete,
		"Tab":       kb.Tab,
		"tab":       kb.Tab,
		"Enter":     kb.Enter,
		"enter":     kb.Enter,
		"Escape":    kb.Escape,
		"escape":    kb.Escape,
		"ArrowUp":   kb.ArrowUp,
		"ArrowDown": kb.ArrowDown,
		"ArrowLeft": kb.ArrowLeft,
		"ArrowRight": kb.ArrowRight,
	}

	if mappedKey, ok := keyMap[key]; ok {
		log.Printf("PressKey: mapping %q to kb constant", key)
		return chromedp.Run(ctx,
			chromedp.KeyEvent(mappedKey),
		)
	}

	// For unmapped keys, try direct key event
	log.Printf("PressKey: using direct key %q", key)
	return chromedp.Run(ctx,
		chromedp.KeyEvent(key),
	)
}

func (b *Browser) SelectAll() error {
	b.mu.Lock()
	defer b.mu.Unlock()

	ctx, cancel := context.WithTimeout(b.ctx, 5*time.Second)
	defer cancel()

	// Use JavaScript to select all text in the focused input/textarea element
	return chromedp.Run(ctx,
		chromedp.Evaluate(`
			(function() {
				var el = document.activeElement;
				if (el && (el.tagName === 'INPUT' || el.tagName === 'TEXTAREA')) {
					el.select();
					return 'selected';
				}
				return 'no input focused';
			})()
		`, nil),
	)
}

func (b *Browser) Scroll(direction string, amount int) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	ctx, cancel := context.WithTimeout(b.ctx, 5*time.Second)
	defer cancel()

	// Calculate scroll delta (negative for up, positive for down)
	deltaY := amount
	if direction == "up" {
		deltaY = -amount
	}

	// Use JavaScript to scroll
	return chromedp.Run(ctx,
		chromedp.Evaluate(fmt.Sprintf(`window.scrollBy(0, %d)`, deltaY), nil),
	)
}

func (b *Browser) GetDOM() (*PageState, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	ctx, cancel := context.WithTimeout(b.ctx, 10*time.Second)
	defer cancel()

	var url, title, html string

	err := chromedp.Run(ctx,
		chromedp.Location(&url),
		chromedp.Title(&title),
		chromedp.ActionFunc(func(ctx context.Context) error {
			node, err := dom.GetDocument().Do(ctx)
			if err != nil {
				return err
			}
			html, err = dom.GetOuterHTML().WithNodeID(node.NodeID).Do(ctx)
			return err
		}),
	)
	if err != nil {
		return nil, err
	}

	return &PageState{
		URL:   url,
		Title: title,
		HTML:  html,
	}, nil
}

// GetSimplifiedPageState extracts essential page info for AI understanding
func (b *Browser) GetSimplifiedPageState() (*SimplifiedPageState, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	ctx, cancel := context.WithTimeout(b.ctx, 10*time.Second)
	defer cancel()

	var url, title string
	var result map[string]interface{}

	jsCode := `
	(function() {
		function getRect(el) {
			const r = el.getBoundingClientRect();
			return { x: Math.round(r.x + r.width/2), y: Math.round(r.y + r.height/2) };
		}

		function getLabel(input) {
			// Try associated label
			if (input.id) {
				const label = document.querySelector('label[for="' + input.id + '"]');
				if (label) return label.textContent.trim();
			}
			// Try parent label
			const parentLabel = input.closest('label');
			if (parentLabel) return parentLabel.textContent.trim();
			// Try aria-label
			if (input.getAttribute('aria-label')) return input.getAttribute('aria-label');
			// Try placeholder
			if (input.placeholder) return input.placeholder;
			return '';
		}

		const focused = document.activeElement;
		const focusedInfo = focused && focused !== document.body ?
			(focused.tagName + (focused.id ? '#' + focused.id : '') + (focused.name ? '[name=' + focused.name + ']' : '')) : '';

		// Get inputs
		const inputs = [];
		document.querySelectorAll('input, textarea, select').forEach(function(el) {
			if (el.offsetParent === null) return; // Skip hidden
			const rect = getRect(el);
			inputs.push({
				type: el.type || el.tagName.toLowerCase(),
				name: el.name || '',
				id: el.id || '',
				placeholder: el.placeholder || '',
				value: el.type === 'password' ? '***' : (el.value || ''),
				label: getLabel(el),
				focused: el === focused,
				x: rect.x,
				y: rect.y
			});
		});

		// Get buttons
		const buttons = [];
		document.querySelectorAll('button, input[type="submit"], input[type="button"], [role="button"]').forEach(function(el) {
			if (el.offsetParent === null) return;
			const rect = getRect(el);
			buttons.push({
				text: el.textContent.trim() || el.value || '',
				type: el.type || '',
				x: rect.x,
				y: rect.y
			});
		});

		// Get links (limit to visible, first 20)
		const links = [];
		const allLinks = document.querySelectorAll('a[href]');
		for (let i = 0; i < allLinks.length && links.length < 20; i++) {
			const el = allLinks[i];
			if (el.offsetParent === null) continue;
			const rect = getRect(el);
			const text = el.textContent.trim();
			if (text) {
				links.push({
					text: text.substring(0, 50),
					href: el.getAttribute('href'),
					x: rect.x,
					y: rect.y
				});
			}
		}

		// Get main text (truncated)
		const bodyText = document.body.innerText.substring(0, 1000);

		return {
			focused_element: focusedInfo,
			inputs: inputs,
			buttons: buttons,
			links: links,
			text: bodyText
		};
	})()
	`

	err := chromedp.Run(ctx,
		chromedp.Location(&url),
		chromedp.Title(&title),
		chromedp.Evaluate(jsCode, &result),
	)
	if err != nil {
		return nil, err
	}

	state := &SimplifiedPageState{
		URL:   url,
		Title: title,
	}

	// Parse result
	if focused, ok := result["focused_element"].(string); ok {
		state.FocusedElement = focused
	}
	if text, ok := result["text"].(string); ok {
		state.Text = text
	}

	// Parse inputs
	if inputsRaw, ok := result["inputs"].([]interface{}); ok {
		for _, v := range inputsRaw {
			if m, ok := v.(map[string]interface{}); ok {
				input := InputInfo{}
				if s, ok := m["type"].(string); ok { input.Type = s }
				if s, ok := m["name"].(string); ok { input.Name = s }
				if s, ok := m["id"].(string); ok { input.ID = s }
				if s, ok := m["placeholder"].(string); ok { input.Placeholder = s }
				if s, ok := m["value"].(string); ok { input.Value = s }
				if s, ok := m["label"].(string); ok { input.Label = s }
				if b, ok := m["focused"].(bool); ok { input.Focused = b }
				if f, ok := m["x"].(float64); ok { input.X = int(f) }
				if f, ok := m["y"].(float64); ok { input.Y = int(f) }
				state.Inputs = append(state.Inputs, input)
			}
		}
	}

	// Parse buttons
	if buttonsRaw, ok := result["buttons"].([]interface{}); ok {
		for _, v := range buttonsRaw {
			if m, ok := v.(map[string]interface{}); ok {
				btn := ButtonInfo{}
				if s, ok := m["text"].(string); ok { btn.Text = s }
				if s, ok := m["type"].(string); ok { btn.Type = s }
				if f, ok := m["x"].(float64); ok { btn.X = int(f) }
				if f, ok := m["y"].(float64); ok { btn.Y = int(f) }
				state.Buttons = append(state.Buttons, btn)
			}
		}
	}

	// Parse links
	if linksRaw, ok := result["links"].([]interface{}); ok {
		for _, v := range linksRaw {
			if m, ok := v.(map[string]interface{}); ok {
				link := LinkInfo{}
				if s, ok := m["text"].(string); ok { link.Text = s }
				if s, ok := m["href"].(string); ok { link.Href = s }
				if f, ok := m["x"].(float64); ok { link.X = int(f) }
				if f, ok := m["y"].(float64); ok { link.Y = int(f) }
				state.Links = append(state.Links, link)
			}
		}
	}

	return state, nil
}

func (b *Browser) GetScreenshot() (*Screenshot, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	ctx, cancel := context.WithTimeout(b.ctx, 10*time.Second)
	defer cancel()

	var url string
	var buf []byte

	err := chromedp.Run(ctx,
		chromedp.Location(&url),
		chromedp.ActionFunc(func(ctx context.Context) error {
			var err error
			buf, err = page.CaptureScreenshot().
				WithFormat(page.CaptureScreenshotFormatJpeg).
				WithQuality(80).
				Do(ctx)
			return err
		}),
	)
	if err != nil {
		return nil, err
	}

	return &Screenshot{
		URL:    url,
		Image:  "data:image/jpeg;base64," + base64.StdEncoding.EncodeToString(buf),
		Width:  1920,
		Height: 1080,
	}, nil
}

func (b *Browser) WatchDOMChanges(callback func(*PageState)) {
	go func() {
		ticker := time.NewTicker(2 * time.Second)
		defer ticker.Stop()

		var lastHTML string
		for range ticker.C {
			state, err := b.GetDOM()
			if err != nil {
				log.Printf("DOM watch error: %v", err)
				continue
			}

			if state.HTML != lastHTML {
				lastHTML = state.HTML
				callback(state)
			}
		}
	}()
}
