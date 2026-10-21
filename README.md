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
