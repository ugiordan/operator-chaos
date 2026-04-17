//go:build ignore

package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"text/template"

	sigsyaml "sigs.k8s.io/yaml"
)

// ---------------------------------------------------------------------------
// Data structures (lightweight, no dependency on internal API types)
// ---------------------------------------------------------------------------

type knowledge struct {
	Operator   operatorMeta    `json:"operator"`
	Components []componentData `json:"components"`
	Recovery   recoveryData    `json:"recovery"`
}

type operatorMeta struct {
	Name       string `json:"name"`
	Namespace  string `json:"namespace"`
	Repository string `json:"repository"`
}

type componentData struct {
	Name             string            `json:"name"`
	Controller       string            `json:"controller"`
	ManagedResources []managedResource `json:"managedResources"`
	Dependencies     []string          `json:"dependencies"`
	Webhooks         []webhookData     `json:"webhooks"`
	Finalizers       []string          `json:"finalizers"`
	SteadyState      steadyStateData   `json:"steadyState"`
}

type managedResource struct {
	APIVersion string `json:"apiVersion"`
	Kind       string `json:"kind"`
	Name       string `json:"name"`
	Namespace  string `json:"namespace"`
}

type webhookData struct {
	Name string `json:"name"`
	Type string `json:"type"`
	Path string `json:"path"`
}

type steadyStateData struct {
	Checks  []steadyCheck `json:"checks"`
	Timeout string        `json:"timeout"`
}

type steadyCheck struct {
	Type          string `json:"type"`
	APIVersion    string `json:"apiVersion"`
	Kind          string `json:"kind"`
	Name          string `json:"name"`
	Namespace     string `json:"namespace"`
	ConditionType string `json:"conditionType"`
}

type recoveryData struct {
	ReconcileTimeout   string `json:"reconcileTimeout"`
	MaxReconcileCycles int    `json:"maxReconcileCycles"`
}

type experimentInfo struct {
	Name          string
	Filename      string
	InjectionType string
	DangerLevel   string
	Component     string
	Description   string
	RawYAML       string
}

type componentPage struct {
	OperatorName string
	DirName      string
	Components   []componentData
	Experiments  []experimentInfo
	Repository   string
	Namespace    string
	Recovery     recoveryData
}

// experimentYAML is the YAML shape we unmarshal from experiment files.
type experimentYAML struct {
	Metadata struct {
		Name string `json:"name"`
	} `json:"metadata"`
	Spec struct {
		Target struct {
			Component string `json:"component"`
		} `json:"target"`
		Injection struct {
			Type        string `json:"type"`
			DangerLevel string `json:"dangerLevel"`
		} `json:"injection"`
		Hypothesis struct {
			Description string `json:"description"`
		} `json:"hypothesis"`
	} `json:"spec"`
}

// ---------------------------------------------------------------------------
// Core functions
// ---------------------------------------------------------------------------

func loadKnowledge(path string) (*knowledge, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	var k knowledge
	if err := sigsyaml.Unmarshal(data, &k); err != nil {
		return nil, fmt.Errorf("unmarshal %s: %w", path, err)
	}
	return &k, nil
}

func loadExperiments(experimentsDir, componentDirName string) ([]experimentInfo, error) {
	dir := filepath.Join(experimentsDir, componentDirName)
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var exps []experimentInfo
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".yaml") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			return nil, err
		}
		var ey experimentYAML
		if err := sigsyaml.Unmarshal(data, &ey); err != nil {
			return nil, fmt.Errorf("unmarshal experiment %s: %w", e.Name(), err)
		}
		danger := ey.Spec.Injection.DangerLevel
		if danger == "" {
			danger = dangerForType(ey.Spec.Injection.Type)
		}
		exps = append(exps, experimentInfo{
			Name:          ey.Metadata.Name,
			Filename:      e.Name(),
			InjectionType: ey.Spec.Injection.Type,
			DangerLevel:   danger,
			Component:     ey.Spec.Target.Component,
			Description:   strings.TrimSpace(ey.Spec.Hypothesis.Description),
			RawYAML:       string(data),
		})
	}
	sort.Slice(exps, func(i, j int) bool {
		return exps[i].Filename < exps[j].Filename
	})
	return exps, nil
}

