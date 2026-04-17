//go:build ignore

package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type svg struct {
	name    string
	width   int
	height  int
	content func(width, height int) string
}

func main() {
	outputDir := flag.String("output-dir", "site/docs/assets/screenshots/", "Output directory for SVG files")
	flag.Parse()

	if err := os.MkdirAll(*outputDir, 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create output directory: %v\n", err)
		os.Exit(1)
	}

	svgs := []svg{
		// Dashboard views
		{name: "dashboard-overview.svg", width: 800, height: 400, content: dashboardOverview},
		{name: "dashboard-live.svg", width: 800, height: 400, content: dashboardLive},
		{name: "dashboard-experiments.svg", width: 800, height: 400, content: dashboardExperiments},
		{name: "dashboard-detail.svg", width: 800, height: 400, content: dashboardDetail},
		{name: "dashboard-suites.svg", width: 800, height: 400, content: dashboardSuites},
		{name: "dashboard-operators.svg", width: 800, height: 400, content: dashboardOperators},
		{name: "dashboard-knowledge.svg", width: 800, height: 400, content: dashboardKnowledge},
		// Grafana panels
		{name: "grafana-verdict-distribution.svg", width: 600, height: 300, content: grafanaPieChart},
		{name: "grafana-recovery-time.svg", width: 600, height: 300, content: grafanaHistogram},
		{name: "grafana-active-experiments.svg", width: 600, height: 300, content: grafanaGauge},
		{name: "grafana-injection-types.svg", width: 600, height: 300, content: grafanaBarChart},
		{name: "grafana-recovery-cycles.svg", width: 600, height: 300, content: grafanaHistogram},
		{name: "grafana-experiment-timeline.svg", width: 600, height: 300, content: grafanaTimeSeries},
		{name: "grafana-deviation-types.svg", width: 600, height: 300, content: grafanaBarChart},
		{name: "grafana-component-health.svg", width: 600, height: 300, content: grafanaTable},
		{name: "grafana-suite-pass-rate.svg", width: 600, height: 300, content: grafanaGauge},
		// Additional Grafana panels referenced in docs
		{name: "grafana-total-injections.svg", width: 600, height: 300, content: grafanaGauge},
		{name: "grafana-deviations-per-experiment.svg", width: 600, height: 300, content: grafanaBarChart},
		{name: "grafana-injections-by-type.svg", width: 600, height: 300, content: grafanaBarChart},
		{name: "grafana-verdicts-over-time.svg", width: 600, height: 300, content: grafanaTimeSeries},
		{name: "grafana-deviations-by-type.svg", width: 600, height: 300, content: grafanaBarChart},
		{name: "grafana-reconcile-cycles.svg", width: 600, height: 300, content: grafanaHistogram},
	}

	generated := 0
	skipped := 0

	for _, s := range svgs {
		outputPath := filepath.Join(*outputDir, s.name)
		baseName := strings.TrimSuffix(s.name, ".svg")
		pngPath := filepath.Join(*outputDir, baseName+".png")

		if _, err := os.Stat(pngPath); err == nil {
			fmt.Printf("Skipping %s (PNG exists)\n", s.name)
			skipped++
			continue
		}

		content := s.content(s.width, s.height)
		if err := os.WriteFile(outputPath, []byte(content), 0o644); err != nil {
			fmt.Fprintf(os.Stderr, "Failed to write %s: %v\n", s.name, err)
			os.Exit(1)
		}

		fmt.Printf("Generated %s\n", s.name)
		generated++
	}

	fmt.Printf("\nSummary: %d generated, %d skipped\n", generated, skipped)
}

// Dashboard view generators

