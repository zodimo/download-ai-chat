package deepseek

import (
	"bytes"
	"net/url"
	"os"
	"strings"
	"testing"

	"github.com/zodimo/download-ai-chat/providers"
	"golang.org/x/net/html"
)

// attachmentPage is a saved page holding an uploaded file. Its card carries no
// content: the content sits in a viewer that openAttachments has marked with the
// name of the turn whose card opened it.
const attachmentPage = "testdata/attachment.html"

// loadAttachmentPage parses the fixture and returns its document with its
// message elements. The file's content sits in a viewer outside the turns, so
// the turns have to be looked at within the page they were parsed from.
func loadAttachmentPage(t *testing.T) (*html.Node, []*html.Node) {
	t.Helper()

	raw, err := os.ReadFile(attachmentPage)
	if err != nil {
		t.Skipf("saved page not available: %v", err)
	}
	doc, err := html.Parse(bytes.NewReader(raw))
	if err != nil {
		t.Fatalf("parse %s: %v", attachmentPage, err)
	}

	var messages []*html.Node
	collectNodesWithClass(doc, "ds-message", &messages)
	if len(messages) == 0 {
		t.Fatalf("no ds-message elements in %s", attachmentPage)
	}
	return doc, messages
}

// openedFiles reads the fixture's viewer the way openAttachments hands the page's
// files to the turn processor: as HTML tagged with the card that opened the file.
// It returns the files and that HTML.
func openedFiles(t *testing.T, page *html.Node, turn *html.Node) ([]attachmentFile, string) {
	t.Helper()

	var viewers []*html.Node
	collectNodesWithClass(page, attachmentViewerClass, &viewers)
	if len(viewers) != 1 {
		t.Fatalf("expected 1 viewer in %s, got %d", attachmentPage, len(viewers))
	}

	// Mark the viewer as openAttachments marks the page's viewer, then copy it
	// out the way openAttachments copies the open file.
	name := cardLabel(t, turn)
	viewers[0].Attr = append(viewers[0].Attr, html.Attribute{Key: attachmentLabelAttr, Val: name})
	holder := &html.Node{
		Type: html.ElementNode, Data: "div",
		Attr: []html.Attribute{
			{Key: "class", Val: openedAttachmentClass},
			{Key: attachmentLabelAttr, Val: name},
			{Key: attachmentIndexAttr, Val: "0"},
		},
	}
	holder.AppendChild(copyNode(viewers[0]))
	holders := &html.Node{Type: html.ElementNode, Data: "body"}
	holders.AppendChild(holder)

	var buf bytes.Buffer
	if err := html.Render(&buf, holders); err != nil {
		t.Fatalf("render opened files: %v", err)
	}
	opened := buf.String()
	return parseAttachmentFiles(opened), opened
}

// copyNode deep-copies a parsed node.
func copyNode(n *html.Node) *html.Node {
	c := *n
	c.Parent, c.FirstChild, c.LastChild, c.PrevSibling, c.NextSibling = nil, nil, nil, nil, nil
	for child := n.FirstChild; child != nil; child = child.NextSibling {
		c.AppendChild(copyNode(child))
	}
	return &c
}

func TestAttachmentIsReferencedAndSaved(t *testing.T) {
	page, messages := loadAttachmentPage(t)
	turn := messages[0]
	files, _ := openedFiles(t, page, turn)

	var refs []string
	var attachments []providers.Attachment
	parseAttachmentCards(turn, &files, &refs, &attachments)

	if len(attachments) != 1 {
		t.Fatalf("expected 1 attachment, got %d", len(attachments))
	}
	got := attachments[0]
	want := "TASK-Go-Native-Security-Sandbox-Architecture.txt"
	if got.Name != want {
		t.Errorf("attachment name = %q, want %q", got.Name, want)
	}
	// The link target is encoded, so that a name with spaces and parentheses
	// does not end the link early; the link text stays the file's name.
	wantRef := "[" + want + "](" + url.PathEscape(want) + ")"
	if len(refs) != 1 || refs[0] != wantRef {
		t.Errorf("references = %q, want %q", refs, wantRef)
	}

	// The saved file holds the text the page showed, not the card's labels.
	content := string(got.Content)
	for _, piece := range []string{
		"# TASK: Go-Native Security Sandbox Architecture (gVisor + nono Integration)",
		"## 3. CONSTRAINTS & STYLES",
	} {
		if !strings.Contains(content, piece) {
			t.Errorf("attachment content is missing %q", piece)
		}
	}
	if strings.Contains(content, "TXT 2.48KB") {
		t.Error("attachment content holds the card's labels")
	}

	// The card is gone from the prompt, replaced by the reference to the file.
	md := strings.Join(renderTurn(turn, refs), "\n\n")
	if !strings.Contains(md, "## User\n\n"+wantRef) {
		t.Errorf("user turn does not reference the attachment:\n%s", md)
	}
	if strings.Contains(md, "TXT 2.48KB") {
		t.Errorf("user turn still holds the card labels:\n%s", md)
	}
	// The reply is in the next message of the fixture and is untouched.
	reply := strings.Join(renderTurn(messages[1], nil), "\n\n")
	if !strings.HasPrefix(reply, "## DeepSeek\n\n") || !strings.Contains(reply, "Plan written.") {
		t.Errorf("reply was lost:\n%s", reply)
	}
}

