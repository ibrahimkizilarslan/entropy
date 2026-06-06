# Output file name
Output docs/img/entropy-demo.gif

# Visual settings
Set FontSize 18
Set Width 1200
Set Height 650
Set Padding 40
Set Theme "Catppuccin Macchiato"

# Setup environment
Hide
Type "export PATH=$(go env GOPATH)/bin:$PATH"
Enter
Type "cd examples/demo-distributed"
Enter
Type "rm -f chaos.yaml"
Enter
Ctrl+L
Show

# --- Scenario Starts ---

# Step 1: Auto-discovery
Type "entropy init"
Sleep 500ms
Enter
Sleep 1.5s

# Step 2: Validation (NEW!)
Type "entropy validate chaos.yaml"
Sleep 500ms
Enter
Sleep 1.5s

# Step 3: Topology visualization
Type "entropy topology"
Sleep 500ms
Enter
Sleep 2s

# Step 4: Resilience Analysis
Type "entropy doctor"
Sleep 500ms
Enter
Sleep 3s

Ctrl+L

# Step 5: Random Chaos Engine
Type "entropy start --detach"
Sleep 500ms
Enter
Sleep 1.5s
Type "entropy status"
Sleep 500ms
Enter
Sleep 1.5s
Type "entropy logs & sleep 3; kill $!"
Sleep 500ms
Enter
Sleep 3.5s
Type "entropy stop"
Sleep 500ms
Enter
Sleep 1s

# Step 6: Emergency Cleanup (RESTORED!)
Type "entropy cleanup"
Sleep 500ms
Enter
Sleep 2s

Ctrl+L

# Step 7: Deterministic Scenario Execution
Type "entropy scenario run ../../chaos-scenario.example.yaml"
Sleep 1s
Enter
Sleep 12s

# Conclusion
Type "# 🌀 Entropy: Developer-first Chaos Engineering"
Sleep 3s
