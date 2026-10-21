package commands

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/zodimo/download-ai-chat/providers"
	_ "github.com/zodimo/download-ai-chat/providers/deepseek"
	_ "github.com/zodimo/download-ai-chat/providers/gemini"
)

var RootCmd = &cobra.Command{
	Use: "download-ai-chat",
	RunE: func(cmd *cobra.Command, args []string) error {
		cmd.SilenceUsage = true
		cmd.SilenceErrors = true

		url := viper.GetString("chaturl")
		outDir := viper.GetString("outdir")
		debug := viper.GetBool("debug")
		fileDateStr := time.Now().Format("2006-01-02-150405")
		fmt.Printf("Downloading url: %s\n", url)

		allproviders := providers.GetAllPlatforms()

		var platform providers.Platform

		for _, provider := range allproviders {
			fmt.Printf("Considering platform: %s\n", provider.UrlPrefix())
			if strings.HasPrefix(url, provider.UrlPrefix()) {
				platform = provider
				break
			}
		}

		if platform == nil {
			return fmt.Errorf("Could not derive plaform from url: %s\n", url)
		}

		processedConveration, err := platform.ConvertToMarkdown(url, debug)

		if err != nil {
			return fmt.Errorf("Could not process url with provider: %s\n %w", platform.Name(), err)
		}

		// Write output file.
		outFileName := fmt.Sprintf("%s-%s-%s.md", platform.Name(), processedConveration.ConversationID, fileDateStr)
		outPath := filepath.Join(outDir, outFileName)
		if err := os.MkdirAll(outDir, 0o755); err != nil {
			return fmt.Errorf("Could not create output directory %q: %v", outDir, err)
		}
		if err := os.WriteFile(outPath, []byte(processedConveration.Markdown), 0o644); err != nil {
			return fmt.Errorf("Could not write output file %q: %v", outPath, err)
		}

		fmt.Printf("Saved conversation to file: %s\n", outPath)

		return nil
	},
}

func init() {
	RootCmd.Flags().String("url", "", "the url of the shared chat")
	RootCmd.MarkFlagRequired("url")
	if err := viper.BindPFlag("chaturl", RootCmd.Flags().Lookup("url")); err != nil {
		panic(fmt.Sprintf("failed to bind --url flag: %v", err))
	}

	RootCmd.Flags().String("out", ".", "Directory to write the output markdown file.")
	if err := viper.BindPFlag("outdir", RootCmd.Flags().Lookup("out")); err != nil {
		panic(fmt.Sprintf("failed to bind --out flag: %v", err))
	}

	RootCmd.Flags().Bool("debug", false, "dump downloaded html to file")
	if err := viper.BindPFlag("debug", RootCmd.Flags().Lookup("debug")); err != nil {
		panic(fmt.Sprintf("failed to bind --out flag: %v", err))
	}
}

func Execute() {
	if err := RootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
