package providers

// Platform is a chat provider that can turn a shared conversation URL into a
// ProcessedConversation.
type Platform interface {
	Name() string
	UrlPrefix() string
	ConvertToMarkdown(url string, debug bool) (*ProcessedConversation, error)
}

// Attachment is a file the user uploaded to the conversation. The markdown
// refers to it by Name, and the command writes its content to a file with that
// name next to the markdown.
type Attachment struct {
	Name    string
	Content []byte
}

type ProcessedConversation struct {
	ConversationID string
	Url            string
	Markdown       string
	Attachments    []Attachment
}

func NewProcessedConversation(
	conversationID string,
	url string,
	markdown string,
) *ProcessedConversation {
	return &ProcessedConversation{
		ConversationID: conversationID,
		Url:            url,
		Markdown:       markdown,
	}
}
