package deepseek

import (
	"bytes"
	"fmt"
	"log"
	"net/url"
	"strings"
	"time"
	"unicode"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/input"
	downlaoder "github.com/zodimo/download-ai-chat/internal/downloader"
	"github.com/zodimo/download-ai-chat/internal/markdown"
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

// An uploaded file is rendered as a card holding its name and its type and
// size. The file's content is not on the page until the card is opened.
const (
	attachmentCardClass = "_158cea4"
	attachmentNameClass = "e70accd6"
	attachmentMetaClass = "_7103a25"
)

// The opened attachment is rendered as plain text in this block of a viewer
// that the share page shows over the conversation.
const (
	attachmentViewerClass = "a44e8e96"
	attachmentBodyClass   = "abbe3c45"
)

// attachmentLabelAttr marks an opened attachment with the name of the turn whose
// card opened it. The attribute is not DeepSeek's: openAttachments adds it to the
// page, so that the file can be matched to its card even though it shows one
// file at a time.
const attachmentLabelAttr = "data-attachment"

// maxAttachmentNameLen bounds the saved name of an attachment. The card shows the
// start of the file's content as its name, which can be a whole paragraph.
const maxAttachmentNameLen = 80

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

	return chatDownloader.Download(".ds-message", mkTurnProcessor(convID, url), openAttachments)
}
func (gp *DeepseekPlatform) Name() string {
	return providerName
}

func init() {
	providers.Register(providerName, &DeepseekPlatform{})
}

// mkTurnProcessor renders the turns of a shared conversation. The attachments a
// user uploaded are opened by openAttachments, which hands their content back as
// HTML, since the page drops the opened file once the next attachment is opened.
func mkTurnProcessor(convID, url string) func(turns rod.Elements, attachmentsHTML string) (*providers.ProcessedConversation, error) {
	return func(turns rod.Elements, attachmentsHTML string) (*providers.ProcessedConversation, error) {
		var sb strings.Builder
		dateStr := time.Now().Format("2006-01-02 15:04:05")

		sb.WriteString(fmt.Sprintf("# DeepSeek Conversation: %s\n\n", convID))
		sb.WriteString(fmt.Sprintf("> Date: %s\n", dateStr))
		sb.WriteString(fmt.Sprintf("> Source: %s\n\n", url))

		files := parseAttachmentFiles(attachmentsHTML)

		conv := providers.NewProcessedConversation(convID, url, "")
		turnNum := 0
		for i, turn := range turns {
			turnHTML, err := turn.HTML()
			if err != nil {
				log.Printf("Warning: could not get HTML for turn %d: %v", i+1, err)
				continue
			}
			doc := parseTurn(turnHTML)
			if doc == nil {
				continue
			}

			var attachmentRefs []string
			parseAttachmentCards(doc, files, &attachmentRefs, &conv.Attachments)
			sections := renderTurn(doc, attachmentRefs)
			if len(sections) == 0 {
				continue
			}
			if turnNum > 0 {
				sb.WriteString("---\n\n")
			}
			sb.WriteString(strings.Join(sections, "\n\n"))
			sb.WriteString("\n\n")
			turnNum++
		}

		if turnNum == 0 {
			return nil, fmt.Errorf("No conversation content could be extracted from the page.")
		}
		conv.Markdown = sb.String()
		return conv, nil
	}
}

// turnToMarkdown converts the outer HTML of one message element into the
// markdown for that turn.
func turnToMarkdown(turnHTML string) string {
	doc := parseTurn(turnHTML)
	if doc == nil {
		return ""
	}
	return strings.Join(renderParsedTurn(doc), "\n\n")
}

// parseTurn parses the outer HTML of a message element and preprocesses it,
// returning nil when the HTML cannot be parsed.
func parseTurn(turnHTML string) *html.Node {
	doc, err := html.Parse(strings.NewReader(turnHTML))
	if err != nil {
		log.Printf("Warning: failed to parse turn HTML: %v", err)
		return nil
	}
	preprocessHTML(doc)
	return doc
}

