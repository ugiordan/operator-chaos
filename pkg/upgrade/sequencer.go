package upgrade

import (
	"fmt"
	"strings"

	"github.com/opendatahub-io/odh-platform-chaos/pkg/model"
)

// Step represents a unit of work in the upgrade sequence.
// Kept separate from PlaybookStep to avoid circular imports.
type Step struct {
	Name         string
	Type         string
	DependsOn    []string
	Component    string // component name for auto-inference (optional)
	KnowledgeRef string // knowledge file for health gates (optional)
	Synthetic    bool
}

// SequencerOptions controls sequencing behavior.
type SequencerOptions struct {
	InjectHealthGates bool
	AutoInferDeps     bool
	Operators         []string // operator names for graph lookups
}

// Sequence validates dependency edges, reorders steps using Kahn's topological
// sort, and optionally injects health gate steps between components.
// Returns reordered steps, warning strings, and an error for hard failures
// (e.g. unknown step references in dependsOn).
func Sequence(steps []Step, graph *model.DependencyGraph, opts SequencerOptions) ([]Step, []string, error) {
	if len(steps) == 0 {
		return nil, nil, nil
	}

	// Build name-to-index lookup and validate uniqueness
	nameIndex := make(map[string]int, len(steps))
	for i, s := range steps {
		nameIndex[s.Name] = i
	}

	// Validate all dependsOn references exist
	for _, s := range steps {
		for _, dep := range s.DependsOn {
			if _, ok := nameIndex[dep]; !ok {
				return nil, nil, fmt.Errorf("step %q depends on unknown step %q", s.Name, dep)
			}
		}
	}

	// Build adjacency list: adj[a] contains b means "a must come before b"
	adj := make(map[string][]string, len(steps))
	inDegree := make(map[string]int, len(steps))
	for _, s := range steps {
		if _, ok := inDegree[s.Name]; !ok {
			inDegree[s.Name] = 0
		}
	}

	addEdge := func(from, to string) {
		// Deduplicate
		for _, existing := range adj[from] {
			if existing == to {
				return
			}
		}
		adj[from] = append(adj[from], to)
		inDegree[to]++
	}

	// Explicit dependsOn edges
	for _, s := range steps {
		for _, dep := range s.DependsOn {
			addEdge(dep, s.Name)
		}
	}

	// Auto-infer dependencies from the knowledge graph
	if opts.AutoInferDeps && graph != nil {
		for i, stepA := range steps {
			if stepA.Component == "" || len(stepA.DependsOn) > 0 {
				continue
			}
			for j, stepB := range steps {
				if i == j || stepB.Component == "" || stepA.Component == stepB.Component || len(stepB.DependsOn) > 0 {
					continue
				}
				// Check if stepB.Component depends on stepA.Component
				// DirectDependents(A) returns things that depend on A
				if componentDependsOn(graph, opts.Operators, stepA.Component, stepB.Component) {
					addEdge(stepA.Name, stepB.Name)
				}
			}
		}
	}

	// Run Kahn's algorithm with cycle handling
	var warnings []string
	sorted, warns := kahnSort(steps, adj, inDegree, nameIndex)
	warnings = append(warnings, warns...)

	// Inject health gates between steps from different components
	if opts.InjectHealthGates {
		sorted = injectHealthGates(sorted)
	}

	return sorted, warnings, nil
}

// componentDependsOn checks if componentB depends on componentA using the
// dependency graph. DirectDependents(A) returns components that depend on A,
// so if B is among them, B depends on A.
func componentDependsOn(graph *model.DependencyGraph, operators []string, compA, compB string) bool {
	for _, op := range operators {
		ref := model.ComponentRef{Operator: op, Component: compA}
		dependents := graph.DirectDependents(ref)
		for _, dep := range dependents {
			if dep.Ref.Component == compB {
				return true
			}
		}
	}
	return false
}