func dashboardOverview(w, h int) string {
	return fmt.Sprintf(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 %d %d">
  <rect width="%d" height="%d" fill="#f5f5f5" rx="8"/>
  <text x="%d" y="30" text-anchor="middle" font-family="Arial" font-size="16" font-weight="bold" fill="#333">Overview Dashboard</text>
  <text x="%d" y="%d" text-anchor="middle" font-family="Arial" font-size="11" fill="#999">PLACEHOLDER - Replace with actual screenshot</text>

  <!-- Metric cards -->
  <rect x="20" y="50" width="180" height="80" fill="white" stroke="#ddd" stroke-width="1" rx="6"/>
  <text x="110" y="75" text-anchor="middle" font-family="Arial" font-size="11" fill="#666">Experiments Run</text>
  <text x="110" y="110" text-anchor="middle" font-family="Arial" font-size="28" font-weight="bold" fill="#1565C0">42</text>

  <rect x="210" y="50" width="180" height="80" fill="white" stroke="#ddd" stroke-width="1" rx="6"/>
  <text x="300" y="75" text-anchor="middle" font-family="Arial" font-size="11" fill="#666">Resilient %%</text>
  <text x="300" y="110" text-anchor="middle" font-family="Arial" font-size="28" font-weight="bold" fill="#2E7D32">87%%</text>

  <rect x="400" y="50" width="180" height="80" fill="white" stroke="#ddd" stroke-width="1" rx="6"/>
  <text x="490" y="75" text-anchor="middle" font-family="Arial" font-size="11" fill="#666">Active</text>
  <text x="490" y="110" text-anchor="middle" font-family="Arial" font-size="28" font-weight="bold" fill="#F57C00">3</text>

  <rect x="590" y="50" width="180" height="80" fill="white" stroke="#ddd" stroke-width="1" rx="6"/>
  <text x="680" y="75" text-anchor="middle" font-family="Arial" font-size="11" fill="#666">Components</text>
  <text x="680" y="110" text-anchor="middle" font-family="Arial" font-size="28" font-weight="bold" fill="#5E35B1">12</text>

  <!-- Recent results table -->
  <rect x="20" y="150" width="750" height="220" fill="white" stroke="#ddd" stroke-width="1" rx="6"/>
  <text x="30" y="175" font-family="Arial" font-size="13" font-weight="bold" fill="#333">Recent Experiment Results</text>
  <line x1="30" y1="185" x2="760" y2="185" stroke="#ddd" stroke-width="1"/>
  <text x="30" y="205" font-family="Arial" font-size="10" fill="#666">Name</text>
  <text x="250" y="205" font-family="Arial" font-size="10" fill="#666">Type</text>
  <text x="400" y="205" font-family="Arial" font-size="10" fill="#666">Verdict</text>
  <text x="550" y="205" font-family="Arial" font-size="10" fill="#666">Component</text>
  <text x="680" y="205" font-family="Arial" font-size="10" fill="#666">Date</text>
  <line x1="30" y1="210" x2="760" y2="210" stroke="#ddd" stroke-width="1"/>
  <text x="30" y="230" font-family="Arial" font-size="10" fill="#333">pod-deletion-test</text>
  <text x="250" y="230" font-family="Arial" font-size="10" fill="#333">PodFailure</text>
  <text x="400" y="230" font-family="Arial" font-size="10" fill="#2E7D32">Pass</text>
  <text x="30" y="250" font-family="Arial" font-size="10" fill="#333">network-partition-test</text>
  <text x="250" y="250" font-family="Arial" font-size="10" fill="#333">NetworkChaos</text>
  <text x="400" y="250" font-family="Arial" font-size="10" fill="#C62828">Fail</text>
  <text x="30" y="270" font-family="Arial" font-size="10" fill="#333">cpu-stress-test</text>
  <text x="250" y="270" font-family="Arial" font-size="10" fill="#333">StressChaos</text>
  <text x="400" y="270" font-family="Arial" font-size="10" fill="#2E7D32">Pass</text>
</svg>`, w, h, w, h, w/2, w/2, h-10)
}

func dashboardLive(w, h int) string {
	return fmt.Sprintf(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 %d %d">
  <rect width="%d" height="%d" fill="#f5f5f5" rx="8"/>
  <text x="%d" y="30" text-anchor="middle" font-family="Arial" font-size="16" font-weight="bold" fill="#333">Live Experiment Progress</text>
  <text x="%d" y="%d" text-anchor="middle" font-family="Arial" font-size="11" fill="#999">PLACEHOLDER - Replace with actual screenshot</text>

  <!-- Experiment info card -->
  <rect x="20" y="50" width="750" height="100" fill="white" stroke="#ddd" stroke-width="1" rx="6"/>
  <text x="30" y="75" font-family="Arial" font-size="13" font-weight="bold" fill="#333">Experiment: pod-deletion-suite</text>
  <text x="30" y="95" font-family="Arial" font-size="11" fill="#666">Type: PodFailure</text>
  <text x="30" y="115" font-family="Arial" font-size="11" fill="#666">Component: dashboard-controller</text>
  <text x="30" y="135" font-family="Arial" font-size="11" fill="#666">Status: Running</text>
  <circle cx="730" cy="90" r="25" fill="none" stroke="#1565C0" stroke-width="3"/>
  <text x="730" y="95" text-anchor="middle" font-family="Arial" font-size="11" fill="#1565C0">45%%</text>

  <!-- Phase timeline -->
  <rect x="20" y="170" width="750" height="200" fill="white" stroke="#ddd" stroke-width="1" rx="6"/>
  <text x="30" y="195" font-family="Arial" font-size="13" font-weight="bold" fill="#333">Phase Timeline</text>
  <line x1="50" y1="220" x2="750" y2="220" stroke="#ddd" stroke-width="2"/>

  <!-- Timeline markers -->
  <circle cx="50" cy="220" r="6" fill="#2E7D32"/>
  <text x="50" y="245" text-anchor="middle" font-family="Arial" font-size="9" fill="#666">Setup</text>
  <text x="50" y="260" text-anchor="middle" font-family="Arial" font-size="9" fill="#2E7D32">Complete</text>

  <circle cx="250" cy="220" r="6" fill="#2E7D32"/>
  <text x="250" y="245" text-anchor="middle" font-family="Arial" font-size="9" fill="#666">Inject</text>
  <text x="250" y="260" text-anchor="middle" font-family="Arial" font-size="9" fill="#2E7D32">Complete</text>

  <circle cx="450" cy="220" r="6" fill="#1565C0"/>
  <text x="450" y="245" text-anchor="middle" font-family="Arial" font-size="9" fill="#666">Observe</text>
  <text x="450" y="260" text-anchor="middle" font-family="Arial" font-size="9" fill="#1565C0">Running</text>

  <circle cx="650" cy="220" r="6" fill="#999"/>
  <text x="650" y="245" text-anchor="middle" font-family="Arial" font-size="9" fill="#666">Teardown</text>
  <text x="650" y="260" text-anchor="middle" font-family="Arial" font-size="9" fill="#999">Pending</text>

  <!-- Event log -->
  <text x="30" y="295" font-family="Arial" font-size="11" font-weight="bold" fill="#333">Recent Events:</text>
  <text x="40" y="315" font-family="Arial" font-size="9" fill="#666">14:32:15 - Pod deletion initiated</text>
  <text x="40" y="330" font-family="Arial" font-size="9" fill="#666">14:32:18 - Pod restarted</text>
  <text x="40" y="345" font-family="Arial" font-size="9" fill="#666">14:32:22 - Health check passed</text>
</svg>`, w, h, w, h, w/2, w/2, h-10)
}