func extractCustomSections(content string) map[string]string {
	sections := make(map[string]string)
	lines := strings.Split(content, "\n")

	for i := 0; i < len(lines); i++ {
		line := strings.TrimSpace(lines[i])
		if !strings.HasPrefix(line, "<!-- custom-start:") {
			continue
		}
		name := strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(line, "<!-- custom-start:"), "-->"))
		endMarker := fmt.Sprintf("<!-- custom-end: %s -->", name)

		// Find end marker
		found := false
		var buf strings.Builder
		for j := i + 1; j < len(lines); j++ {
			if strings.TrimSpace(lines[j]) == endMarker {
				found = true
				i = j
				break
			}
			buf.WriteString(lines[j])
			buf.WriteByte('\n')
		}
		if !found {
			// Malformed: return empty map
			return make(map[string]string)
		}
		sections[name] = buf.String()
	}
	return sections
}

func injectCustomSections(content string, sections map[string]string) string {
	if len(sections) == 0 {
		return content
	}
	lines := strings.Split(content, "\n")
	var out strings.Builder

	for i := 0; i < len(lines); i++ {
		line := strings.TrimSpace(lines[i])
		if !strings.HasPrefix(line, "<!-- custom-start:") {
			out.WriteString(lines[i])
			if i < len(lines)-1 {
				out.WriteByte('\n')
			}
			continue
		}
		name := strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(line, "<!-- custom-start:"), "-->"))
		out.WriteString(lines[i])
		out.WriteByte('\n')

		// Write custom content if we have it
		if customContent, ok := sections[name]; ok && customContent != "" {
			out.WriteString(customContent)
		}

		// Skip to end marker
		for j := i + 1; j < len(lines); j++ {
			endMarker := fmt.Sprintf("<!-- custom-end: %s -->", name)
			if strings.TrimSpace(lines[j]) == endMarker {
				i = j
				break
			}
		}
		out.WriteString(lines[i])
		if i < len(lines)-1 {
			out.WriteByte('\n')
		}
	}
	return out.String()
}

func countResources(resources []managedResource) map[string]int {
	counts := make(map[string]int)
	for _, r := range resources {
		counts[r.Kind]++
	}
	return counts
}

func dangerForType(t string) string {
	switch t {
	case "PodKill":
		return "low"
	case "NetworkPartition":
		return "medium"
	case "ConfigDrift":
		return "medium"
	case "CRDMutation":
		return "medium"
	case "FinalizerBlock":
		return "low"
	case "RBACRevoke":
		return "high"
	case "WebhookDisrupt":
		return "high"
	case "ClientFault":
		return "low"
	default:
		return "medium"
	}
}

func writeWithPreservation(path string, tmpl *template.Template, data any) error {
	var sections map[string]string
	if existing, err := os.ReadFile(path); err == nil {
		sections = extractCustomSections(string(existing))
	}

	var buf strings.Builder
	if err := tmpl.Execute(&buf, data); err != nil {
		return fmt.Errorf("execute template for %s: %w", path, err)
	}

	output := buf.String()
	if len(sections) > 0 {
		output = injectCustomSections(output, sections)
	}

	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(output), 0644)
}

// ---------------------------------------------------------------------------
// Template helper functions
// ---------------------------------------------------------------------------

