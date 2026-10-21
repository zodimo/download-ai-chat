package main

import (
	"github.com/spf13/cobra"
	"github.com/zodimo/download-ai-chat/internal/commands"
)

func init() {
	cobra.EnableTraverseRunHooks = true
}

func main() {
	commands.Execute()
}