// renderParsedTurn renders a turn parsed by parseTurn, which has no opened
// attachments of its own.
func renderParsedTurn(turn *html.Node) []string {
	var attachmentRefs []string
	var attachments []providers.Attachment
	parseAttachmentCards(turn, nil, &attachmentRefs, &attachments)
	return renderTurn(turn, attachmentRefs)
}

// renderTurn builds the markdown sections of one parsed message element, which
// has been preprocessed and has lost its attachment cards to renderTurn's
// attachment references.
func renderTurn(turn *html.Node, attachmentRefs []string) []string {
	var sections []string

	userMD := nodeToMarkdown(findByClass(turn, userTurnClass), "user")
	switch {
	case userMD == "" && len(attachmentRefs) > 0:
		// The turn is only the uploaded files.
		userMD = strings.Join(attachmentRefs, "\n")
	case userMD == "":
		// A turn that only holds files the page would not open still names
		// them, so that the turn does not read as if it had no prompt.
		if cards := attachmentCardMarkdown(turn); cards != "" {
			userMD = cards
		}
	}
	if userMD != "" {
		sections = append(sections, "## User\n\n"+userMD)
	}

	reply := ""
	if reasoningMD := nodeToMarkdown(findByClass(turn, reasoningTurnClass), "reasoning"); reasoningMD != "" {
		reply = fmt.Sprintf("<details>\n<summary>%s</summary>\n\n%s\n\n</details>\n\n",
			reasoningLabel(turn), reasoningMD)
	}
	reply += nodeToMarkdown(findByClass(turn, assistantTurnClass), "assistant")
	if reply = strings.TrimSpace(reply); reply != "" {
		sections = append(sections, "## DeepSeek\n\n"+reply)
	}

	return sections
}

// parseAttachmentCards replaces every attachment card of a parsed turn with the
// reference to the file it links to, in the order the cards appear, so prompts
// keep the files where the user attached them. It also appends each file to the
// turn's attachments.
//
// The file's content is not on the page until the card is opened. openAttachments
// opens the cards and reads the file of each from the viewer that the page shows
// outside the conversation, keyed by the file's name.
func parseAttachmentCards(turn *html.Node, files map[string][]byte, refs *[]string, attachments *[]providers.Attachment) {
	var cards []*html.Node
	collectNodesWithClass(turn, attachmentCardClass, &cards)

	for _, card := range cards {
		label := findByClass(card, attachmentNameClass)
		if label == nil {
			continue
		}
		name := strings.Join(strings.Fields(getText(label)), " ")
		content, ok := files[name]
		if !ok {
			// Keep what the page shows in place of the file.
			log.Printf("Warning: attachment %q could not be read: it was not opened", name)
			continue
		}
		fileName := attachmentFileName(strings.TrimSpace(getText(label)), strings.TrimSpace(getText(findByClass(card, attachmentMetaClass))))
		fileName = uniqueAttachmentName(fileName, *attachments)
		// The link target is encoded so a name with spaces or parentheses does
		// not end the link early; the name itself stays readable.
		*refs = append(*refs, fmt.Sprintf("[%s](%s)", fileName, url.PathEscape(fileName)))
		*attachments = append(*attachments, providers.Attachment{Name: fileName, Content: content})

		ref := &html.Node{Type: html.TextNode, Data: (*refs)[len(*refs)-1]}
		card.Parent.InsertBefore(ref, card)
		removeNode(card)
	}
}

// parseAttachmentFiles reads the files that openAttachments saved into the HTML
// it handed back, keyed by the name of the file's card.
func parseAttachmentFiles(attachmentsHTML string) map[string][]byte {
	files := map[string][]byte{}
	if attachmentsHTML == "" {
		return files
	}
	doc, err := html.Parse(strings.NewReader(attachmentsHTML))
	if err != nil {
		log.Printf("Warning: failed to parse the opened attachments: %v", err)
		return files
	}
	// openAttachments keeps each opened file in a holder of its own, since the
	// page drops an opened file once the next one is opened.
	var holders []*html.Node
	collectNodesWithClass(doc, openedAttachmentClass, &holders)
	for _, holder := range holders {
		name := getAttr(holder, attachmentLabelAttr)
		if name == "" {
			continue
		}
		files[name] = []byte(getText(findByClass(holder, attachmentBodyClass)))
	}
	return files
}

