package cmd

import (
	"fmt"
	"time"

	"github.com/spf13/cobra"
	"github.com/stevejkang/tokfresh-cli/internal/cloudflare"
	"github.com/stevejkang/tokfresh-cli/internal/config"
	"github.com/stevejkang/tokfresh-cli/internal/schedule"
	"github.com/stevejkang/tokfresh-cli/internal/ui"
)

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "List all managed TokFresh instances",
	Long:  "Shows a styled table of all configured TokFresh worker instances and the next scheduled trigger time.",
	RunE:  runStatus,
}

func init() {
	rootCmd.AddCommand(statusCmd)
}

func runStatus(cmd *cobra.Command, args []string) error {
	instances := config.ListInstances()

	if len(instances) == 0 {
		fmt.Println()
		fmt.Println(ui.MutedStyle.Render("  No TokFresh instances configured. Run `tokfresh init` to get started."))
		fmt.Println()
		return nil
	}

	fmt.Println()
	fmt.Println(ui.BoldStyle.Render("  TokFresh Instances"))
	fmt.Println()
	fmt.Println(ui.RenderStatusTable(instances))
	fmt.Println()

	for _, inst := range instances {
		fmt.Printf("  %s → %s\n", inst.Name, cloudflare.ConsoleURL(inst.CloudflareAccountID, inst.Name))
	}
	fmt.Println()

	var soonestTime time.Time
	var soonestLabel string
	var soonestName string
	for _, inst := range instances {
		slots := schedule.Calculate(inst.Schedule)
		triggerTime, label := schedule.GetNextTrigger(slots, inst.Timezone)
		if soonestTime.IsZero() || triggerTime.Before(soonestTime) {
			soonestTime = triggerTime
			soonestLabel = label
			soonestName = inst.Name
		}
	}

	if soonestLabel != "" {
		fmt.Printf("  Next: %s at %s\n", soonestName, soonestLabel)
		fmt.Println()
	}

	return nil
}
