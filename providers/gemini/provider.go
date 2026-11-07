package gemini

import (
	"bytes"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/go-rod/rod"
	downlaoder "github.com/zodimo/download-ai-chat/internal/downloader"
	"github.com/zodimo/download-ai-chat/internal/markdown"
	"github.com/zodimo/download-ai-chat/internal/urlhelper"
	"github.com/zodimo/download-ai-chat/providers"
	"golang.org/x/net/html"
)

// urlPattern = "https://share.gemini.google/{session-id}"
const urlPrefix = "https://share.gemini.google"
const providerName = "gemini"

var _ providers.Platform = (*GeminiPlatform)(nil)

type GeminiPlatform struct {
}

func (gp *GeminiPlatform) UrlPrefix() string {
	return urlPrefix
}
func (gp *GeminiPlatform) ConvertToMarkdown(url string, debug bool) (*providers.ProcessedConversation, error) {
	convID := urlhelper.ConversationID(url)
	if convID == "" {
		return nil, fmt.Errorf("Could not extract conversation ID from URL: %s", url)
	}

	log.Printf("Conversation ID: %s", convID)

	chatDownloader := downlaoder.NewDownloader(url, "user-query-content", debug)

	return chatDownloader.Download("share-turn-viewer", mkTurnProcessor(convID, url), nil)
}
func (gp *GeminiPlatform) Name() string {
	return providerName
}

func init() {
	providers.Register(providerName, &GeminiPlatform{})
}

// mkTurnProcessor renders the turns of a shared conversation. Gemini keeps all
// of its content in the turns, so the HTML prepared outside them is unused.
func mkTurnProcessor(convID, url string) func(turns rod.Elements, _ string) (*providers.ProcessedConversation, error) {
	return func(turns rod.Elements, _ string) (*providers.ProcessedConversation, error) {

		var sb strings.Builder
		dateStr := time.Now().Format("2006-01-02 15:04:05")

		sb.WriteString(fmt.Sprintf("# Gemini Conversation: %s\n\n", convID))
		sb.WriteString(fmt.Sprintf("> Date: %s\n", dateStr))
		sb.WriteString(fmt.Sprintf("> Source: %s\n\n", url))

		turnNum := 0
		for _, turn := range turns {
			// Try to find user query in this turn.
			userEls, err := turn.Elements("user-query-content .query-text")
			if err == nil && len(userEls) > 0 {
				userEl := userEls[0]
				html, err := userEl.HTML()
				if err != nil {
					log.Printf("Warning: could not get HTML for user turn: %v", err)
				} else {
					md, err := markdown.Convert(preprocessHTML(html))
					if err != nil {
						log.Printf("Warning: could not convert user HTML to markdown: %v", err)
					} else {
						md = strings.TrimSpace(md)
						if md != "" {
							if turnNum > 0 {
								sb.WriteString("---\n\n")
							}
							sb.WriteString("## User\n\n")
							sb.WriteString(md)
							sb.WriteString("\n\n")
							turnNum++
						}
					}
				}
			}

			// Try to find agent response in this turn.
			agentEls, err := turn.Elements(".markdown.markdown-main-panel")
			if err == nil && len(agentEls) > 0 {
				agentEl := agentEls[0]
				html, err := agentEl.HTML()
				if err != nil {
					log.Printf("Warning: could not get HTML for agent turn: %v", err)
				} else {
					md, err := markdown.Convert(preprocessHTML(html))
					if err != nil {
						log.Printf("Warning: could not convert agent HTML to markdown: %v", err)
					} else {
						md = strings.TrimSpace(md)
						if md != "" {
							if turnNum > 0 {
								sb.WriteString("---\n\n")
							}
							sb.WriteString("## Gemini\n\n")
							sb.WriteString(md)
							sb.WriteString("\n\n")
							turnNum++
						}
					}
				}
			}
		}

		if turnNum == 0 {
			return nil, fmt.Errorf("No conversation content could be extracted from the page.")
		}
		return providers.NewProcessedConversation(
			convID,
			url,
			sb.String(),
		), nil
	}

}

func getAttr(n *html.Node, key string) string {
	for _, a := range n.Attr {
		if a.Key == key {
			return a.Val
		}
	}
	return ""
}

func findNodeWithClass(n *html.Node, tag, className string) *html.Node {
	if n.Type == html.ElementNode && n.Data == tag && strings.Contains(getAttr(n, "class"), className) {
		return n
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if res := findNodeWithClass(c, tag, className); res != nil {
			return res
		}
	}
	return nil
}

func findFirstElement(n *html.Node, tag string) *html.Node {
	if n.Type == html.ElementNode && n.Data == tag {
		return n
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if res := findFirstElement(c, tag); res != nil {
			return res
		}
	}
	return nil
}

func getText(n *html.Node) string {
	if n.Type == html.TextNode {
		return n.Data
	}
	var sb strings.Builder
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		sb.WriteString(getText(c))
	}
	return sb.String()
}

func collectCodeBlocks(n *html.Node, list *[]*html.Node) {
	if n.Type == html.ElementNode && n.Data == "code-block" {
		*list = append(*list, n)
		return
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		collectCodeBlocks(c, list)
	}
}

var languageMap = map[string]string{
	"protocol buffers": "protobuf",
	"protobuf":         "protobuf",
	"go":               "go",
	"golang":           "go",
	"c++":              "cpp",
	"c#":               "csharp",
	"typescript":       "typescript",
	"javascript":       "javascript",
	"bash":             "bash",
	"shell":            "bash",
	"dockerfile":       "dockerfile",
	"docker":           "dockerfile",
	"python":           "python",
	"html":             "html",
	"css":              "css",
	"sql":              "sql",
	"json":             "json",
	"yaml":             "yaml",
	"markdown":         "markdown",
	"rust":             "rust",
}

func getLanguageAnnotation(lang string) string {
	lang = strings.TrimSpace(strings.ToLower(lang))
	if val, ok := languageMap[lang]; ok {
		return val
	}
	return lang
}

func preprocessHTML(htmlStr string) string {
	doc, err := html.Parse(strings.NewReader(htmlStr))
	if err != nil {
		log.Printf("Warning: failed to parse HTML for preprocessing: %v", err)
		return htmlStr
	}

	var codeBlocks []*html.Node
	collectCodeBlocks(doc, &codeBlocks)

	for _, cb := range codeBlocks {
		decor := findNodeWithClass(cb, "div", "code-block-decoration")
		var lang string
		if decor != nil {
			span := findFirstElement(decor, "span")
			if span != nil {
				lang = getLanguageAnnotation(getText(span))
			}
		}

		preNode := findFirstElement(cb, "pre")
		if preNode == nil {
			continue
		}

		codeNode := findFirstElement(preNode, "code")
		if codeNode != nil && lang != "" {
			found := false
			for i, a := range codeNode.Attr {
				if a.Key == "class" {
					codeNode.Attr[i].Val = "language-" + lang
					found = true
					break
				}
			}
			if !found {
				codeNode.Attr = append(codeNode.Attr, html.Attribute{Key: "class", Val: "language-" + lang})
			}
		}

		if cb.Parent != nil {
			if preNode.Parent != nil {
				preNode.Parent.RemoveChild(preNode)
			}
			cb.Parent.InsertBefore(preNode, cb)
			cb.Parent.RemoveChild(cb)
		}
	}

	var buf bytes.Buffer
	if err := html.Render(&buf, doc); err != nil {
		log.Printf("Warning: failed to render preprocessed HTML: %v", err)
		return htmlStr
	}
	return buf.String()
}
