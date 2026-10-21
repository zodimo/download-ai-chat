package deepseek

import (
	"bytes"
	"fmt"
	"log"
	"strings"
	"time"

	htmltomarkdown "github.com/JohannesKaufmann/html-to-markdown/v2"
	"github.com/go-rod/rod"
	downlaoder "github.com/zodimo/download-ai-chat/internal/downloader"
	"github.com/zodimo/download-ai-chat/internal/urlhelper"
	"github.com/zodimo/download-ai-chat/providers"
	"golang.org/x/net/html"
)

// urlPattern = "https://chat.deepseek.com/share/{session-id}"
const urlPrefix = "https://chat.deepseek.com/share"
const providerName = "deepseek"

// Class names of the sections of a single .ds-message element on a shared
// DeepSeek page. A message holds the user's prompt, the model's reasoning panel
// and/or the model's reply, depending on the layout the share link renders with.
const (
	userTurnClass      = "fbb737a4"
	assistantTurnClass = "ds-assistant-message-main-content"
	reasoningTurnClass = "ds-think-content"
	citationClass      = "ds-markdown-cite"
)

var _ providers.Platform = (*DeepseekPlatform)(nil)

type DeepseekPlatform struct {
}

func (gp *DeepseekPlatform) UrlPrefix() string {
	return urlPrefix
}
func (gp *DeepseekPlatform) ConvertToMarkdown(url string, debug bool) (*providers.ProcessedConversation, error) {
	convID := urlhelper.ConversationID(url)
	if convID == "" {
		return nil, fmt.Errorf("Could not extract conversation ID from URL: %s", url)
	}

	log.Printf("Conversation ID: %s", convID)

	chatDownloader := downlaoder.NewDownloader(url, ".ds-virtual-list-items", debug)

	return chatDownloader.Download(".ds-message", mkTurnProcessor(convID, url))
}
func (gp *DeepseekPlatform) Name() string {
	return providerName
}

func init() {
	providers.Register(providerName, &DeepseekPlatform{})
}

