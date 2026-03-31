package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"time"

	"github.com/charmbracelet/log"
	"github.com/spf13/cobra"
	"github.com/stevejkang/tokfresh-cli/internal/cloudflare"
	"github.com/stevejkang/tokfresh-cli/internal/config"
	"github.com/stevejkang/tokfresh-cli/internal/ui"
	"nhooyr.io/websocket"
)

var logsCmd = &cobra.Command{
	Use:   "logs <name>",
	Short: "Stream live worker execution logs",
	Long:  "Creates a Cloudflare Tail session and streams real-time worker events via WebSocket. Press Ctrl+C to stop.",
	Args:  cobra.ExactArgs(1),
	RunE:  runLogs,
}

func init() {
	rootCmd.AddCommand(logsCmd)
}

type tailEvent struct {
	Outcome    string      `json:"outcome"`
	ScriptName string      `json:"scriptName"`
	Logs       []tailLog   `json:"logs"`
	Exceptions []tailError `json:"exceptions"`
	EventTS    *int64      `json:"eventTimestamp"`
}

type tailLog struct {
	Message []interface{} `json:"message"`
	Level   string        `json:"level"`
}

type tailError struct {
	Name    string `json:"name"`
	Message string `json:"message"`
}

func runLogs(cmd *cobra.Command, args []string) error {
	name := args[0]

	inst := config.GetInstance(name)
	if inst == nil {
		return instanceNotFoundError(name)
	}

	auth, err := resolveCloudflareAuthAuto()
	if err != nil {
		return err
	}

	accountID := auth.AccountID
	if accountID == "" {
		accountID = inst.CloudflareAccountID
	}

	// Create tail session
	fmt.Println()
	fmt.Printf("  %s\n", ui.MutedStyle.Render(fmt.Sprintf("Tailing %s... (Ctrl+C to stop)", name)))
	fmt.Println()

	tailID, wsURL, err := cloudflare.CreateTail(accountID, auth.Token, name)
	if err != nil {
		return fmt.Errorf("failed to create tail session: %w\n  Hint: check token permissions or visit https://dash.cloudflare.com", err)
	}
	log.Debug("tail session created", "id", tailID, "ws_url", wsURL)

	// Ensure cleanup on exit
	defer func() {
		log.Debug("cleaning up tail session", "id", tailID)
		if delErr := cloudflare.DeleteTail(accountID, auth.Token, name, tailID); delErr != nil {
			log.Warn("failed to delete tail session", "error", delErr)
		}
	}()

	// Connect WebSocket with trace-v1 subprotocol
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle Ctrl+C
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt)
	go func() {
		<-sigCh
		fmt.Println()
		fmt.Println(ui.MutedStyle.Render("  Stopping tail..."))
		cancel()
	}()

	conn, _, dialErr := websocket.Dial(ctx, wsURL, &websocket.DialOptions{
		Subprotocols: []string{"trace-v1"},
	})
	if dialErr != nil {
		return fmt.Errorf("WebSocket connection failed: %w", dialErr)
	}
	defer conn.CloseNow()

	// Read and display events
	for {
		_, message, readErr := conn.Read(ctx)
		if readErr != nil {
			if ctx.Err() != nil {
				return nil
			}
			return fmt.Errorf("WebSocket read error: %w", readErr)
		}

		var event tailEvent
		if jsonErr := json.Unmarshal(message, &event); jsonErr != nil {
			log.Debug("failed to parse tail event", "error", jsonErr, "raw", string(message))
			continue
		}

		printTailEvent(&event)
	}
}

func printTailEvent(event *tailEvent) {
	ts := time.Now().Format("2006-01-02 15:04:05")
	if event.EventTS != nil {
		ts = time.UnixMilli(*event.EventTS).Format("2006-01-02 15:04:05")
	}

	outcomeIcon := ui.SuccessStyle.Render("✓")
	outcomeText := event.Outcome
	if event.Outcome == "exception" || event.Outcome == "exceededCpu" || event.Outcome == "canceled" {
		outcomeIcon = ui.ErrorStyle.Render("✗")
	}

	// Print main event line
	fmt.Printf("  %s %s %s", ts, outcomeIcon, outcomeText)

	// Print log messages
	for _, l := range event.Logs {
		for _, msg := range l.Message {
			fmt.Printf("  %s", msg)
		}
	}

	// Print exceptions
	for _, e := range event.Exceptions {
		fmt.Printf("  %s", ui.ErrorStyle.Render(fmt.Sprintf("%s: %s", e.Name, e.Message)))
	}

	fmt.Println()
}