func dashboardExperiments(w, h int) string {
	return fmt.Sprintf(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 %d %d">
  <rect width="%d" height="%d" fill="#f5f5f5" rx="8"/>
  <text x="%d" y="30" text-anchor="middle" font-family="Arial" font-size="16" font-weight="bold" fill="#333">Experiments List</text>
  <text x="%d" y="%d" text-anchor="middle" font-family="Arial" font-size="11" fill="#999">PLACEHOLDER - Replace with actual screenshot</text>

  <!-- Filter bar -->
  <rect x="20" y="50" width="750" height="40" fill="white" stroke="#ddd" stroke-width="1" rx="6"/>
  <text x="30" y="73" font-family="Arial" font-size="11" fill="#666">Filters:</text>
  <rect x="80" y="60" width="100" height="20" fill="#eee" stroke="#ccc" stroke-width="1" rx="3"/>
  <text x="85" y="74" font-family="Arial" font-size="9" fill="#333">Type: All</text>
  <rect x="190" y="60" width="120" height="20" fill="#eee" stroke="#ccc" stroke-width="1" rx="3"/>
  <text x="195" y="74" font-family="Arial" font-size="9" fill="#333">Verdict: All</text>
  <rect x="320" y="60" width="140" height="20" fill="#eee" stroke="#ccc" stroke-width="1" rx="3"/>
  <text x="325" y="74" font-family="Arial" font-size="9" fill="#333">Component: All</text>

  <!-- Results table -->
  <rect x="20" y="110" width="750" height="260" fill="white" stroke="#ddd" stroke-width="1" rx="6"/>
  <text x="30" y="130" font-family="Arial" font-size="10" font-weight="bold" fill="#666">Name</text>
  <text x="250" y="130" font-family="Arial" font-size="10" font-weight="bold" fill="#666">Type</text>
  <text x="400" y="130" font-family="Arial" font-size="10" font-weight="bold" fill="#666">Verdict</text>
  <text x="520" y="130" font-family="Arial" font-size="10" font-weight="bold" fill="#666">Component</text>
  <text x="680" y="130" font-family="Arial" font-size="10" font-weight="bold" fill="#666">Date</text>
  <line x1="30" y1="137" x2="760" y2="137" stroke="#ddd" stroke-width="1"/>

  <!-- Table rows -->
  <text x="30" y="160" font-family="Arial" font-size="10" fill="#333">pod-deletion-test</text>
  <text x="250" y="160" font-family="Arial" font-size="10" fill="#333">PodFailure</text>
  <text x="400" y="160" font-family="Arial" font-size="10" fill="#2E7D32">Pass</text>
  <text x="520" y="160" font-family="Arial" font-size="10" fill="#333">dashboard-controller</text>
  <text x="680" y="160" font-family="Arial" font-size="10" fill="#666">2024-03-15</text>

  <text x="30" y="185" font-family="Arial" font-size="10" fill="#333">network-partition-test</text>
  <text x="250" y="185" font-family="Arial" font-size="10" fill="#333">NetworkChaos</text>
  <text x="400" y="185" font-family="Arial" font-size="10" fill="#C62828">Fail</text>
  <text x="520" y="185" font-family="Arial" font-size="10" fill="#333">kserve-controller</text>
  <text x="680" y="185" font-family="Arial" font-size="10" fill="#666">2024-03-14</text>

  <text x="30" y="210" font-family="Arial" font-size="10" fill="#333">cpu-stress-test</text>
  <text x="250" y="210" font-family="Arial" font-size="10" fill="#333">StressChaos</text>
  <text x="400" y="210" font-family="Arial" font-size="10" fill="#2E7D32">Pass</text>
  <text x="520" y="210" font-family="Arial" font-size="10" fill="#333">model-mesh</text>
  <text x="680" y="210" font-family="Arial" font-size="10" fill="#666">2024-03-13</text>

  <text x="30" y="235" font-family="Arial" font-size="10" fill="#333">disk-fill-test</text>
  <text x="250" y="235" font-family="Arial" font-size="10" fill="#333">IOChaos</text>
  <text x="400" y="235" font-family="Arial" font-size="10" fill="#F57C00">Inconclusive</text>
  <text x="520" y="235" font-family="Arial" font-size="10" fill="#333">workbenches</text>
  <text x="680" y="235" font-family="Arial" font-size="10" fill="#666">2024-03-12</text>

  <text x="30" y="260" font-family="Arial" font-size="10" fill="#333">memory-leak-test</text>
  <text x="250" y="260" font-family="Arial" font-size="10" fill="#333">StressChaos</text>
  <text x="400" y="260" font-family="Arial" font-size="10" fill="#2E7D32">Pass</text>
  <text x="520" y="260" font-family="Arial" font-size="10" fill="#333">pipelines</text>
  <text x="680" y="260" font-family="Arial" font-size="10" fill="#666">2024-03-11</text>
</svg>`, w, h, w, h, w/2, w/2, h-10)
}

func dashboardDetail(w, h int) string {
	return fmt.Sprintf(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 %d %d">
  <rect width="%d" height="%d" fill="#f5f5f5" rx="8"/>
  <text x="%d" y="30" text-anchor="middle" font-family="Arial" font-size="16" font-weight="bold" fill="#333">Experiment Detail</text>
  <text x="%d" y="%d" text-anchor="middle" font-family="Arial" font-size="11" fill="#999">PLACEHOLDER - Replace with actual screenshot</text>

  <!-- Tab navigation -->
  <rect x="20" y="50" width="100" height="30" fill="white" stroke="#1565C0" stroke-width="2" rx="6"/>
  <text x="70" y="70" text-anchor="middle" font-family="Arial" font-size="11" fill="#1565C0">Timeline</text>
  <rect x="125" y="50" width="100" height="30" fill="white" stroke="#ddd" stroke-width="1" rx="6"/>
  <text x="175" y="70" text-anchor="middle" font-family="Arial" font-size="11" fill="#666">Events</text>
  <rect x="230" y="50" width="100" height="30" fill="white" stroke="#ddd" stroke-width="1" rx="6"/>
  <text x="280" y="70" text-anchor="middle" font-family="Arial" font-size="11" fill="#666">Raw YAML</text>

  <!-- Content area -->
  <rect x="20" y="95" width="750" height="275" fill="white" stroke="#ddd" stroke-width="1" rx="6"/>
  <text x="30" y="120" font-family="Arial" font-size="13" font-weight="bold" fill="#333">pod-deletion-test</text>
  <text x="30" y="140" font-family="Arial" font-size="11" fill="#666">Type: PodFailure</text>
  <text x="30" y="155" font-family="Arial" font-size="11" fill="#666">Verdict: Pass</text>
  <text x="30" y="170" font-family="Arial" font-size="11" fill="#666">Component: dashboard-controller</text>

  <!-- Timeline visualization -->
  <line x1="50" y1="200" x2="750" y2="200" stroke="#ddd" stroke-width="2"/>
  <circle cx="150" cy="200" r="6" fill="#2E7D32"/>
  <text x="150" y="220" text-anchor="middle" font-family="Arial" font-size="9" fill="#666">Setup</text>
  <circle cx="300" cy="200" r="6" fill="#2E7D32"/>
  <text x="300" y="220" text-anchor="middle" font-family="Arial" font-size="9" fill="#666">Inject</text>
  <circle cx="450" cy="200" r="6" fill="#2E7D32"/>
  <text x="450" y="220" text-anchor="middle" font-family="Arial" font-size="9" fill="#666">Observe</text>
  <circle cx="600" cy="200" r="6" fill="#2E7D32"/>
  <text x="600" y="220" text-anchor="middle" font-family="Arial" font-size="9" fill="#666">Teardown</text>

  <!-- Metrics -->
  <text x="30" y="250" font-family="Arial" font-size="11" font-weight="bold" fill="#333">Metrics:</text>
  <text x="40" y="270" font-family="Arial" font-size="10" fill="#666">Duration: 5m 32s</text>
  <text x="40" y="285" font-family="Arial" font-size="10" fill="#666">Recovery Time: 2m 15s</text>
  <text x="40" y="300" font-family="Arial" font-size="10" fill="#666">Recovery Cycles: 1</text>
  <text x="40" y="315" font-family="Arial" font-size="10" fill="#666">Deviation Score: 0.12</text>

  <text x="300" y="270" font-family="Arial" font-size="10" fill="#666">Pods Affected: 3</text>
  <text x="300" y="285" font-family="Arial" font-size="10" fill="#666">Events Captured: 47</text>
  <text x="300" y="300" font-family="Arial" font-size="10" fill="#666">State Transitions: 12</text>
</svg>`, w, h, w, h, w/2, w/2, h-10)
}

