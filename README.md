# Download AI chat

The goal is to be able to download a shared chat link to local markdown file

# Support for providers
- gemini - "https://share.gemini.google/{session-id}"
- deepseek - "https://chat.deepseek.com/share/{session-id}"

## Installation

You can install the tool directly using `go install`:

```bash
go install github.com/zodimo/download-ai-chat/cmd/download-ai-chat@latest
```

## Usage

Once installed, you can run the tool from anywhere:

```bash
download-ai-chat --url https://gemini.google.com/share/{session-id}
# or
download-ai-chat --url https://chat.deepseek.com/share/{session-id}
```

Alternatively, run it directly from the source code without installing:

```bash
go run ./cmd/download-ai-chat/ --url https://gemini.google.com/share/{session-id}
#  or
go run ./cmd/download-ai-chat/ --url https://chat.deepseek.com/share/{session-id}
```

### Flags

- `--url` **(Required)**: The URL of the shared conversation.
- `--out` *(Optional)*: The directory to write the output markdown file. Defaults to `.`.

## Output

The script extracts the conversation ID from the URL and creates a markdown file in the specified output directory.
The generated file will be named like `<ProviderName>-<ConversationID>-<YYYY-MM-DD-HHMMSS>.md`.

Each file contains the conversation's metadata (ID, date, source URL) followed by the alternating turns of the **User** and **LLM**.

### Attachments

Files the user uploaded are saved next to the markdown, in a folder named after the output file:

```
deepseek-<ConversationID>-<YYYY-MM-DD-HHMMSS>.md
deepseek-<ConversationID>-<YYYY-MM-DD-HHMMSS>-artifacts/<file name>.<type>
```

The markdown refers to each file where the user attached it, as `[name](name)`, so the links work from the
markdown file. The folder is only created when the conversation has attachments.

The card DeepSeek renders for an upload does not show a file name: it shows the start of the file's own content,
cut off mid-sentence. Saved files are therefore named from that text, reduced to a slug, with the type taken from
the card's type label. A prompt starting with `# TASK Go-Native Security Sandbox Architecture (gVisor + nono
Integration)` saved as a text file becomes `TASK-Go-Native-Security-Sandbox-Architecture.txt`.

DeepSeek only loads an uploaded file when its card is clicked, so the downloader opens each card to read the
file's content before saving it.