func makeFuncMap() template.FuncMap {
	return template.FuncMap{
		"join": strings.Join,
		"truncate": func(n int, s string) string {
			s = strings.ReplaceAll(s, "\n", " ")
			s = strings.TrimSpace(s)
			if len(s) <= n {
				return s
			}
			return s[:n] + "..."
		},
		"indent": func(spaces int, s string) string {
			prefix := strings.Repeat(" ", spaces)
			lines := strings.Split(s, "\n")
			var out []string
			for _, l := range lines {
				if l == "" {
					out = append(out, "")
				} else {
					out = append(out, prefix+l)
				}
			}
			return strings.Join(out, "\n")
		},
		"countRes": func(components []componentData) map[string]int {
			all := make(map[string]int)
			for _, c := range components {
				for k, v := range countResources(c.ManagedResources) {
					all[k] += v
				}
			}
			return all
		},
		"allResources": func(components []componentData) []managedResource {
			var all []managedResource
			for _, c := range components {
				all = append(all, c.ManagedResources...)
			}
			return all
		},
		"coverageCell": func(experiments []experimentInfo, injType string) string {
			for _, e := range experiments {
				if e.InjectionType == injType {
					return "Y"
				}
			}
			return "-"
		},
		"sortedKinds": func(m map[string]int) []string {
			var keys []string
			for k := range m {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			return keys
		},
		"add": func(a, b int) int { return a + b },
	}
}

// ---------------------------------------------------------------------------
// Templates
// ---------------------------------------------------------------------------

var indexTmpl = `# {{ .OperatorName }}

## Overview

| Property | Value |
|----------|-------|
| **Operator** | {{ .OperatorName }} |
| **Namespace** | {{ .Namespace }} |
| **Repository** | [{{ .Repository }}]({{ .Repository }}) |
| **Components** | {{ len .Components }} |
| **Reconcile Timeout** | {{ .Recovery.ReconcileTimeout }} |
| **Max Reconcile Cycles** | {{ .Recovery.MaxReconcileCycles }} |

## Resource Summary

| Kind | Count |
|------|-------|
{{- $counts := countRes .Components }}
{{- range $k := sortedKinds $counts }}
| {{ $k }} | {{ index $counts $k }} |
{{- end }}
| **Total** | **{{ len (allResources .Components) }}** |

## Components
{{ range .Components }}
### {{ .Name }}

**Controller:** {{ .Controller }}
{{- if .Dependencies }}
**Dependencies:** {{ join .Dependencies ", " }}
{{- end }}

#### Managed Resources

| API Version | Kind | Name | Namespace |
|-------------|------|------|-----------|
{{- range .ManagedResources }}
| {{ .APIVersion }} | {{ .Kind }} | {{ .Name }} | {{ .Namespace }} |
{{- end }}
{{ if .Webhooks }}
#### Webhooks

| Name | Type | Path |
|------|------|------|
{{- range .Webhooks }}
| {{ .Name }} | {{ .Type }} | ` + "`{{ .Path }}`" + ` |
{{- end }}
{{ end }}
{{- if .Finalizers }}
#### Finalizers

{{- range .Finalizers }}
- ` + "`{{ . }}`" + `
{{- end }}
{{ end }}
#### Steady-State Checks

| Type | Kind | Name | Namespace | Condition |
|------|------|------|-----------|-----------|
{{- range .SteadyState.Checks }}
| {{ .Type }} | {{ .Kind }} | {{ .Name }} | {{ .Namespace }} | {{ .ConditionType }} |
{{- end }}

Timeout: {{ .SteadyState.Timeout }}
{{ end }}

<!-- custom-start: notes -->
<!-- custom-end: notes -->
`

var failureModesTmpl = `# {{ .OperatorName }} Failure Modes

## Coverage

| Injection Type | Danger | Experiment | Description |
|----------------|--------|------------|-------------|
{{- range .Experiments }}
| {{ .InjectionType }} | {{ .DangerLevel }} | {{ .Filename }} | {{ truncate 80 .Description }} |
{{- end }}

## Experiment Details
{{ range .Experiments }}
### {{ .Name }}

- **Type:** {{ .InjectionType }}
- **Danger Level:** {{ .DangerLevel }}
- **Component:** {{ .Component }}

{{ .Description }}

<details>
<summary>Experiment YAML</summary>

` + "```yaml" + `
{{ .RawYAML }}` + "```" + `

</details>
{{ end }}

<!-- custom-start: known-issues -->
<!-- custom-end: known-issues -->
`

var validationResultsTmpl = `# {{ .OperatorName }} Validation Results

## Results

<!-- custom-start: results -->
No validation results yet. Run experiments and document findings here.
<!-- custom-end: results -->

## Known Issues

<!-- custom-start: known-issues -->
<!-- custom-end: known-issues -->
`

var customExperimentsTmpl = `# {{ .OperatorName }} Custom Experiments

This page provides templates for writing custom chaos experiments targeting {{ .OperatorName }}.

{{ range .Components }}
## {{ .Name }}

` + "```yaml" + `
apiVersion: chaos.opendatahub.io/v1alpha1
kind: ChaosExperiment
metadata:
  name: {{ .Name }}-custom
spec:
  target:
    operator: {{ $.OperatorName }}
    component: {{ .Name }}
  steadyState:
    checks:
{{- range .SteadyState.Checks }}
      - type: {{ .Type }}
        apiVersion: {{ .APIVersion }}
        kind: {{ .Kind }}
        name: {{ .Name }}
        namespace: {{ .Namespace }}
{{- if .ConditionType }}
        conditionType: {{ .ConditionType }}
{{- end }}
{{- end }}
    timeout: "{{ .SteadyState.Timeout }}"
  injection:
    type: PodKill  # Change to desired injection type
    parameters:
      labelSelector: app={{ .Name }}
    ttl: "300s"
  hypothesis:
    description: >-
      Describe the expected behavior after fault injection.
    recoveryTimeout: 120s
` + "```" + `
{{ end }}

## Running Custom Experiments

1. Save your experiment YAML to a file
2. Run: ` + "`chaos-cli run --experiment <file>`" + `
3. Check results: ` + "`chaos-cli results --latest`" + `

<!-- custom-start: examples -->
<!-- custom-end: examples -->
`

var overviewTmpl = `# Component Overview

Coverage matrix showing which failure modes have experiments defined for each component.

| Component | PodKill | ConfigDrift | CRDMutation | FinalizerBlock | NetworkPartition | RBACRevoke | WebhookDisrupt | ClientFault |
|-----------|---------|-------------|-------------|----------------|------------------|------------|----------------|-------------|
{{- range . }}
| [{{ .DirName }}]({{ .DirName }}/index.md) | {{ coverageCell .Experiments "PodKill" }} | {{ coverageCell .Experiments "ConfigDrift" }} | {{ coverageCell .Experiments "CRDMutation" }} | {{ coverageCell .Experiments "FinalizerBlock" }} | {{ coverageCell .Experiments "NetworkPartition" }} | {{ coverageCell .Experiments "RBACRevoke" }} | {{ coverageCell .Experiments "WebhookDisrupt" }} | {{ coverageCell .Experiments "ClientFault" }} |
{{- end }}
`

// ---------------------------------------------------------------------------
// Main
// ---------------------------------------------------------------------------

func main() {
	knowledgeDir := flag.String("knowledge-dir", "knowledge", "Directory containing knowledge YAML files")
	experimentsDir := flag.String("experiments-dir", "experiments", "Directory containing experiment subdirectories")
	outputDir := flag.String("output-dir", "site/docs/components", "Output directory for generated docs")
	flag.Parse()

	funcMap := makeFuncMap()

	tmplIndex := template.Must(template.New("index").Funcs(funcMap).Parse(indexTmpl))
	tmplFailure := template.Must(template.New("failure").Funcs(funcMap).Parse(failureModesTmpl))
	tmplValidation := template.Must(template.New("validation").Funcs(funcMap).Parse(validationResultsTmpl))
	tmplCustom := template.Must(template.New("custom").Funcs(funcMap).Parse(customExperimentsTmpl))
	tmplOverview := template.Must(template.New("overview").Funcs(funcMap).Parse(overviewTmpl))

	// Read knowledge YAMLs (top-level only, skip subdirs)
	entries, err := os.ReadDir(*knowledgeDir)
	if err != nil {
		log.Fatalf("read knowledge dir: %v", err)
	}

	var allPages []componentPage

	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".yaml") {
			continue
		}
		kPath := filepath.Join(*knowledgeDir, e.Name())
		k, err := loadKnowledge(kPath)
		if err != nil {
			log.Fatalf("load knowledge %s: %v", e.Name(), err)
		}

		dirName := strings.TrimSuffix(e.Name(), ".yaml")
		exps, err := loadExperiments(*experimentsDir, dirName)
		if err != nil {
			log.Fatalf("load experiments for %s: %v", dirName, err)
		}

		page := componentPage{
			OperatorName: k.Operator.Name,
			DirName:      dirName,
			Components:   k.Components,
			Experiments:  exps,
			Repository:   k.Operator.Repository,
			Namespace:    k.Operator.Namespace,
			Recovery:     k.Recovery,
		}
		allPages = append(allPages, page)

		outDir := filepath.Join(*outputDir, dirName)

		if err := writeWithPreservation(filepath.Join(outDir, "index.md"), tmplIndex, page); err != nil {
			log.Fatalf("write index for %s: %v", dirName, err)
		}
		if err := writeWithPreservation(filepath.Join(outDir, "failure-modes.md"), tmplFailure, page); err != nil {
			log.Fatalf("write failure-modes for %s: %v", dirName, err)
		}
		if err := writeWithPreservation(filepath.Join(outDir, "validation-results.md"), tmplValidation, page); err != nil {
			log.Fatalf("write validation-results for %s: %v", dirName, err)
		}
		if err := writeWithPreservation(filepath.Join(outDir, "custom-experiments.md"), tmplCustom, page); err != nil {
			log.Fatalf("write custom-experiments for %s: %v", dirName, err)
		}

		fmt.Printf("generated: %s/ (components=%d, experiments=%d)\n", dirName, len(k.Components), len(exps))
	}

	// Sort by DirName for overview
	sort.Slice(allPages, func(i, j int) bool {
		return allPages[i].DirName < allPages[j].DirName
	})

	if err := writeWithPreservation(filepath.Join(*outputDir, "index.md"), tmplOverview, allPages); err != nil {
		log.Fatalf("write overview: %v", err)
	}
	fmt.Printf("generated: index.md (overview, %d components)\n", len(allPages))
}