func dashboardSuites(w, h int) string {
	return fmt.Sprintf(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 %d %d">
  <rect width="%d" height="%d" fill="#f5f5f5" rx="8"/>
  <text x="%d" y="30" text-anchor="middle" font-family="Arial" font-size="16" font-weight="bold" fill="#333">Experiment Suites</text>
  <text x="%d" y="%d" text-anchor="middle" font-family="Arial" font-size="11" fill="#999">PLACEHOLDER - Replace with actual screenshot</text>

  <!-- Suite cards -->
  <rect x="20" y="50" width="350" height="120" fill="white" stroke="#ddd" stroke-width="1" rx="6"/>
  <text x="30" y="75" font-family="Arial" font-size="13" font-weight="bold" fill="#333">Operator Resilience Suite</text>
  <text x="30" y="95" font-family="Arial" font-size="10" fill="#666">5 experiments</text>
  <text x="30" y="110" font-family="Arial" font-size="10" fill="#666">Last run: 2024-03-15 14:32</text>
  <text x="30" y="125" font-family="Arial" font-size="10" fill="#666">Pass rate: 80%%</text>
  <rect x="30" y="135" width="200" height="20" fill="#eee" stroke="#ccc" stroke-width="1" rx="3"/>
  <rect x="30" y="135" width="160" height="20" fill="#2E7D32" rx="3"/>
  <text x="130" y="149" text-anchor="middle" font-family="Arial" font-size="9" fill="white">4/5 passed</text>

  <rect x="390" y="50" width="350" height="120" fill="white" stroke="#ddd" stroke-width="1" rx="6"/>
  <text x="400" y="75" font-family="Arial" font-size="13" font-weight="bold" fill="#333">Network Failure Suite</text>
  <text x="400" y="95" font-family="Arial" font-size="10" fill="#666">3 experiments</text>
  <text x="400" y="110" font-family="Arial" font-size="10" fill="#666">Last run: 2024-03-14 10:15</text>
  <text x="400" y="125" font-family="Arial" font-size="10" fill="#666">Pass rate: 67%%</text>
  <rect x="400" y="135" width="200" height="20" fill="#eee" stroke="#ccc" stroke-width="1" rx="3"/>
  <rect x="400" y="135" width="134" height="20" fill="#F57C00" rx="3"/>
  <text x="500" y="149" text-anchor="middle" font-family="Arial" font-size="9" fill="white">2/3 passed</text>

  <!-- Run comparison -->
  <rect x="20" y="190" width="750" height="180" fill="white" stroke="#ddd" stroke-width="1" rx="6"/>
  <text x="30" y="215" font-family="Arial" font-size="13" font-weight="bold" fill="#333">Run Comparison - Operator Resilience Suite</text>
  <line x1="30" y1="225" x2="760" y2="225" stroke="#ddd" stroke-width="1"/>

  <text x="30" y="245" font-family="Arial" font-size="10" font-weight="bold" fill="#666">Experiment</text>
  <text x="250" y="245" font-family="Arial" font-size="10" font-weight="bold" fill="#666">Run 1 (Mar 15)</text>
  <text x="400" y="245" font-family="Arial" font-size="10" font-weight="bold" fill="#666">Run 2 (Mar 10)</text>
  <text x="550" y="245" font-family="Arial" font-size="10" font-weight="bold" fill="#666">Run 3 (Mar 5)</text>
  <line x1="30" y1="252" x2="760" y2="252" stroke="#ddd" stroke-width="1"/>

  <text x="30" y="272" font-family="Arial" font-size="9" fill="#333">pod-deletion</text>
  <text x="250" y="272" font-family="Arial" font-size="9" fill="#2E7D32">Pass (2m 15s)</text>
  <text x="400" y="272" font-family="Arial" font-size="9" fill="#2E7D32">Pass (2m 10s)</text>
  <text x="550" y="272" font-family="Arial" font-size="9" fill="#2E7D32">Pass (2m 20s)</text>

  <text x="30" y="292" font-family="Arial" font-size="9" fill="#333">network-partition</text>
  <text x="250" y="292" font-family="Arial" font-size="9" fill="#C62828">Fail (timeout)</text>
  <text x="400" y="292" font-family="Arial" font-size="9" fill="#2E7D32">Pass (4m 30s)</text>
  <text x="550" y="292" font-family="Arial" font-size="9" fill="#2E7D32">Pass (4m 25s)</text>

  <text x="30" y="312" font-family="Arial" font-size="9" fill="#333">cpu-stress</text>
  <text x="250" y="312" font-family="Arial" font-size="9" fill="#2E7D32">Pass (3m 45s)</text>
  <text x="400" y="312" font-family="Arial" font-size="9" fill="#2E7D32">Pass (3m 50s)</text>
  <text x="550" y="312" font-family="Arial" font-size="9" fill="#F57C00">Inconclusive</text>
</svg>`, w, h, w, h, w/2, w/2, h-10)
}

func dashboardOperators(w, h int) string {
	return fmt.Sprintf(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 %d %d">
  <rect width="%d" height="%d" fill="#f5f5f5" rx="8"/>
  <text x="%d" y="30" text-anchor="middle" font-family="Arial" font-size="16" font-weight="bold" fill="#333">Operator Dashboard</text>
  <text x="%d" y="%d" text-anchor="middle" font-family="Arial" font-size="11" fill="#999">PLACEHOLDER - Replace with actual screenshot</text>

  <!-- Operator cards -->
  <rect x="20" y="50" width="230" height="100" fill="white" stroke="#ddd" stroke-width="1" rx="6"/>
  <text x="30" y="75" font-family="Arial" font-size="12" font-weight="bold" fill="#333">dashboard-controller</text>
  <text x="30" y="92" font-family="Arial" font-size="9" fill="#666">Status: Healthy</text>
  <text x="30" y="107" font-family="Arial" font-size="9" fill="#666">Experiments: 12</text>
  <text x="30" y="122" font-family="Arial" font-size="9" fill="#666">Resilience: 87%%</text>
  <circle cx="220" cy="70" r="8" fill="#2E7D32"/>

  <rect x="265" y="50" width="230" height="100" fill="white" stroke="#ddd" stroke-width="1" rx="6"/>
  <text x="275" y="75" font-family="Arial" font-size="12" font-weight="bold" fill="#333">kserve-controller</text>
  <text x="275" y="92" font-family="Arial" font-size="9" fill="#666">Status: Degraded</text>
  <text x="275" y="107" font-family="Arial" font-size="9" fill="#666">Experiments: 8</text>
  <text x="275" y="122" font-family="Arial" font-size="9" fill="#666">Resilience: 62%%</text>
  <circle cx="465" cy="70" r="8" fill="#F57C00"/>

  <rect x="510" y="50" width="230" height="100" fill="white" stroke="#ddd" stroke-width="1" rx="6"/>
  <text x="520" y="75" font-family="Arial" font-size="12" font-weight="bold" fill="#333">model-mesh</text>
  <text x="520" y="92" font-family="Arial" font-size="9" fill="#666">Status: Healthy</text>
  <text x="520" y="107" font-family="Arial" font-size="9" fill="#666">Experiments: 15</text>
  <text x="520" y="122" font-family="Arial" font-size="9" fill="#666">Resilience: 93%%</text>
  <circle cx="710" cy="70" r="8" fill="#2E7D32"/>

  <!-- Dependency graph -->
  <rect x="20" y="170" width="750" height="200" fill="white" stroke="#ddd" stroke-width="1" rx="6"/>
  <text x="30" y="195" font-family="Arial" font-size="13" font-weight="bold" fill="#333">Component Dependency Graph</text>

  <!-- Nodes -->
  <rect x="100" y="220" width="120" height="40" fill="#1565C0" fill-opacity="0.1" stroke="#1565C0" stroke-width="2" rx="4"/>
  <text x="160" y="244" text-anchor="middle" font-family="Arial" font-size="10" fill="#1565C0">dashboard-controller</text>

  <rect x="340" y="220" width="120" height="40" fill="#F57C00" fill-opacity="0.1" stroke="#F57C00" stroke-width="2" rx="4"/>
  <text x="400" y="244" text-anchor="middle" font-family="Arial" font-size="10" fill="#F57C00">kserve-controller</text>

  <rect x="580" y="220" width="120" height="40" fill="#2E7D32" fill-opacity="0.1" stroke="#2E7D32" stroke-width="2" rx="4"/>
  <text x="640" y="244" text-anchor="middle" font-family="Arial" font-size="10" fill="#2E7D32">model-mesh</text>

  <rect x="220" y="300" width="120" height="40" fill="#5E35B1" fill-opacity="0.1" stroke="#5E35B1" stroke-width="2" rx="4"/>
  <text x="280" y="324" text-anchor="middle" font-family="Arial" font-size="10" fill="#5E35B1">workbenches</text>

  <rect x="460" y="300" width="120" height="40" fill="#C62828" fill-opacity="0.1" stroke="#C62828" stroke-width="2" rx="4"/>
  <text x="520" y="324" text-anchor="middle" font-family="Arial" font-size="10" fill="#C62828">pipelines</text>

  <!-- Edges -->
  <line x1="220" y1="240" x2="340" y2="240" stroke="#999" stroke-width="1" marker-end="url(#arrowhead)"/>
  <line x1="460" y1="240" x2="580" y2="240" stroke="#999" stroke-width="1" marker-end="url(#arrowhead)"/>
  <line x1="160" y1="260" x2="280" y2="300" stroke="#999" stroke-width="1" marker-end="url(#arrowhead)"/>
  <line x1="400" y1="260" x2="520" y2="300" stroke="#999" stroke-width="1" marker-end="url(#arrowhead)"/>

  <defs>
    <marker id="arrowhead" markerWidth="10" markerHeight="10" refX="9" refY="3" orient="auto">
      <polygon points="0 0, 10 3, 0 6" fill="#999"/>
    </marker>
  </defs>
</svg>`, w, h, w, h, w/2, w/2, h-10)
}