// kahnSort performs Kahn's topological sort with deterministic tie-breaking
// by original position. If cycles are detected, it breaks them by dropping
// the edge from the cycle member with the highest original index.
func kahnSort(steps []Step, adj map[string][]string, inDegree map[string]int, nameIndex map[string]int) ([]Step, []string) {
	var warnings []string

	// Copy inDegree so we can mutate it
	deg := make(map[string]int, len(inDegree))
	for k, v := range inDegree {
		deg[k] = v
	}

	// Copy adjacency list so we can mutate it for cycle breaking
	adjCopy := make(map[string][]string, len(adj))
	for k, v := range adj {
		copied := make([]string, len(v))
		copy(copied, v)
		adjCopy[k] = copied
	}

	stepMap := make(map[string]Step, len(steps))
	for _, s := range steps {
		stepMap[s.Name] = s
	}

	for {
		result := runKahn(steps, adjCopy, deg, nameIndex)
		if len(result) == len(steps) {
			return result, warnings
		}

		// Cycle detected: find remaining nodes
		processed := make(map[string]bool, len(result))
		for _, s := range result {
			processed[s.Name] = true
		}

		var cycleMembers []string
		for _, s := range steps {
			if !processed[s.Name] {
				cycleMembers = append(cycleMembers, s.Name)
			}
		}

		warnings = append(warnings, fmt.Sprintf("cycle detected among steps: %s", strings.Join(cycleMembers, ", ")))

		// Break cycle: find the cycle member with the highest original index,
		// drop one incoming edge to it
		highestIdx := -1
		var highestName string
		for _, name := range cycleMembers {
			if nameIndex[name] > highestIdx {
				highestIdx = nameIndex[name]
				highestName = name
			}
		}

		// Remove one edge pointing to highestName from another cycle member
		for _, from := range cycleMembers {
			newAdj := adjCopy[from][:0]
			removed := false
			for _, to := range adjCopy[from] {
				if to == highestName && !removed {
					deg[highestName]--
					removed = true
					continue
				}
				newAdj = append(newAdj, to)
			}
			if removed {
				adjCopy[from] = newAdj
				break
			}
		}
	}
}

// runKahn executes one pass of Kahn's algorithm. Returns as many sorted
// steps as possible (less than total means a cycle exists).
func runKahn(steps []Step, adj map[string][]string, deg map[string]int, nameIndex map[string]int) []Step {
	// Fresh copy of degrees
	d := make(map[string]int, len(deg))
	for k, v := range deg {
		d[k] = v
	}

	// Initialize queue with zero in-degree nodes, sorted by original position
	var queue []string
	for _, s := range steps {
		if d[s.Name] == 0 {
			queue = append(queue, s.Name)
		}
	}

	stepMap := make(map[string]Step, len(steps))
	for _, s := range steps {
		stepMap[s.Name] = s
	}

	var result []Step
	for len(queue) > 0 {
		// Pick the node with the lowest original position (deterministic)
		bestIdx := 0
		for i := 1; i < len(queue); i++ {
			if nameIndex[queue[i]] < nameIndex[queue[bestIdx]] {
				bestIdx = i
			}
		}
		current := queue[bestIdx]
		queue = append(queue[:bestIdx], queue[bestIdx+1:]...)

		result = append(result, stepMap[current])

		for _, neighbor := range adj[current] {
			d[neighbor]--
			if d[neighbor] == 0 {
				queue = append(queue, neighbor)
			}
		}
	}

	return result
}

// injectHealthGates inserts synthetic validate-version steps between
// consecutive steps that target different components.
func injectHealthGates(steps []Step) []Step {
	if len(steps) <= 1 {
		return steps
	}

	var result []Step
	result = append(result, steps[0])

	for i := 1; i < len(steps); i++ {
		prev := steps[i-1]
		curr := steps[i]

		if prev.Component != "" && curr.Component != "" && prev.Component != curr.Component {
			gateName := fmt.Sprintf("health-check-%s", prev.Component)
			result = append(result, Step{
				Name:      gateName,
				Type:      "validate-version",
				Component: prev.Component,
				Synthetic: true,
			})
		}

		result = append(result, curr)
	}

	return result
}
