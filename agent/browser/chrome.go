package browser

import (
	"context"
	"encoding/base64"
	"log"
	"sync"
	"time"

	"github.com/chromedp/cdproto/dom"
	"github.com/chromedp/cdproto/input"
	"github.com/chromedp/cdproto/page"
	"github.com/chromedp/chromedp"
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