// attachmentCardMarkdown names the attachment cards of a turn that could not be
// read, so the files the user attached are not lost from the transcript.
func attachmentCardMarkdown(turn *html.Node) string {
	var cards []*html.Node
	collectNodesWithClass(turn, attachmentCardClass, &cards)

	var names []string
	for _, card := range cards {
		label := findByClass(card, attachmentNameClass)
		if label == nil {
			continue
		}
		meta := strings.Join(strings.Fields(getText(findByClass(card, attachmentMetaClass))), " ")
		name := strings.Join(strings.Fields(getText(label)), " ")
		if meta != "" {
			name += " (" + meta + ")"
		}
		names = append(names, "- "+name)
	}
	return strings.Join(names, "\n")
}

// attachmentFileName turns the card's labels into the name of the file to save.
//
// The label is not a file name: it is the start of the file's own content, cut
// off mid-sentence and still carrying the markdown of its first line, as in
// "# TASK Go-Native Security Sandbox Architecture (.txt". Only the type from the
// metadata is a reliable file name, so the label is reduced to a slug and the
// type supplies the extension.
func attachmentFileName(label, meta string) string {
	ext := attachmentFileExt(meta)
	name := attachmentSlug(label, ext)
	if name == "" {
		name = "attachment"
	}
	return name + "." + ext
}

// attachmentFileExt reads the file's extension from the card's metadata, which
// holds the type and the size, as in "TXT 2.48KB".
func attachmentFileExt(meta string) string {
	fields := strings.Fields(meta)
	if len(fields) == 0 {
		return "bin"
	}
	ext := strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			return unicode.ToLower(r)
		}
		return -1
	}, fields[0])
	if ext == "" {
		return "bin"
	}
	return ext
}

// attachmentSlug reduces a card's label to something readable as a file name:
// the markdown of the first line is dropped, separators collapse to a single
// dash, and the extension the label shows is left to the metadata.
func attachmentSlug(label, ext string) string {
	name := strings.NewReplacer("\r", " ", "\n", " ", "\t", " ").Replace(label)
	name = strings.Join(strings.Fields(name), " ")

	// Drop the markdown that opens the first line, such as a heading marker.
	name = strings.TrimLeft(name, "#>*-_`~ ")
	// Drop a label that was cut off inside parentheses, as a long first line is
	// when the extension lands mid-word: "... Architecture (.txt".
	if i := strings.LastIndexByte(name, '('); i >= 0 && !strings.ContainsRune(name[i:], ')') {
		name = name[:i]
	}
	// Drop the extension the label ends with, since the metadata states it.
	name = strings.TrimSpace(name)
	if lower := strings.ToLower(name); ext != "" && strings.HasSuffix(lower, "."+ext) {
		name = name[:len(name)-len(ext)-1]
	}

	var sb strings.Builder
	separator := false
	for _, r := range name {
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			sb.WriteRune(r)
			separator = false
		// A dot or underscore only reads well between words.
		case (r == '.' || r == '_') && sb.Len() > 0 && !separator:
			sb.WriteRune(r)
		default:
			if sb.Len() > 0 && !separator {
				sb.WriteRune('-')
				separator = true
			}
		}
	}

	slug := strings.Trim(sb.String(), "-._")
	if len(slug) > maxAttachmentNameLen {
		slug = strings.TrimRight(slug[:maxAttachmentNameLen], "-._")
	}
	return slug
}

// uniqueAttachmentName keeps two files whose labels reduced to the same name from
// overwriting each other in the artifacts folder.
func uniqueAttachmentName(name string, taken []providers.Attachment) string {
	unique := name
	for n := 2; attachmentNameTaken(unique, taken); n++ {
		base, ext := unique, ""
		if i := strings.LastIndexByte(name, '.'); i > 0 {
			base, ext = name[:i], name[i:]
		}
		unique = fmt.Sprintf("%s-%d%s", base, n, ext)
	}
	return unique
}

