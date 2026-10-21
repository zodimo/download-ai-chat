package urlhelper

import (
	"net/url"
	"strings"
)

// conversationID extracts the last path segment from a Gemini share URL.
// e.g. gemini "https://gemini.google.com/share/123434" → "123434"
// e.g. deepseek "https://chat.deepseek.com/share/123434" → "123434"
func ConversationID(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}
	segments := strings.Split(strings.Trim(u.Path, "/"), "/")
	if len(segments) == 0 {
		return ""
	}
	return segments[len(segments)-1]
}