func dashboardKnowledge(w, h int) string {
	return fmt.Sprintf(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 %d %d">
  <rect width="%d" height="%d" fill="#f5f5f5" rx="8"/>
  <text x="%d" y="30" text-anchor="middle" font-family="Arial" font-size="16" font-weight="bold" fill="#333">Knowledge Model Viewer</text>
  <text x="%d" y="%d" text-anchor="middle" font-family="Arial" font-size="11" fill="#999">PLACEHOLDER - Replace with actual screenshot</text>

  <!-- Resource tree (left panel) -->
  <rect x="20" y="50" width="280" height="320" fill="white" stroke="#ddd" stroke-width="1" rx="6"/>
  <text x="30" y="75" font-family="Arial" font-size="12" font-weight="bold" fill="#333">Resource Tree</text>
  <line x1="30" y1="82" x2="290" y2="82" stroke="#ddd" stroke-width="1"/>

  <text x="35" y="105" font-family="Arial" font-size="10" fill="#333">▼ Operators</text>
  <text x="50" y="125" font-family="Arial" font-size="9" fill="#666">▼ dashboard-controller</text>
  <text x="65" y="142" font-family="Arial" font-size="9" fill="#1565C0">Deployment</text>
  <text x="65" y="157" font-family="Arial" font-size="9" fill="#1565C0">Service</text>
  <text x="65" y="172" font-family="Arial" font-size="9" fill="#1565C0">ConfigMap</text>
  <text x="50" y="192" font-family="Arial" font-size="9" fill="#666">▶ kserve-controller</text>
  <text x="50" y="209" font-family="Arial" font-size="9" fill="#666">▶ model-mesh</text>

  <text x="35" y="235" font-family="Arial" font-size="10" fill="#333">▼ Applications</text>
  <text x="50" y="255" font-family="Arial" font-size="9" fill="#666">▶ workbenches</text>
  <text x="50" y="272" font-family="Arial" font-size="9" fill="#666">▶ pipelines</text>
  <text x="50" y="289" font-family="Arial" font-size="9" fill="#666">▶ model-serving</text>

  <text x="35" y="315" font-family="Arial" font-size="10" fill="#333">▶ Infrastructure</text>
  <text x="35" y="335" font-family="Arial" font-size="10" fill="#333">▶ Networking</text>

  <!-- Detail panel (right panel) -->
  <rect x="320" y="50" width="450" height="320" fill="white" stroke="#ddd" stroke-width="1" rx="6"/>
  <text x="330" y="75" font-family="Arial" font-size="12" font-weight="bold" fill="#333">Resource Detail: dashboard-controller/Deployment</text>
  <line x1="330" y1="82" x2="760" y2="82" stroke="#ddd" stroke-width="1"/>

  <text x="330" y="105" font-family="Arial" font-size="10" font-weight="bold" fill="#666">Properties:</text>
  <text x="340" y="125" font-family="Arial" font-size="9" fill="#333">Name: odh-dashboard-controller</text>
  <text x="340" y="140" font-family="Arial" font-size="9" fill="#333">Namespace: redhat-ods-applications</text>
  <text x="340" y="155" font-family="Arial" font-size="9" fill="#333">Replicas: 2</text>
  <text x="340" y="170" font-family="Arial" font-size="9" fill="#333">Status: Running</text>

  <text x="330" y="195" font-family="Arial" font-size="10" font-weight="bold" fill="#666">Dependencies:</text>
  <text x="340" y="215" font-family="Arial" font-size="9" fill="#333">→ ConfigMap: odh-dashboard-config</text>
  <text x="340" y="230" font-family="Arial" font-size="9" fill="#333">→ Service: odh-dashboard</text>
  <text x="340" y="245" font-family="Arial" font-size="9" fill="#333">→ Secret: odh-dashboard-certs</text>

  <text x="330" y="270" font-family="Arial" font-size="10" font-weight="bold" fill="#666">Chaos Experiments:</text>
  <text x="340" y="290" font-family="Arial" font-size="9" fill="#333">• pod-deletion-test (Pass)</text>
  <text x="340" y="305" font-family="Arial" font-size="9" fill="#333">• cpu-stress-test (Pass)</text>
  <text x="340" y="320" font-family="Arial" font-size="9" fill="#333">• network-partition-test (Fail)</text>

  <text x="330" y="345" font-family="Arial" font-size="10" font-weight="bold" fill="#666">Resilience Score:</text>
  <rect x="340" y="350" width="200" height="15" fill="#eee" stroke="#ccc" stroke-width="1" rx="3"/>
  <rect x="340" y="350" width="174" height="15" fill="#2E7D32" rx="3"/>
  <text x="440" y="361" text-anchor="middle" font-family="Arial" font-size="9" fill="white">87%%</text>
</svg>`, w, h, w, h, w/2, w/2, h-10)
}

// Grafana panel generators

func grafanaPieChart(w, h int) string {
	return fmt.Sprintf(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 %d %d">
  <rect width="%d" height="%d" fill="#1a1a1a" rx="4"/>
  <text x="10" y="25" font-family="Arial" font-size="14" font-weight="bold" fill="#fff">Verdict Distribution</text>
  <text x="%d" y="%d" text-anchor="middle" font-family="Arial" font-size="9" fill="#999">PLACEHOLDER - Replace with Grafana screenshot</text>

  <!-- Pie chart wireframe -->
  <circle cx="300" cy="160" r="70" fill="none" stroke="#2E7D32" stroke-width="50" stroke-dasharray="220 440"/>
  <circle cx="300" cy="160" r="70" fill="none" stroke="#C62828" stroke-width="50" stroke-dasharray="44 440" stroke-dashoffset="-220"/>
  <circle cx="300" cy="160" r="70" fill="none" stroke="#F57C00" stroke-width="50" stroke-dasharray="22 440" stroke-dashoffset="-264"/>

  <!-- Legend -->
  <rect x="400" y="100" width="15" height="15" fill="#2E7D32"/>
  <text x="420" y="112" font-family="Arial" font-size="11" fill="#fff">Pass (70%%)</text>
  <rect x="400" y="130" width="15" height="15" fill="#C62828"/>
  <text x="420" y="142" font-family="Arial" font-size="11" fill="#fff">Fail (20%%)</text>
  <rect x="400" y="160" width="15" height="15" fill="#F57C00"/>
  <text x="420" y="172" font-family="Arial" font-size="11" fill="#fff">Inconclusive (10%%)</text>
</svg>`, w, h, w, h, w/2, h-10)
}

