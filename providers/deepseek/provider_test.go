package deepseek

import (
	"bytes"
	"os"
	"regexp"
	"strings"
	"testing"

	"golang.org/x/net/html"
)

// debugPage is a saved share page, produced by running the downloader with
// --debug.
const debugPage = "../../debug-2026-10-20-151022.html"

// loadTurns returns the outer HTML of every message element on the saved page,
// which is what the downloader hands to the turn processor.
func loadTurns(t *testing.T) []string {
	t.Helper()
	return loadTurnsFrom(t, debugPage)
}

// loadTurnsFrom returns the outer HTML of every message element in the given
// saved page. The page path is relative to the package directory.
func loadTurnsFrom(t *testing.T, page string) []string {
	t.Helper()

	raw, err := os.ReadFile(page)
	if err != nil {
		t.Skipf("saved debug page not available: %v", err)
	}
	doc, err := html.Parse(bytes.NewReader(raw))
	if err != nil {
		t.Fatalf("parse debug page: %v", err)
	}

	var messages []*html.Node
	collectNodesWithClass(doc, "ds-message", &messages)
	if len(messages) == 0 {
		t.Fatalf("no ds-message elements in %s", debugPage)
	}

	turns := make([]string, 0, len(messages))
	for _, message := range messages {
		var buf bytes.Buffer
		if err := html.Render(&buf, message); err != nil {
			t.Fatalf("render message: %v", err)
		}
		turns = append(turns, buf.String())
	}
	return turns
}

func TestTurnToMarkdownOnSavedPage(t *testing.T) {
	var sb strings.Builder
	for i, turn := range loadTurns(t) {
		md := turnToMarkdown(turn)
		if md == "" {
			t.Fatalf("turn %d produced no markdown", i+1)
		}
		sb.WriteString(md)
		sb.WriteString("\n\n")
	}
	got := sb.String()

	// The saved page holds three prompts, each followed by a reply that carries
	// a "Thought for N seconds" panel.
	if n := strings.Count(got, "## User\n\n"); n != 3 {
		t.Errorf("expected 3 user sections, got %d", n)
	}
	if n := strings.Count(got, "## DeepSeek\n\n"); n != 3 {
		t.Errorf("expected 3 DeepSeek sections, got %d", n)
	}
	if n := strings.Count(got, "<details>"); n != 3 {
		t.Errorf("expected 3 reasoning blocks, got %d", n)
	}
	for _, want := range []string{
		"<summary>Thought for 12 seconds</summary>",
		"<summary>Thought for 9 seconds</summary>",
		"<summary>Thought for 8 seconds</summary>",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing reasoning summary %q", want)
		}
	}

	// Answers keep their citations as links, without the badge spacer.
	if !strings.Contains(got, "https://theses.hal.science/tel-04551620v1/file/125450_GOBERT_2024_diffusion.pdf") {
		t.Error("citation link was dropped")
	}
	if strings.Contains(got, "[-31]") || strings.Contains(got, "](https://theses.hal.science/tel-04551620v1/file/125450_GOBERT_2024_diffusion.pdf#38%237)") == false {
		t.Error("citation badge not rendered as a plain link")
	}

	// Page chrome must not leak into the transcript.
	for _, unwanted := range []string{"ds-collapsible-text", "ds-button", "site_logo", "Continue with DeepSeek"} {
		if strings.Contains(got, unwanted) {
			t.Errorf("output contains chrome element %q", unwanted)
		}
	}
	if regexp.MustCompile(`\n{4,}`).MatchString(got) {
		t.Error("output has runs of blank lines")
	}
}

// debugPageWithCode is a saved share page whose reply contains a python code
// block rendered with a language banner.
const debugPageWithCode = "../../debug-2026-10-20-161453.html"

func TestCodeBlockOnSavedPage(t *testing.T) {
	var got string
	for _, turn := range loadTurnsFrom(t, debugPageWithCode) {
		got += turnToMarkdown(turn) + "\n\n"
	}

	if !strings.Contains(got, "```python\n") {
		t.Error("code block is missing its language tag")
	}
	if !strings.Contains(got, "class LeanProof(Agent):") {
		t.Error("code block body is missing")
	}
	// The banner must not leak as loose text next to the fence.
	for _, unwanted := range []string{"\nCopy\n", "\nDownload\n", "\npython\n\n"} {
		if strings.Contains(got, unwanted) {
			t.Errorf("code block banner leaked into output as %q", unwanted)
		}
	}
}

func TestTurnToMarkdown(t *testing.T) {
	tests := []struct {
		name string
		html string
		want string
	}{
		{
			name: "user only",
			html: `<div class="x ds-message"><div class="fbb737a4"><span>hello</span></div></div>`,
			want: "## User\n\nhello",
		},
		{
			name: "assistant only",
			html: `<div class="x ds-message"><div class="ds-markdown ds-assistant-message-main-content">` +
				`<p>hi there</p></div></div>`,
			want: "## DeepSeek\n\nhi there",
		},
		{
			name: "prompt, reasoning and reply",
			html: `<div class="x ds-message">` +
				`<div class="fbb737a4"><span>question</span></div>` +
				`<span>Thought for 3 seconds</span>` +
				`<div class="ds-think-content"><p>considering</p></div>` +
				`<div class="ds-markdown ds-assistant-message-main-content"><p>answer</p></div></div>`,
			want: "## User\n\nquestion\n\n## DeepSeek\n\n" +
				"<details>\n<summary>Thought for 3 seconds</summary>\n\nconsidering\n\n</details>\n\nanswer",
		},
		{
			name: "reasoning without a header",
			html: `<div class="x ds-message"><div class="ds-think-content"><p>musing</p></div>` +
				`<div class="ds-markdown ds-assistant-message-main-content"><p>done</p></div></div>`,
			want: "## DeepSeek\n\n<details>\n<summary>Reasoning</summary>\n\nmusing\n\n</details>\n\ndone",
		},
		{
			name: "section headings nest under the turn heading",
			html: `<div class="x ds-message"><div class="ds-markdown ds-assistant-message-main-content">` +
				`<h2>Overview</h2><p>body</p><h4>Detail</h4><p>more</p></div></div>`,
			want: "## DeepSeek\n\n### Overview\n\nbody\n\n##### Detail\n\nmore",
		},
		{
			name: "code block with banner and bare span lines",
			html: `<div class="x ds-message"><div class="ds-markdown ds-assistant-message-main-content">` +
				`<div class="md-code-block"><div class="md-code-block-banner-wrap">` +
				`<div class="md-code-block-banner"><span class="d813de27">go</span>` +
				`<span class="code-info-button-text">Download</span></div></div>` +
				`<pre><span>fmt.Println("hi")</span></pre></div></div></div>`,
			want: "## DeepSeek\n\n```go\nfmt.Println(\"hi\")\n```",
		},
		{
			name: "code block that already has a code element",
			html: `<div class="x ds-message"><div class="ds-markdown ds-assistant-message-main-content">` +
				`<div class="md-code-block"><div class="md-code-block-banner-wrap">` +
				`<span class="d813de27">TypeScript</span></div>` +
				`<pre><code>let x = 1</code></pre></div></div></div>`,
			want: "## DeepSeek\n\n```typescript\nlet x = 1\n```",
		},
		{
			name: "nothing to convert",
			html: `<div class="x ds-message"><div class="foo"><span>   </span></div></div>`,
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := turnToMarkdown(tt.html); got != tt.want {
				t.Errorf("turnToMarkdown() =\n%q\nwant\n%q", got, tt.want)
			}
		})
	}
}