func mkTurnProcessor(convID, url string) func(turns rod.Elements) (*providers.ProcessedConversation, error) {
	return func(turns rod.Elements) (*providers.ProcessedConversation, error) {
		var sb strings.Builder
		dateStr := time.Now().Format("2006-01-02 15:04:05")

		sb.WriteString(fmt.Sprintf("# DeepSeek Conversation: %s\n\n", convID))
		sb.WriteString(fmt.Sprintf("> Date: %s\n", dateStr))
		sb.WriteString(fmt.Sprintf("> Source: %s\n\n", url))

		turnNum := 0
		for i, turn := range turns {
			turnHTML, err := turn.HTML()
			if err != nil {
				log.Printf("Warning: could not get HTML for turn %d: %v", i+1, err)
				continue
			}
			turnMD := turnToMarkdown(turnHTML)
			if turnMD == "" {
				continue
			}
			if turnNum > 0 {
				sb.WriteString("---\n\n")
			}
			sb.WriteString(turnMD)
			sb.WriteString("\n\n")
			turnNum++
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

// turnToMarkdown converts the outer HTML of one message element into the
// markdown for that turn.
func turnToMarkdown(turnHTML string) string {
	doc, err := html.Parse(strings.NewReader(turnHTML))
	if err != nil {
		log.Printf("Warning: failed to parse turn HTML: %v", err)
		return ""
	}
	preprocessHTML(doc)

	var sections []string

	if userMD := nodeToMarkdown(findByClass(doc, userTurnClass), "user"); userMD != "" {
		sections = append(sections, "## User\n\n"+userMD)
	}

	reply := ""
	if reasoningMD := nodeToMarkdown(findByClass(doc, reasoningTurnClass), "reasoning"); reasoningMD != "" {
		reply = fmt.Sprintf("<details>\n<summary>%s</summary>\n\n%s\n\n</details>\n\n",
			reasoningLabel(doc), reasoningMD)
	}
	reply += nodeToMarkdown(findByClass(doc, assistantTurnClass), "assistant")
	if reply = strings.TrimSpace(reply); reply != "" {
		sections = append(sections, "## DeepSeek\n\n"+reply)
	}

	return strings.Join(sections, "\n\n")
}

// nodeToMarkdown converts an element to markdown, returning "" when there is
// nothing to convert. The element is mutated, since it is a detached node of the
// turn document that the turn discards afterwards.
func nodeToMarkdown(node *html.Node, name string) string {
	if node == nil {
		return ""
	}
	// Markup is preprocessed on the whole turn document before this is called,
	// so code blocks can still see their language banner.
	var buf bytes.Buffer
	if err := html.Render(&buf, node); err != nil {
		log.Printf("Warning: failed to render %s HTML: %v", name, err)
		return ""
	}
	md, err := htmltomarkdown.ConvertString(buf.String())
	if err != nil {
		log.Printf("Warning: could not convert %s HTML to markdown: %v", name, err)
		return ""
	}
	return strings.TrimSpace(md)
}

// reasoningLabel reads the "Thought for N seconds" header, which sits next to
// the reasoning panel rather than inside it.
func reasoningLabel(doc *html.Node) string {
	const fallback = "Reasoning"

	var spans []*html.Node
	collectElements(doc, "span", &spans)
	for _, span := range spans {
		text := strings.Join(strings.Fields(getText(span)), " ")
		if strings.HasPrefix(strings.ToLower(text), "thought for") {
			return text
		}
	}
	return fallback
}

// preprocessHTML prepares rendered DeepSeek markup for the html-to-markdown
// converter:
//
//   - code blocks carry their language in a banner outside the <pre>, and the
//     <pre> often holds bare <span> lines rather than a <code> element, so move
//     the language onto a language-<lang> class the converter turns into a
//     fenced block with a language tag
//   - section headings are demoted one level so they nest under the transcript's
//     own "## User" and "## DeepSeek" headings
//   - citation badges pad themselves with a hidden dash next to the visible
//     number, which the converter would render as "[-31](url)", so drop the
//     spacer and keep the citation number as the link text
func preprocessHTML(doc *html.Node) {
	annotateCodeBlocks(doc)

	var headings []*html.Node
	for level := 1; level <= 5; level++ {
		collectElements(doc, fmt.Sprintf("h%d", level), &headings)
	}
	for _, heading := range headings {
		level := int(heading.Data[1] - '0')
		heading.Data = fmt.Sprintf("h%d", level+1)
	}

	var citations []*html.Node
	collectNodesWithClass(doc, citationClass, &citations)
	for _, cite := range citations {
		var spacers []*html.Node
		for child := cite.FirstChild; child != nil; child = child.NextSibling {
			if child.Type != html.ElementNode {
				continue
			}
			text := strings.TrimSpace(getText(child))
			if strings.Trim(text, "-–— ") == "" {
				spacers = append(spacers, child)
			}
		}
		for _, spacer := range spacers {
			removeNode(spacer)
		}
	}
}

// annotateCodeBlocks tags every <pre> with the language shown in its banner, so
// the converter emits a fenced block with a language tag. The banner lives
// outside the <pre> and the <pre> may hold bare <span> lines instead of a single
// <code>, so a <code> is added when one is missing. The banner itself is dropped
// so its language label and Copy/Download buttons do not leak into the output.
func annotateCodeBlocks(doc *html.Node) {
	var codeBlocks []*html.Node
	collectElements(doc, "pre", &codeBlocks)
	for _, pre := range codeBlocks {
		lang := codeBlockLanguage(pre)
		if lang != "" {
			class := "language-" + lang
			code := findFirstElement(pre, "code")
			if code == nil {
				// Wrap the existing lines so the class survives conversion.
				code = &html.Node{Type: html.ElementNode, Data: "code"}
				for child := pre.FirstChild; child != nil; {
					next := child.NextSibling
					pre.RemoveChild(child)
					code.AppendChild(child)
					child = next
				}
				pre.AppendChild(code)
			}
			code.Attr = append(removeAttr(code, "class"), html.Attribute{Key: "class", Val: class})
		}

		// Drop the banner and its wrapper, keeping any siblings such as the
		// code block's own container.
		container := pre.Parent
		if container == nil || !hasClass(container, "md-code-block") {
			continue
		}
		var banners []*html.Node
		collectNodesWithClass(container, "md-code-block-banner-wrap", &banners)
		if len(banners) == 0 {
			collectNodesWithClass(container, "md-code-block-banner", &banners)
		}
		for _, banner := range banners {
			removeNode(banner)
		}
	}
}

// codeBlockLanguage finds the language label in the banner rendered alongside a
// code block. The banner is a sibling of the <pre>, so it is looked up on the
// enclosing code-block container.
func codeBlockLanguage(pre *html.Node) string {
	container := pre.Parent
	if container == nil || !hasClass(container, "md-code-block") {
		// Without the banner wrapper, fall back to a language-* class already
		// set on the <code> element.
		if code := findFirstElement(pre, "code"); code != nil {
			for _, c := range strings.Fields(getAttr(code, "class")) {
				if lang, ok := strings.CutPrefix(c, "language-"); ok {
					return getLanguageAnnotation(lang)
				}
			}
		}
		return ""
	}
	label := findByClass(container, "d813de27")
	if label == nil {
		return ""
	}
	return getLanguageAnnotation(getText(label))
}

func removeAttr(n *html.Node, key string) []html.Attribute {
	attrs := n.Attr[:0]
	for _, a := range n.Attr {
		if a.Key != key {
			attrs = append(attrs, a)
		}
	}
	return attrs
}

func findFirstElement(n *html.Node, tag string) *html.Node {
	if n.Type == html.ElementNode && n.Data == tag {
		return n
	}
	for child := n.FirstChild; child != nil; child = child.NextSibling {
		if found := findFirstElement(child, tag); found != nil {
			return found
		}
	}
	return nil
}

func getAttr(n *html.Node, key string) string {
	for _, a := range n.Attr {
		if a.Key == key {
			return a.Val
		}
	}
	return ""
}

func hasClass(n *html.Node, className string) bool {
	for _, c := range strings.Fields(getAttr(n, "class")) {
		if c == className {
			return true
		}
	}
	return false
}

func findByClass(n *html.Node, className string) *html.Node {
	if n.Type == html.ElementNode && hasClass(n, className) {
		return n
	}
	for child := n.FirstChild; child != nil; child = child.NextSibling {
		if found := findByClass(child, className); found != nil {
			return found
		}
	}
	return nil
}

func collectNodesWithClass(n *html.Node, className string, list *[]*html.Node) {
	if n.Type == html.ElementNode && hasClass(n, className) {
		*list = append(*list, n)
		return
	}
	for child := n.FirstChild; child != nil; child = child.NextSibling {
		collectNodesWithClass(child, className, list)
	}
}

func collectElements(n *html.Node, tag string, list *[]*html.Node) {
	if n.Type == html.ElementNode && n.Data == tag {
		*list = append(*list, n)
	}
	for child := n.FirstChild; child != nil; child = child.NextSibling {
		collectElements(child, tag, list)
	}
}

func getText(n *html.Node) string {
	if n.Type == html.TextNode {
		return n.Data
	}
	var sb strings.Builder
	for child := n.FirstChild; child != nil; child = child.NextSibling {
		sb.WriteString(getText(child))
	}
	return sb.String()
}

func removeNode(n *html.Node) {
	if n.Parent != nil {
		n.Parent.RemoveChild(n)
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
