package cmd

import (
	"os"

	"github.com/charmbracelet/log"
	"github.com/spf13/cobra"
)

var verbose int
var debug bool

// Injected by ldflags at build time
var (
	Version = "dev"
	Commit  = "none"
	Date    = "unknown"
)

var rootCmd = &cobra.Command{
	Use:   "tokfresh",
	Short: "Automate your Claude token reset timing",
	Long:  "TokFresh deploys a Cloudflare Worker that pre-triggers your Claude Pro/Max token reset on a schedule you set.",
	PersistentPreRun: func(cmd *cobra.Command, args []string) {
		setupLogging()
	},
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func init() {
	rootCmd.PersistentFlags().CountVarP(&verbose, "verbose", "v", "Increase verbosity (-v info, -vv debug)")
	rootCmd.PersistentFlags().BoolVar(&debug, "debug", false, "Enable debug logging (same as -vv)")
	rootCmd.CompletionOptions.HiddenDefaultCmd = true
}

func setupLogging() {
	log.SetOutput(os.Stderr)

	switch {
	case debug || verbose >= 2:
		log.SetLevel(log.DebugLevel)
		log.SetReportCaller(true)
	case verbose == 1:
		log.SetLevel(log.InfoLevel)
	default:
		log.SetLevel(log.WarnLevel)
	}
}
