# Banyan Dashboard Demo
# Record with: make demo
# Requires: vhs (https://github.com/charmbracelet/vhs)

Output demo.gif
Output demo.mp4

Set Shell "bash"
Set FontSize 16
Set Width 1920
Set Height 1080
Set Theme "Catppuccin Mocha"
Set Padding 10

# Start the dashboard (vhs runs as root via sudo, so no sudo needed here)
Type "banyan-cli dashboard"
Enter
Sleep 1.5s

# Overview screen loads
Sleep 1.5s

# Switch to Agents view
Type "2"
Sleep 1.5s

# Switch to Deployments view
Type "3"
Sleep 1.5s

# Switch to Containers view
Type "4"
Sleep 1.5s

# Open command palette
Type "p"
Sleep 1s

# Type to filter
Type "over"
Sleep 500ms
Enter
Sleep 1.5s

# Force refresh
Type "r"
Sleep 1.5s

# Show help
Type "?"
Sleep 1.5s
Escape
Sleep 1s

# Quit
Type "q"