func grafanaHistogram(w, h int) string {
	return fmt.Sprintf(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 %d %d">
  <rect width="%d" height="%d" fill="#1a1a1a" rx="4"/>
  <text x="10" y="25" font-family="Arial" font-size="14" font-weight="bold" fill="#fff">Recovery Time Distribution</text>
  <text x="%d" y="%d" text-anchor="middle" font-family="Arial" font-size="9" fill="#999">PLACEHOLDER - Replace with Grafana screenshot</text>

  <!-- Axes -->
  <line x1="50" y1="250" x2="550" y2="250" stroke="#555" stroke-width="1"/>
  <line x1="50" y1="60" x2="50" y2="250" stroke="#555" stroke-width="1"/>

  <!-- Bars -->
  <rect x="70" y="180" width="40" height="70" fill="#1565C0" fill-opacity="0.8"/>
  <rect x="130" y="140" width="40" height="110" fill="#1565C0" fill-opacity="0.8"/>
  <rect x="190" y="100" width="40" height="150" fill="#1565C0" fill-opacity="0.8"/>
  <rect x="250" y="120" width="40" height="130" fill="#1565C0" fill-opacity="0.8"/>
  <rect x="310" y="160" width="40" height="90" fill="#1565C0" fill-opacity="0.8"/>
  <rect x="370" y="200" width="40" height="50" fill="#1565C0" fill-opacity="0.8"/>
  <rect x="430" y="220" width="40" height="30" fill="#1565C0" fill-opacity="0.8"/>
  <rect x="490" y="235" width="40" height="15" fill="#1565C0" fill-opacity="0.8"/>

  <!-- Labels -->
  <text x="90" y="268" text-anchor="middle" font-family="Arial" font-size="9" fill="#999">1m</text>
  <text x="150" y="268" text-anchor="middle" font-family="Arial" font-size="9" fill="#999">2m</text>
  <text x="210" y="268" text-anchor="middle" font-family="Arial" font-size="9" fill="#999">3m</text>
  <text x="270" y="268" text-anchor="middle" font-family="Arial" font-size="9" fill="#999">4m</text>
  <text x="330" y="268" text-anchor="middle" font-family="Arial" font-size="9" fill="#999">5m</text>
  <text x="390" y="268" text-anchor="middle" font-family="Arial" font-size="9" fill="#999">6m</text>
  <text x="450" y="268" text-anchor="middle" font-family="Arial" font-size="9" fill="#999">7m</text>
  <text x="510" y="268" text-anchor="middle" font-family="Arial" font-size="9" fill="#999">8m+</text>
</svg>`, w, h, w, h, w/2, h-10)
}

func grafanaGauge(w, h int) string {
	return fmt.Sprintf(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 %d %d">
  <rect width="%d" height="%d" fill="#1a1a1a" rx="4"/>
  <text x="10" y="25" font-family="Arial" font-size="14" font-weight="bold" fill="#fff">Active Experiments</text>
  <text x="%d" y="%d" text-anchor="middle" font-family="Arial" font-size="9" fill="#999">PLACEHOLDER - Replace with Grafana screenshot</text>

  <!-- Gauge arc -->
  <path d="M 150 200 A 100 100 0 0 1 450 200" fill="none" stroke="#333" stroke-width="30"/>
  <path d="M 150 200 A 100 100 0 0 1 390 130" fill="none" stroke="#1565C0" stroke-width="30"/>

  <!-- Center value -->
  <text x="300" y="180" text-anchor="middle" font-family="Arial" font-size="48" font-weight="bold" fill="#1565C0">3</text>
  <text x="300" y="210" text-anchor="middle" font-family="Arial" font-size="14" fill="#999">experiments</text>

  <!-- Scale labels -->
  <text x="120" y="220" font-family="Arial" font-size="11" fill="#999">0</text>
  <text x="300" y="90" text-anchor="middle" font-family="Arial" font-size="11" fill="#999">5</text>
  <text x="480" y="220" text-anchor="end" font-family="Arial" font-size="11" fill="#999">10</text>
</svg>`, w, h, w, h, w/2, h-10)
}

