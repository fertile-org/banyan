# Banyan Dashboard Demo
# Record with: vhs demo.tape
# Requires: vhs (https://github.com/charmbracelet/vhs)

Output demo.gif
Output demo.mp4

Set Shell "bash"
Set FontSize 14
Set Width 1200
Set Height 700
Set Theme "Catppuccin Mocha"
Set Padding 10

# Start the dashboard
Type "banyan-cli dashboard"
Enter
Sleep 2s

# Overview screen loads
Sleep 3s

# Switch to Agents view
Type "2"
Sleep 2s

# Switch to Deployments view
Type "3"
Sleep 2s

# Switch to Containers view
Type "4"
Sleep 2s

# Open command palette
Type "p"
Sleep 1s

# Type to filter
Type "over"
Sleep 500ms
Enter
Sleep 2s

# Force refresh
Type "r"
Sleep 2s

# Show help
Type "?"
Sleep 2s
Escape
Sleep 1s

# Quit
Type "q"
