# Output file name
Output docs/img/entropy-demo.gif

# Visual settings
Set FontSize 22
Set Width 1200
Set Height 600
Set Padding 40
Set Theme "Catppuccin Macchiato"

# --- Scenario Starts ---

# Step 0: Move to the example directory where docker-compose.yml exists
Type "cd examples/demo-distributed"
Enter
Sleep 1s
Ctrl+L

# Step 1: Init command (Auto-discovery)
Type "entropy init"
Sleep 500ms
Enter
Sleep 2s

# Step 2: Topology visualization
Type "entropy topology"
Sleep 500ms
Enter
Sleep 3s

# Step 3: Entropy Doctor (Resilience Analysis)
Type "entropy doctor"
Sleep 500ms
Enter
Sleep 4s

# Clear screen
Ctrl+L

# Step 4: Random Fault Injection (Background Engine)
Type "entropy start --detach"
Sleep 500ms
Enter
Sleep 2s
Type "entropy status"
Sleep 500ms
Enter
Sleep 3s
Type "entropy stop"
Sleep 500ms
Enter
Sleep 2s

# Clear screen for the main event
Ctrl+L

# Step 5: Run a deterministic chaos scenario
Type "entropy scenario run ../../chaos-scenario.example.yaml"
Sleep 1s
Enter
# Wait for the scenario to complete (approx 15-20s)
Sleep 20s

# Conclusion
Type "# Ready for controlled chaos! 🌀"
Sleep 3s