func grafanaBarChart(w, h int) string {
	return fmt.Sprintf(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 %d %d">
  <rect width="%d" height="%d" fill="#1a1a1a" rx="4"/>
  <text x="10" y="25" font-family="Arial" font-size="14" font-weight="bold" fill="#fff">Injection Types</text>
  <text x="%d" y="%d" text-anchor="middle" font-family="Arial" font-size="9" fill="#999">PLACEHOLDER - Replace with Grafana screenshot</text>

  <!-- Axes -->
  <line x1="100" y1="250" x2="550" y2="250" stroke="#555" stroke-width="1"/>
  <line x1="100" y1="60" x2="100" y2="250" stroke="#555" stroke-width="1"/>

  <!-- Horizontal bars -->
  <rect x="105" y="70" width="280" height="25" fill="#1565C0" fill-opacity="0.8"/>
  <text x="110" y="87" font-family="Arial" font-size="10" fill="#fff">PodFailure</text>
  <text x="390" y="87" font-family="Arial" font-size="10" fill="#fff">28</text>

  <rect x="105" y="105" width="210" height="25" fill="#2E7D32" fill-opacity="0.8"/>
  <text x="110" y="122" font-family="Arial" font-size="10" fill="#fff">NetworkChaos</text>
  <text x="320" y="122" font-family="Arial" font-size="10" fill="#fff">21</text>

  <rect x="105" y="140" width="180" height="25" fill="#F57C00" fill-opacity="0.8"/>
  <text x="110" y="157" font-family="Arial" font-size="10" fill="#fff">StressChaos</text>
  <text x="290" y="157" font-family="Arial" font-size="10" fill="#fff">18</text>

  <rect x="105" y="175" width="140" height="25" fill="#5E35B1" fill-opacity="0.8"/>
  <text x="110" y="192" font-family="Arial" font-size="10" fill="#fff">IOChaos</text>
  <text x="250" y="192" font-family="Arial" font-size="10" fill="#fff">14</text>

  <rect x="105" y="210" width="90" height="25" fill="#C62828" fill-opacity="0.8"/>
  <text x="110" y="227" font-family="Arial" font-size="10" fill="#fff">TimeChaos</text>
  <text x="200" y="227" font-family="Arial" font-size="10" fill="#fff">9</text>
</svg>`, w, h, w, h, w/2, h-10)
}

func grafanaTimeSeries(w, h int) string {
	return fmt.Sprintf(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 %d %d">
  <rect width="%d" height="%d" fill="#1a1a1a" rx="4"/>
  <text x="10" y="25" font-family="Arial" font-size="14" font-weight="bold" fill="#fff">Experiment Timeline</text>
  <text x="%d" y="%d" text-anchor="middle" font-family="Arial" font-size="9" fill="#999">PLACEHOLDER - Replace with Grafana screenshot</text>

  <!-- Grid lines -->
  <line x1="50" y1="60" x2="550" y2="60" stroke="#333" stroke-width="1" stroke-dasharray="2,2"/>
  <line x1="50" y1="110" x2="550" y2="110" stroke="#333" stroke-width="1" stroke-dasharray="2,2"/>
  <line x1="50" y1="160" x2="550" y2="160" stroke="#333" stroke-width="1" stroke-dasharray="2,2"/>
  <line x1="50" y1="210" x2="550" y2="210" stroke="#333" stroke-width="1" stroke-dasharray="2,2"/>

  <!-- Axes -->
  <line x1="50" y1="260" x2="550" y2="260" stroke="#555" stroke-width="1"/>
  <line x1="50" y1="60" x2="50" y2="260" stroke="#555" stroke-width="1"/>

  <!-- Time series line -->
  <polyline points="50,200 100,180 150,160 200,140 250,150 300,120 350,110 400,130 450,100 500,90 550,85"
            fill="none" stroke="#1565C0" stroke-width="2"/>

  <!-- Data points -->
  <circle cx="50" cy="200" r="3" fill="#1565C0"/>
  <circle cx="100" cy="180" r="3" fill="#1565C0"/>
  <circle cx="150" cy="160" r="3" fill="#1565C0"/>
  <circle cx="200" cy="140" r="3" fill="#1565C0"/>
  <circle cx="250" cy="150" r="3" fill="#1565C0"/>
  <circle cx="300" cy="120" r="3" fill="#1565C0"/>
  <circle cx="350" cy="110" r="3" fill="#1565C0"/>
  <circle cx="400" cy="130" r="3" fill="#1565C0"/>
  <circle cx="450" cy="100" r="3" fill="#1565C0"/>
  <circle cx="500" cy="90" r="3" fill="#1565C0"/>
  <circle cx="550" cy="85" r="3" fill="#1565C0"/>

  <!-- Time labels -->
  <text x="50" y="278" text-anchor="middle" font-family="Arial" font-size="9" fill="#999">00:00</text>
  <text x="200" y="278" text-anchor="middle" font-family="Arial" font-size="9" fill="#999">06:00</text>
  <text x="350" y="278" text-anchor="middle" font-family="Arial" font-size="9" fill="#999">12:00</text>
  <text x="500" y="278" text-anchor="middle" font-family="Arial" font-size="9" fill="#999">18:00</text>

  <!-- Y-axis labels -->
  <text x="40" y="65" text-anchor="end" font-family="Arial" font-size="9" fill="#999">10</text>
  <text x="40" y="115" text-anchor="end" font-family="Arial" font-size="9" fill="#999">7.5</text>
  <text x="40" y="165" text-anchor="end" font-family="Arial" font-size="9" fill="#999">5</text>
  <text x="40" y="215" text-anchor="end" font-family="Arial" font-size="9" fill="#999">2.5</text>
  <text x="40" y="265" text-anchor="end" font-family="Arial" font-size="9" fill="#999">0</text>
</svg>`, w, h, w, h, w/2, h-10)
}

