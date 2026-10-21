package providers

type Platform interface {
	Name() string
	UrlPrefix() string
	ConvertToMarkdown(url string, debug bool) (*ProcessedConversation, error)
}

type ProcessedConversation struct {
	ConversationID string
	Url            string
	Markdown       string
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