// A prompt that holds both text and an uploaded file is the common shape: the
// user typed around the file. The reference has to survive the converter as a
// link there, not just in a turn that is nothing but the file.
func TestAttachmentInsidePromptIsALink(t *testing.T) {
	page, messages := loadAttachmentPage(t)
	files, _ := openedFiles(t, page, messages[0])

	// Rebuild the first message with the card inside a prompt.
	turnHTML := renderHTML(t, messages[0])
	turn, err := html.Parse(strings.NewReader(turnHTML))
	if err != nil {
		t.Fatalf("parse turn: %v", err)
	}
	card := findByClass(turn, attachmentCardClass)
	prompt := &html.Node{Type: html.ElementNode, Data: "div", Attr: []html.Attribute{{Key: "class", Val: userTurnClass}}}
	inner := &html.Node{Type: html.ElementNode, Data: "div"}
	inner.AppendChild(&html.Node{Type: html.TextNode, Data: "please review "})
	card.Parent.RemoveChild(card)
	inner.AppendChild(card)
	prompt.AppendChild(inner)
	turn.FirstChild.AppendChild(prompt)

	var refs []string
	var attachments []providers.Attachment
	parseAttachmentCards(turn, &files, &refs, &attachments)
	if len(attachments) != 1 {
		t.Fatalf("expected 1 attachment, got %d", len(attachments))
	}
	want := "TASK-Go-Native-Security-Sandbox-Architecture.txt"
	wantRef := "[" + want + "](" + url.PathEscape(want) + ")"

	md := strings.Join(renderTurn(turn, refs), "\n\n")
	if !strings.Contains(md, "please review "+wantRef) {
		t.Errorf("prompt does not link the attachment in place:\n%s", md)
	}
	// The converter escapes brackets it finds in text, which turns a hand-built
	// reference into "\\[name](name)". The file must be a real link instead.
	if strings.Contains(md, `\[`) || strings.Contains(md, `\]`) {
		t.Errorf("attachment reference was escaped in the prompt:\n%s", md)
	}
}

func TestAttachmentWithoutOpenedFileKeepsTheCard(t *testing.T) {
	_, messages := loadAttachmentPage(t)
	turn := messages[0]

	// Nothing was opened, so the turn has no files to refer to.
	var files []attachmentFile
	var refs []string
	var attachments []providers.Attachment
	parseAttachmentCards(turn, &files, &refs, &attachments)

	if len(attachments) != 0 || len(refs) != 0 {
		t.Fatalf("expected no attachments, got %d attachments and %d references", len(attachments), len(refs))
	}
	// The card stays in the prompt as the page rendered it, so the files the
	// user attached are still named even though they could not be read.
	md := strings.Join(renderTurn(turn, refs), "\n\n")
	if !strings.Contains(md, "## User") {
		t.Errorf("turn with an unreadable attachment lost its user section:\n%s", md)
	}
	if !strings.Contains(md, "# TASK Go-Native Security Sandbox Architecture (.txt") {
		t.Errorf("turn with an unreadable attachment lost the file's name:\n%s", md)
	}
}

func TestAttachmentFileName(t *testing.T) {
	tests := []struct {
		name  string
		label string
		meta  string
		want  string
	}{
		{
			// The label is the file's first line, cut off mid-sentence and still
			// carrying its markdown, so it has to be reduced to a file name.
			name:  "content label with markdown and type",
			label: "# TASK Go-Native Security Sandbox Architecture (.txt",
			meta:  "TXT 2.48KB",
			want:  "TASK-Go-Native-Security-Sandbox-Architecture.txt",
		},
		{
			name:  "label without the extension gets it from the type",
			label: "quarterly report",
			meta:  "PDF 1.2MB",
			want:  "quarterly-report.pdf",
		},
		{
			name:  "multi-line label is flattened",
			label: "line one\nline two",
			meta:  "PDF 1.2MB",
			want:  "line-one-line-two.pdf",
		},
		{
			name:  "long label is cut",
			label: strings.Repeat("a", 200),
			meta:  "PNG 12KB",
			want:  strings.Repeat("a", 80) + ".png",
		},
		{
			name:  "path characters are replaced",
			label: "a/b:c*d?e",
			meta:  "CSV 1KB",
			want:  "a-b-c-d-e.csv",
		},
		{
			name:  "markers and quotes are dropped",
			label: "## \"Quoted note\" — part 2",
			meta:  "MD 1KB",
			want:  "Quoted-note-part-2.md",
		},
		{
			name:  "type keeps only letters and digits",
			label: "diagram",
			meta:  "SVG. 1.4KB",
			want:  "diagram.svg",
		},
		{
			name:  "missing type falls back to bin",
			label: "notes",
			meta:  "",
			want:  "notes.bin",
		},
		{
			name:  "empty label still gets a name",
			label: " ",
			meta:  "",
			want:  "attachment.bin",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := attachmentFileName(tt.label, tt.meta); got != tt.want {
				t.Errorf("attachmentFileName(%q, %q) = %q, want %q", tt.label, tt.meta, got, tt.want)
			}
		})
	}
}

