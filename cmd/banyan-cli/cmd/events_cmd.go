package cmd

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/fertile-org/banyan/cmd/banyan-cli/dashboard"
)

var eventsTail int

var eventsCmd = &cobra.Command{
	Use:   "events",
	Short: "List recent cluster events",
	Long: `List recent cluster events with timestamp, severity, type, and message.

Examples:
  banyan-cli events
  banyan-cli events --tail 10
  banyan-cli events -o json`,
	RunE: runEvents,
}

func init() {
	rootCmd.AddCommand(eventsCmd)
	eventsCmd.Flags().IntVar(&eventsTail, "tail", 50, "Number of events to show")
	eventsCmd.Flags().StringVarP(&outputFormat, "output", "o", "", "Output format (json)")
}

func runEvents(_ *cobra.Command, _ []string) error {
	data, err := fetchClusterData()
	if err != nil {
		return err
	}

	events := data.Events
	if eventsTail > 0 && eventsTail < len(events) {
		events = events[len(events)-eventsTail:]
	}

	return listEvents(events)
}

func listEvents(events []dashboard.EventData) error {
	if outputFormat == "json" {
		return printJSON(events)
	}

	if len(events) == 0 {
		fmt.Println("No events found")
		return nil
	}

	fmt.Printf("%-20s %-10s %-25s %s\n",
		"TIMESTAMP", "SEVERITY", "TYPE", "MESSAGE")
	fmt.Println(strings.Repeat("-", 90))

	for _, e := range events {
		ts := e.Timestamp.Format("2006-01-02 15:04:05")
		fmt.Printf("%-20s %-10s %-25s %s\n",
			ts, e.Severity, e.Type, e.Message)
	}

	return nil
}
