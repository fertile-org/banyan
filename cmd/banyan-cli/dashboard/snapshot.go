package dashboard

import (
	"fmt"
	"html/template"
	"io"
	"strings"
	"time"
)

// snapshotHTML is the Go template for the self-contained HTML snapshot.
var snapshotHTML = template.Must(template.New("snapshot").Funcs(template.FuncMap{
	"statusClass": func(s string) string {
		switch s {
		case "running", "ready", "up", "completed":
			return "green"
		case "pending", "deploying":
			return "yellow"
		case "failed", "error", "stale":
			return "red"
		default:
			return "gray"
		}
	},
	"fmtBytes": formatBytes,
	"fmtDuration": func(t time.Time) string {
		if t.IsZero() {
			return "-"
		}
		return formatDuration(time.Since(t))
	},
	"fmtPercent": func(ratio float64) string {
		return fmt.Sprintf("%.1f%%", ratio*100)
	},
	"joinPorts": func(ports []string) string {
		if len(ports) == 0 {
			return "-"
		}
		return strings.Join(ports, ", ")
	},
}).Parse(`<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>Banyan Cluster Snapshot</title>
<style>
  :root {
    --bg: #1a1a2e;
    --surface: #16213e;
    --border: #558B2F;
    --primary: #7CB342;
    --text: #e0e0e0;
    --dim: #9e9e9e;
    --green: #66BB6A;
    --yellow: #FFA726;
    --red: #EF5350;
    --gray: #757575;
    --blue: #42A5F5;
  }
  * { box-sizing: border-box; margin: 0; padding: 0; }
  body {
    font-family: 'SF Mono', 'Fira Code', 'JetBrains Mono', monospace;
    background: var(--bg);
    color: var(--text);
    padding: 24px;
    line-height: 1.5;
  }
  h1 {
    color: var(--primary);
    font-size: 1.4rem;
    margin-bottom: 4px;
  }
  .meta {
    color: var(--dim);
    font-size: 0.8rem;
    margin-bottom: 20px;
  }
  .grid {
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(180px, 1fr));
    gap: 12px;
    margin-bottom: 20px;
  }
  .card {
    background: var(--surface);
    border: 1px solid var(--border);
    border-radius: 6px;
    padding: 12px;
  }
  .card .label { color: var(--dim); font-size: 0.75rem; text-transform: uppercase; }
  .card .value { font-size: 1.5rem; font-weight: bold; color: var(--primary); }
  .section { margin-bottom: 20px; }
  .section h2 {
    color: var(--primary);
    font-size: 1rem;
    border-bottom: 1px solid var(--border);
    padding-bottom: 4px;
    margin-bottom: 8px;
  }
  table {
    width: 100%;
    border-collapse: collapse;
    font-size: 0.85rem;
  }
  th {
    text-align: left;
    color: var(--dim);
    font-weight: 600;
    padding: 6px 10px;
    border-bottom: 1px solid var(--border);
  }
  td {
    padding: 6px 10px;
    border-bottom: 1px solid rgba(85,139,47,0.2);
  }
  .dot {
    display: inline-block;
    width: 8px;
    height: 8px;
    border-radius: 50%;
    margin-right: 6px;
  }
  .dot.green { background: var(--green); }
  .dot.yellow { background: var(--yellow); }
  .dot.red { background: var(--red); }
  .dot.gray { background: var(--gray); }
  .bar-bg {
    display: inline-block;
    width: 60px;
    height: 8px;
    background: rgba(255,255,255,0.1);
    border-radius: 4px;
    overflow: hidden;
    vertical-align: middle;
    margin-right: 6px;
  }
  .bar-fill {
    height: 100%;
    border-radius: 4px;
  }
  .bar-fill.green { background: var(--green); }
  .bar-fill.yellow { background: var(--yellow); }
  .bar-fill.red { background: var(--red); }
  footer {
    color: var(--dim);
    font-size: 0.75rem;
    margin-top: 24px;
    text-align: center;
  }
</style>
</head>
<body>
<h1>Banyan Cluster Snapshot</h1>
<p class="meta">Generated {{.FetchedAt.Format "2006-01-02 15:04:05 MST"}} · Engine {{.Engine.Status}}</p>

<div class="grid">
  <div class="card">
    <div class="label">Agents</div>
    <div class="value">{{.Summary.ConnectedAgents}}/{{.Summary.TotalAgents}}</div>
  </div>
  <div class="card">
    <div class="label">Deployments</div>
    <div class="value">{{.Summary.RunningDeployments}}/{{.Summary.TotalDeployments}}</div>
  </div>
  <div class="card">
    <div class="label">Containers</div>
    <div class="value">{{.Summary.HealthyContainers}}/{{.Summary.TotalContainers}}</div>
  </div>
  <div class="card">
    <div class="label">Tasks</div>
    <div class="value">{{.Summary.CompletedTasks}} ok / {{.Summary.FailedTasks}} fail</div>
  </div>
</div>

{{if .Agents}}
<div class="section">
  <h2>Agents ({{len .Agents}})</h2>
  <table>
    <tr><th>Name</th><th>Status</th><th>CPU</th><th>Memory</th><th>Containers</th><th>Subnet</th><th>Uptime</th></tr>
    {{range .Agents}}
    <tr>
      <td>{{.Name}}</td>
      <td><span class="dot {{statusClass .Status}}"></span>{{.Status}}</td>
      <td>{{fmtPercent .CPU}}</td>
      <td>{{fmtBytes .MemUsed}} / {{fmtBytes .MemTotal}}</td>
      <td>{{.ContainerCount}}</td>
      <td>{{.VPCSubnet}}</td>
      <td>{{fmtDuration .CreatedAt}}</td>
    </tr>
    {{end}}
  </table>
</div>
{{end}}

{{if .Deployments}}
<div class="section">
  <h2>Deployments ({{len .Deployments}})</h2>
  <table>
    <tr><th>Name</th><th>Status</th><th>Health</th><th>Services</th><th>Created</th></tr>
    {{range .Deployments}}
    <tr>
      <td>{{.Name}}</td>
      <td><span class="dot {{statusClass .Status}}"></span>{{.Status}}</td>
      <td>{{.Healthy}}/{{.Total}}</td>
      <td>{{.Services}}</td>
      <td>{{fmtDuration .CreatedAt}}</td>
    </tr>
    {{end}}
  </table>
</div>
{{end}}

{{if .Containers}}
<div class="section">
  <h2>Containers ({{len .Containers}})</h2>
  <table>
    <tr><th>Name</th><th>Service</th><th>Agent</th><th>Status</th><th>CPU</th><th>Memory</th><th>Ports</th></tr>
    {{range .Containers}}
    <tr>
      <td>{{.Name}}</td>
      <td>{{.ServiceName}}</td>
      <td>{{.AgentName}}</td>
      <td><span class="dot {{statusClass .ContainerStatus}}"></span>{{.ContainerStatus}}</td>
      <td>{{printf "%.1f%%" .CPUPercent}}</td>
      <td>{{fmtBytes .MemUsed}}</td>
      <td>{{joinPorts .Ports}}</td>
    </tr>
    {{end}}
  </table>
</div>
{{end}}

{{if .Events}}
<div class="section">
  <h2>Recent Events ({{len .Events}})</h2>
  <table>
    <tr><th>Time</th><th>Type</th><th>Message</th></tr>
    {{range .Events}}
    <tr>
      <td>{{.Timestamp.Format "15:04:05"}}</td>
      <td>{{.Type}}</td>
      <td>{{.Message}}</td>
    </tr>
    {{end}}
  </table>
</div>
{{end}}

<footer>Banyan · banyan dashboard --snapshot</footer>
</body>
</html>
`))

// RenderSnapshot writes a self-contained HTML snapshot of the cluster to w.
func RenderSnapshot(data *DashboardData, w io.Writer) error {
	return snapshotHTML.Execute(w, data)
}
