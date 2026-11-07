// Package markdown converts the HTML of a chat turn into markdown.
package markdown

import (
	"github.com/JohannesKaufmann/html-to-markdown/v2/converter"
	"github.com/JohannesKaufmann/html-to-markdown/v2/plugin/base"
	"github.com/JohannesKaufmann/html-to-markdown/v2/plugin/commonmark"
	"github.com/JohannesKaufmann/html-to-markdown/v2/plugin/table"
)

// converterWithTables renders the tables of a turn as markdown tables. Without
// the table plugin the base converter drops a <table> together with its cells,
// leaving the text of every cell run together in the surrounding paragraph.
var converterWithTables = converter.NewConverter(
	converter.WithPlugins(
		base.NewBasePlugin(),
		commonmark.NewCommonmarkPlugin(),
		table.NewTablePlugin(),
	),
)

// Convert returns the markdown for a piece of turn HTML.
func Convert(html string) (string, error) {
	return converterWithTables.ConvertString(html)
}
