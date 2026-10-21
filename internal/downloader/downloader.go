package downlaoder

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/launcher"
)

type Downloader struct {
	url            string
	waitForElement string
	debug          bool
}

func NewDownloader(url, waitForElement string, debug bool) *Downloader {
	return &Downloader{
		url:            url,
		waitForElement: waitForElement,
		debug:          debug,
	}
}

func (d *Downloader) Download[T any](turnContainer string, turnProcessor func(rod.Elements) (T, error)) (T, error) {

	// Launch headless browser.
	l := launcher.New().Headless(true)
	defer l.Cleanup()

	controlURL := l.MustLaunch()

	browser := rod.New().ControlURL(controlURL).MustConnect()
	defer browser.MustClose()

	page := browser.MustPage(d.url)

	// Wait for the Angular app to render the conversation content.
	// MustWaitStable / WaitRequestIdle both block indefinitely on Gemini because
	// it keeps persistent WebSocket connections. Use a fixed sleep instead, then
	// poll for the first user-query element with a timeout.
	log.Println("Waiting for page content to render...")
	time.Sleep(5 * time.Second)
	page.Timeout(30 * time.Second).MustElement(d.waitForElement)
	log.Println("Content ready.")

	if d.debug {
		// write html to file
		html, err := page.HTML()
		if err == nil {
			fileDateStr := time.Now().Format("2006-01-02-150405")
			outDir := "."
			// Write output file.
			outFileName := fmt.Sprintf("debug-%s.html", fileDateStr)
			outPath := filepath.Join(outDir, outFileName)
			if err := os.MkdirAll(outDir, 0o755); err == nil {
				if err := os.WriteFile(outPath, []byte(html), 0o644); err == nil {
					fmt.Printf("Saved debug html to file: %s\n", outPath)
				} else {
					fmt.Printf("Could not write output file %q: %w", outPath, err)
				}
			} else {
				fmt.Printf("Could not create output directory %q: %w", outDir, err)
			}
		} else {
			fmt.Printf("Could not download page html: %w\n", err)
		}
	}

	turns := page.MustElements(turnContainer)
	if len(turns) == 0 {
		var zero T
		return zero, fmt.Errorf("No %s elements found on the page.", turnContainer)
	}
	log.Printf("Found %d conversation turn(s).", len(turns))

	return turnProcessor(turns)
}
