package dashboard

import (
	"encoding/csv"
	"fmt"
	"io"
	"os"
	"strings"
	"time"
)

// exportViewCSV writes the current view data to a CSV file and returns the filename.
// Declared as a variable for test injection.
var exportViewCSV = defaultExportViewCSV

func defaultExportViewCSV(data *DashboardData, view View) (string, error) {
	if data == nil {
		return "", fmt.Errorf("no data to export")
	}

	filename := fmt.Sprintf("banyan-%s-%s.csv", viewExportName(view), time.Now().Format("2006-01-02-150405"))

	f, err := os.Create(filename)
	if err != nil {
		return "", fmt.Errorf("create file: %w", err)
	}
	defer f.Close()

	if err := writeCSV(f, data, view); err != nil {
		return "", err
	}

	return filename, nil
}

// writeCSV writes CSV data for the given view to the writer.
func writeCSV(w io.Writer, data *DashboardData, view View) error {
	cw := csv.NewWriter(w)

	// writeErr captures the first write error; writeRow short-circuits after it.
	var writeErr error
	writeRow := func(record []string) {
		if writeErr == nil {
			writeErr = cw.Write(record)
		}
	}

	switch view {
	case ViewEvents:
		writeRow([]string{"Time", "Type", "Severity", "Message"})
		for _, e := range data.Events {
			writeRow([]string{
				e.Timestamp.Format("2006-01-02 15:04:05"),
				e.Type,
				e.Severity,
				e.Message,
			})
		}
	case ViewContainers:
		writeRow([]string{"Container", "Service", "Agent", "Deployment", "Status", "Ports"})
		for i := range data.Containers {
			c := &data.Containers[i]
			status := c.ContainerStatus
			if status == "" {
				status = c.Status
			}
			writeRow([]string{
				c.Name,
				c.ServiceName,
				c.AgentName,
				c.DeploymentName,
				status,
				strings.Join(c.Ports, ";"),
			})
		}
	case ViewAgents:
		writeRow([]string{"Name", "Status", "Tags", "CPU%", "Memory Used", "Memory Total", "Disk Used", "Disk Total", "Containers"})
		for i := range data.Agents {
			a := &data.Agents[i]
			writeRow([]string{
				a.Name,
				a.Status,
				strings.Join(a.Tags, ";"),
				fmt.Sprintf("%.1f", a.CPU*100),
				formatBytes(a.MemUsed),
				formatBytes(a.MemTotal),
				formatBytes(a.DiskUsed),
				formatBytes(a.DiskTotal),
				fmt.Sprintf("%d", a.ContainerCount),
			})
		}
	case ViewDeploys:
		writeRow([]string{"Name", "ID", "Status", "Healthy", "Total", "Services", "Tags", "Created"})
		for i := range data.Deployments {
			d := &data.Deployments[i]
			writeRow([]string{
				d.Name,
				d.ID,
				d.Status,
				fmt.Sprintf("%d", d.Healthy),
				fmt.Sprintf("%d", d.Total),
				fmt.Sprintf("%d", d.Services),
				strings.Join(d.Tags, ";"),
				d.CreatedAt.Format("2006-01-02 15:04:05"),
			})
		}
	default:
		return fmt.Errorf("export not supported for this view")
	}

	if writeErr != nil {
		return writeErr
	}
	cw.Flush()
	return cw.Error()
}

// viewExportName returns a filesystem-friendly name for the view.
func viewExportName(v View) string {
	switch v {
	case ViewAgents:
		return "agents"
	case ViewDeploys:
		return "deployments"
	case ViewContainers:
		return "containers"
	case ViewEvents:
		return "events"
	default:
		return "data"
	}
}
