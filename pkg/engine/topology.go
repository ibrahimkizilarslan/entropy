package engine

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/pterm/pterm"
	"gopkg.in/yaml.v3"
)

type TopologyNode struct {
	Name      string
	Networks  []string
	DependsOn []string
}

// GenerateTopologyTree analyzes the compose file and generates a pterm.TreeNode
// representing the blast radius based on networks and depends_on.
func GenerateTopologyTree(dir string) (*pterm.TreeNode, error) {
	var composePath string
	candidates := []string{"docker-compose.yml", "docker-compose.yaml", "compose.yaml"}
	for _, c := range candidates {
		p := filepath.Join(dir, c)
		if _, err := os.Stat(p); err == nil {
			composePath = p
			break
		}
	}

	if composePath == "" {
		return nil, fmt.Errorf("could not find any compose file in %s", dir)
	}

	data, err := os.ReadFile(composePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read compose file: %w", err)
	}

	// Parse generic yaml since networks can be complex (map or list) and depends_on too
	var compose map[string]interface{}
	if err := yaml.Unmarshal(data, &compose); err != nil {
		return nil, fmt.Errorf("failed to parse compose file: %w", err)
	}

	services, ok := compose["services"].(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("no services found in compose file")
	}

	nodes := make(map[string]TopologyNode)
	networkMap := make(map[string][]string) // Network -> []Service

	for name, svcData := range services {
		svcMap, ok := svcData.(map[string]interface{})
		if !ok {
			continue
		}

		node := TopologyNode{Name: name}

		// Parse Networks
		if nets, ok := svcMap["networks"]; ok {
			switch n := nets.(type) {
			case []interface{}:
				for _, netName := range n {
					if strName, ok := netName.(string); ok {
						node.Networks = append(node.Networks, strName)
						networkMap[strName] = append(networkMap[strName], name)
					}
				}
			case map[string]interface{}:
				for netName := range n {
					node.Networks = append(node.Networks, netName)
					networkMap[netName] = append(networkMap[netName], name)
				}
			}
		}

		// If no network is specified, it uses "default"
		if len(node.Networks) == 0 {
			node.Networks = append(node.Networks, "default")
			networkMap["default"] = append(networkMap["default"], name)
		}

		// Parse depends_on
		if deps, ok := svcMap["depends_on"]; ok {
			switch d := deps.(type) {
			case []interface{}:
				for _, depName := range d {
					if strName, ok := depName.(string); ok {
						node.DependsOn = append(node.DependsOn, strName)
					}
				}
			case map[string]interface{}:
				for depName := range d {
					node.DependsOn = append(node.DependsOn, depName)
				}
			}
		}

		nodes[name] = node
	}

	// Build the tree
	rootNodes := []pterm.TreeNode{}

	// Let's group by Network to show Blast Radius easily
	for netName, svcs := range networkMap {
		netChildren := []pterm.TreeNode{}

		// Sort for determinism
		sort.Strings(svcs)

		for _, svc := range svcs {
			svcNode := nodes[svc]
			svcText := pterm.Cyan(svc)

			if len(svcNode.DependsOn) > 0 {
				svcText += pterm.Gray(fmt.Sprintf(" (depends on: %v)", svcNode.DependsOn))
			}

			netChildren = append(netChildren, pterm.TreeNode{Text: svcText})
		}

		rootNodes = append(rootNodes, pterm.TreeNode{
			Text:     pterm.Green("Network: " + netName),
			Children: netChildren,
		})
	}

	tree := pterm.TreeNode{
		Text:     pterm.LightMagenta("Cluster Topology & Blast Radius"),
		Children: rootNodes,
	}

	return &tree, nil
}
