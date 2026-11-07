package deepseek

import (
	"strings"
	"testing"
)

func TestTableInTurn(t *testing.T) {
	// A reply table as DeepSeek renders it: cells hold spans and code, and the
	// header sits in a thead.
	turn := `<div class="x ds-message"><div class="ds-markdown ds-assistant-message-main-content">` +
		`<p><strong>Key interception points</strong></p>` +
		`<table><thead><tr>` +
		`<th><span>Domain</span></th><th><span>gVisor mechanism</span></th></tr></thead>` +
		`<tbody><tr>` +
		`<td><span>VFS / file ops</span></td>` +
		`<td><code>SyscallTable.ExternalFilterBefore</code><span> and </span><code>SyscallFn</code></td>` +
		`</tr></tbody></table>` +
		`</div></div>`

	got := turnToMarkdown(turn)

	// The table keeps its header and rows, each cell on its own line. The
	// converter pads the cells, so the columns are not asserted exactly.
	for _, want := range []string{
		"## DeepSeek\n\n**Key interception points**",
		"| Domain         | gVisor mechanism",
		"|----------------|----",
		"| VFS / file ops | `SyscallTable.ExternalFilterBefore` and `SyscallFn` |",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("turnToMarkdown() =\n%s\nmissing %q", got, want)
		}
	}

	// The cells must not be run together the way the base converter leaves them.
	if strings.Contains(got, "DomaingVisor mechanism") {
		t.Error("table cells were concatenated")
	}
}