func grafanaTable(w, h int) string {
	return fmt.Sprintf(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 %d %d">
  <rect width="%d" height="%d" fill="#1a1a1a" rx="4"/>
  <text x="10" y="25" font-family="Arial" font-size="14" font-weight="bold" fill="#fff">Component Health</text>
  <text x="%d" y="%d" text-anchor="middle" font-family="Arial" font-size="9" fill="#999">PLACEHOLDER - Replace with Grafana screenshot</text>

  <!-- Table headers -->
  <rect x="20" y="50" width="560" height="30" fill="#2a2a2a"/>
  <text x="30" y="70" font-family="Arial" font-size="11" font-weight="bold" fill="#ccc">Component</text>
  <text x="200" y="70" font-family="Arial" font-size="11" font-weight="bold" fill="#ccc">Status</text>
  <text x="320" y="70" font-family="Arial" font-size="11" font-weight="bold" fill="#ccc">Experiments</text>
  <text x="450" y="70" font-family="Arial" font-size="11" font-weight="bold" fill="#ccc">Resilience</text>

  <!-- Table rows -->
  <rect x="20" y="80" width="560" height="30" fill="#1f1f1f"/>
  <text x="30" y="100" font-family="Arial" font-size="10" fill="#fff">dashboard-controller</text>
  <circle cx="215" cy="96" r="5" fill="#2E7D32"/>
  <text x="230" y="100" font-family="Arial" font-size="10" fill="#fff">Healthy</text>
  <text x="340" y="100" font-family="Arial" font-size="10" fill="#fff">12</text>
  <rect x="460" y="90" width="80" height="15" fill="#2E7D32" fill-opacity="0.3" rx="2"/>
  <text x="500" y="101" text-anchor="middle" font-family="Arial" font-size="9" fill="#fff">87%%</text>

  <rect x="20" y="110" width="560" height="30" fill="#252525"/>
  <text x="30" y="130" font-family="Arial" font-size="10" fill="#fff">kserve-controller</text>
  <circle cx="215" cy="126" r="5" fill="#F57C00"/>
  <text x="230" y="130" font-family="Arial" font-size="10" fill="#fff">Degraded</text>
  <text x="340" y="130" font-family="Arial" font-size="10" fill="#fff">8</text>
  <rect x="460" y="120" width="50" height="15" fill="#F57C00" fill-opacity="0.3" rx="2"/>
  <text x="485" y="131" text-anchor="middle" font-family="Arial" font-size="9" fill="#fff">62%%</text>

  <rect x="20" y="140" width="560" height="30" fill="#1f1f1f"/>
  <text x="30" y="160" font-family="Arial" font-size="10" fill="#fff">model-mesh</text>
  <circle cx="215" cy="156" r="5" fill="#2E7D32"/>
  <text x="230" y="160" font-family="Arial" font-size="10" fill="#fff">Healthy</text>
  <text x="340" y="160" font-family="Arial" font-size="10" fill="#fff">15</text>
  <rect x="460" y="150" width="75" height="15" fill="#2E7D32" fill-opacity="0.3" rx="2"/>
  <text x="497" y="161" text-anchor="middle" font-family="Arial" font-size="9" fill="#fff">93%%</text>

  <rect x="20" y="170" width="560" height="30" fill="#252525"/>
  <text x="30" y="190" font-family="Arial" font-size="10" fill="#fff">workbenches</text>
  <circle cx="215" cy="186" r="5" fill="#2E7D32"/>
  <text x="230" y="190" font-family="Arial" font-size="10" fill="#fff">Healthy</text>
  <text x="340" y="190" font-family="Arial" font-size="10" fill="#fff">10</text>
  <rect x="460" y="180" width="70" height="15" fill="#2E7D32" fill-opacity="0.3" rx="2"/>
  <text x="495" y="191" text-anchor="middle" font-family="Arial" font-size="9" fill="#fff">85%%</text>

  <rect x="20" y="200" width="560" height="30" fill="#1f1f1f"/>
  <text x="30" y="220" font-family="Arial" font-size="10" fill="#fff">pipelines</text>
  <circle cx="215" cy="216" r="5" fill="#C62828"/>
  <text x="230" y="220" font-family="Arial" font-size="10" fill="#fff">Unhealthy</text>
  <text x="340" y="220" font-family="Arial" font-size="10" fill="#fff">6</text>
  <rect x="460" y="210" width="35" height="15" fill="#C62828" fill-opacity="0.3" rx="2"/>
  <text x="477" y="221" text-anchor="middle" font-family="Arial" font-size="9" fill="#fff">45%%</text>
</svg>`, w, h, w, h, w/2, h-10)
}