// Two uploads can look alike: both cards show the start of the file's content, so
// two files that begin the same way have the same label. Each file still has to
// reach the turn that holds its card, so the files are matched by the order their
// cards appear rather than by their label.
func TestLookAlikeAttachmentsStayInTheirOwnTurns(t *testing.T) {
	card := func(label, meta, text string) *html.Node {
		first := &html.Node{Type: html.ElementNode, Data: "div", Attr: []html.Attribute{{Key: "class", Val: attachmentNameClass}}}
		first.AppendChild(&html.Node{Type: html.TextNode, Data: label})
		metaNode := &html.Node{Type: html.ElementNode, Data: "div", Attr: []html.Attribute{{Key: "class", Val: attachmentMetaClass}}}
		metaNode.AppendChild(&html.Node{Type: html.TextNode, Data: meta})
		card := &html.Node{Type: html.ElementNode, Data: "div", Attr: []html.Attribute{{Key: "class", Val: attachmentCardClass}}}
		card.AppendChild(first)
		card.AppendChild(metaNode)
		prompt := &html.Node{Type: html.ElementNode, Data: "div", Attr: []html.Attribute{{Key: "class", Val: userTurnClass}}}
		inner := &html.Node{Type: html.ElementNode, Data: "div"}
		inner.AppendChild(&html.Node{Type: html.TextNode, Data: text})
		inner.AppendChild(card)
		prompt.AppendChild(inner)
		turn := &html.Node{Type: html.ElementNode, Data: "div", Attr: []html.Attribute{{Key: "class", Val: "ds-message"}}}
		turn.AppendChild(prompt)
		return turn
	}

	// Two cards that look alike, in the order the page shows them.
	files := []attachmentFile{
		{Card: "# notes", Content: []byte("first file")},
		{Card: "# notes", Content: []byte("second file")},
	}
	turns := []*html.Node{
		card("# notes", "TXT 1KB", "first: "),
		card("# notes", "TXT 1KB", "second: "),
	}

	// The attachments accumulate across the turns, as they do in the transcript.
	var names []string
	var collected []providers.Attachment
	for i, turn := range turns {
		var refs []string
		parseAttachmentCards(turn, &files, &refs, &collected)
		if len(collected) != i+1 {
			t.Fatalf("turn %d: expected %d attachment(s), got %d", i+1, i+1, len(collected))
		}
		latest := collected[len(collected)-1]
		names = append(names, latest.Name)
		if got, want := string(latest.Content), []string{"first file", "second file"}[i]; got != want {
			t.Errorf("turn %d holds %q, want %q", i+1, got, want)
		}
	}

	// The files are alike by label, so the second gets a name that keeps it from
	// overwriting the first in the artifacts folder.
	if len(names) != 2 || names[0] == names[1] {
		t.Errorf("attachment names = %q, want two different names", names)
	}
	if len(files) != 0 {
		t.Errorf("%d file(s) were left unclaimed", len(files))
	}
}

func TestUniqueAttachmentName(t *testing.T) {
	// Two files whose labels reduce to the same name must not overwrite each
	// other in the artifacts folder.
	taken := []providers.Attachment{{Name: "notes.txt"}}
	if got := uniqueAttachmentName("notes.txt", taken); got != "notes-2.txt" {
		t.Errorf("uniqueAttachmentName() = %q, want %q", got, "notes-2.txt")
	}

	taken = append(taken, providers.Attachment{Name: "notes-2.txt"})
	if got := uniqueAttachmentName("notes.txt", taken); got != "notes-3.txt" {
		t.Errorf("uniqueAttachmentName() = %q, want %q", got, "notes-3.txt")
	}

	// A name that is still free is kept as it is.
	if got := uniqueAttachmentName("other.txt", taken); got != "other.txt" {
		t.Errorf("uniqueAttachmentName() = %q, want %q", got, "other.txt")
	}
}

// cardLabel is the name of the attachment card of a turn, read the way
// openAttachments reads it.
func cardLabel(t *testing.T, turn *html.Node) string {
	t.Helper()

	var cards []*html.Node
	collectNodesWithClass(turn, attachmentCardClass, &cards)
	if len(cards) != 1 {
		t.Fatalf("expected 1 card in the turn, got %d", len(cards))
	}
	label := findByClass(cards[0], attachmentNameClass)
	if label == nil {
		t.Fatal("card has no name")
	}
	return strings.Join(strings.Fields(getText(label)), " ")
}

func renderHTML(t *testing.T, node *html.Node) string {
	t.Helper()
	var buf bytes.Buffer
	if err := html.Render(&buf, node); err != nil {
		t.Fatalf("render: %v", err)
	}
	return buf.String()
}