func attachmentNameTaken(name string, taken []providers.Attachment) bool {
	for _, attachment := range taken {
		if attachment.Name == name {
			return true
		}
	}
	return false
}

// openAttachments opens every attachment card on the page and returns the HTML of
// the opened files, each tagged with the name of its card. The page shows one
// opened file at a time, so each file is copied out while it is on screen.
func openAttachments(page *rod.Page) (string, error) {
	var cards []*rod.Element
	if err := rod.Try(func() {
		cards = page.Timeout(10 * time.Second).MustElements("." + attachmentCardClass)
	}); err != nil {
		log.Printf("Warning: could not look for attachments: %v", err)
		return "", nil
	}
	if len(cards) == 0 {
		return "", nil
	}

	var opened strings.Builder
	for i, card := range cards {
		label, err := card.Element("." + attachmentNameClass)
		if err != nil {
			log.Printf("Warning: could not read attachment %d: %v", i+1, err)
			continue
		}
		name := strings.Join(strings.Fields(label.MustText()), " ")

		if err := rod.Try(func() { card.MustClick() }); err != nil {
			log.Printf("Warning: could not open attachment %q: %v", name, err)
			continue
		}
		if err := waitForAttachment(page); err != nil {
			log.Printf("Warning: could not open attachment %q: %v", name, err)
			continue
		}

		sel := "." + attachmentViewerClass + " ." + attachmentBodyClass
		// Copy the file out while it is on screen, into an element the turn
		// processor reads, tagged with the name of the card that opened it.
		if err := rod.Try(func() {
			page.Timeout(30 * time.Second).MustElement(sel)
			page.MustEval(`(args) => {
				const [sel, attr, name] = args;
				const body = document.querySelector(sel);
				if (!body) return false;
				const holder = document.createElement("div");
				holder.className = "`+openedAttachmentClass+`";
				holder.setAttribute(attr, name);
				const copy = document.createElement("div");
				copy.className = "`+attachmentBodyClass+`";
				copy.textContent = body.textContent;
				holder.appendChild(copy);
				document.body.appendChild(holder);
				return true;
			}`, []any{sel, attachmentLabelAttr, name})
		}); err != nil {
			log.Printf("Warning: could not read attachment %q: %v", name, err)
		}

		closeAttachmentViewer(page)
	}

	var holders []*rod.Element
	if err := rod.Try(func() {
		holders = page.MustElements("." + openedAttachmentClass)
	}); err != nil || len(holders) == 0 {
		return "", nil
	}
	for _, holder := range holders {
		h, err := holder.HTML()
		if err != nil {
			continue
		}
		opened.WriteString(h)
	}
	if len(holders) > 0 {
		log.Printf("Read %d attachment(s).", len(holders))
	}
	return opened.String(), nil
}

// openedAttachmentClass holds a copy of an opened attachment for the turn
// processor. The page closes the viewer once the next card is opened.
const openedAttachmentClass = "download-ai-chat-opened-attachment"

// waitForAttachment waits for the opened file to load. The viewer shows a
// loading placeholder until then, and stays on screen with the file's text.
func waitForAttachment(page *rod.Page) error {
	if err := rod.Try(func() {
		page.Timeout(30 * time.Second).MustElements("." + attachmentViewerClass)
	}); err != nil {
		return err
	}
	return page.Timeout(60 * time.Second).WaitStable(1500 * time.Millisecond)
}

// closeAttachmentViewer closes the viewer once its file has been read, so that
// the next card opens its own file instead of showing this one again.
func closeAttachmentViewer(page *rod.Page) {
	if err := rod.Try(func() { page.Keyboard.MustType(input.Escape) }); err != nil {
		log.Printf("Warning: could not close the attachment viewer: %v", err)
		return
	}
	if err := page.Timeout(10 * time.Second).WaitStable(500 * time.Millisecond); err != nil {
		log.Printf("Warning: the attachment viewer did not close in time: %v", err)
	}
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
	md, err := markdown.Convert(buf.String())
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
