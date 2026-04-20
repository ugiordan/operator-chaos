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
// Data structures
// ---------------------------------------------------------------------------

type failureModeMeta struct {
	Name        string      `json:"name"`
	Type        string      `json:"type"`
	Danger      string      `json:"danger"`
	Description string      `json:"description"`
	SpecFields  []specField `json:"spec_fields"`
	Body        string      // markdown body after frontmatter
	SrcFilename string      // original filename, e.g. "podkill.md"
}

type specField struct {
	Name        string `json:"name"`
	Type        string `json:"type"`
	Required    bool   `json:"required"`
	Default     string `json:"default"`
	Description string `json:"description"`
}

type experimentRef struct {
	Name        string
	Component   string // directory name, e.g. "odh-model-controller"
	Filename    string
	DangerLevel string
	Description string
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

// typePageData is the template data for a per-type page.
type typePageData struct {
	Meta        failureModeMeta
	Experiments []experimentRef
	DangerBadge string
}

// overviewData is the template data for the overview page.
type overviewData struct {
	Modes    []failureModeMeta
	ByType   map[string][]experimentRef
	Coverage []coverageRow
}

type coverageRow struct {
	Component string
	Cells     map[string]bool
	Total     int
}

// ---------------------------------------------------------------------------
// Core functions
// ---------------------------------------------------------------------------

func loadMetadata(path string) (*failureModeMeta, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	content := string(data)

	// Split on --- frontmatter delimiters.
	// Format: starts with ---, then YAML, then ---, then markdown body.
	parts := strings.SplitN(content, "---", 3)
	if len(parts) < 3 {
		return nil, fmt.Errorf("invalid frontmatter in %s: expected --- delimiters", path)
	}

	frontmatter := strings.TrimSpace(parts[1])
	body := strings.TrimSpace(parts[2])

	var meta failureModeMeta
	if err := sigsyaml.Unmarshal([]byte(frontmatter), &meta); err != nil {
		return nil, fmt.Errorf("unmarshal frontmatter in %s: %w", path, err)
	}
	meta.Body = body
	meta.SrcFilename = filepath.Base(path)
	return &meta, nil
}

func loadAllExperiments(experimentsDir string) (map[string][]experimentRef, error) {
	result := make(map[string][]experimentRef)

	topEntries, err := os.ReadDir(experimentsDir)
	if err != nil {
		return nil, fmt.Errorf("read experiments dir: %w", err)
	}

	for _, topEntry := range topEntries {
		if !topEntry.IsDir() {
			continue
		}
		componentDir := topEntry.Name()
		dirPath := filepath.Join(experimentsDir, componentDir)
		entries, err := os.ReadDir(dirPath)
		if err != nil {
			return nil, fmt.Errorf("read experiments subdir %s: %w", componentDir, err)
		}

		for _, e := range entries {
			if e.IsDir() || !strings.HasSuffix(e.Name(), ".yaml") {
				continue
			}
			data, err := os.ReadFile(filepath.Join(dirPath, e.Name()))
			if err != nil {
				return nil, fmt.Errorf("read experiment %s/%s: %w", componentDir, e.Name(), err)
			}
			var ey experimentYAML
			if err := sigsyaml.Unmarshal(data, &ey); err != nil {
				return nil, fmt.Errorf("unmarshal experiment %s/%s: %w", componentDir, e.Name(), err)
			}

			danger := ey.Spec.Injection.DangerLevel
			if danger == "" {
				danger = dangerForType(ey.Spec.Injection.Type)
			}

			ref := experimentRef{
				Name:        ey.Metadata.Name,
				Component:   componentDir,
				Filename:    e.Name(),
				DangerLevel: danger,
				Description: strings.TrimSpace(ey.Spec.Hypothesis.Description),
			}
			injType := ey.Spec.Injection.Type
			result[injType] = append(result[injType], ref)
		}
	}

	// Sort each type's experiments by component then filename.
	for k := range result {
		sort.Slice(result[k], func(i, j int) bool {
			if result[k][i].Component != result[k][j].Component {
				return result[k][i].Component < result[k][j].Component
			}
			return result[k][i].Filename < result[k][j].Filename
		})
	}

	return result, nil
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
	case "OwnerRefOrphan":
		return "medium"
	case "QuotaExhaustion":
		return "medium"
	case "WebhookLatency":
		return "high"
	default:
		return "medium"
	}
}

func dangerBadge(level string) string {
	switch strings.ToLower(level) {
	case "low":
		return ":material-shield-check: Low"
	case "medium":
		return ":material-shield-alert: Medium"
	case "high":
		return ":material-shield-remove: High"
	default:
		return level
	}
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

		if customContent, ok := sections[name]; ok && customContent != "" {
			out.WriteString(customContent)
		}

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
// Template helpers
// ---------------------------------------------------------------------------

func makeFuncMap() template.FuncMap {
	return template.FuncMap{
		"dangerBadge": dangerBadge,
		"truncate": func(n int, s string) string {
			s = strings.ReplaceAll(s, "\n", " ")
			s = strings.TrimSpace(s)
			if len(s) <= n {
				return s
			}
			return s[:n] + "..."
		},
		"lower": strings.ToLower,
		"hasExperiments": func(byType map[string][]experimentRef, injType string) bool {
			return len(byType[injType]) > 0
		},
		"getExperiments": func(byType map[string][]experimentRef, injType string) []experimentRef {
			return byType[injType]
		},
		"coverageCheck": func(cells map[string]bool, injType string) string {
			if cells[injType] {
				return ":material-check:"
			}
			return "-"
		},
	}
}

// ---------------------------------------------------------------------------
// Templates
// ---------------------------------------------------------------------------

var typePageTmpl = `# {{ .Meta.Name }}

**Danger Level:** {{ .DangerBadge }}

{{ .Meta.Description }}

## Spec Fields

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
{{- range .Meta.SpecFields }}
| ` + "`{{ .Name }}`" + ` | ` + "`{{ .Type }}`" + ` | {{ if .Required }}Yes{{ else }}No{{ end }} | {{ if .Default }}` + "`{{ .Default }}`" + `{{ else }}-{{ end }} | {{ .Description }} |
{{- end }}

{{ .Meta.Body }}

## Cross-Component Results

{{ if .Experiments -}}
| Component | Experiment | Danger | Description |
|-----------|------------|--------|-------------|
{{- range .Experiments }}
| {{ .Component }} | {{ .Name }} | {{ .DangerLevel }} | {{ truncate 80 .Description }} |
{{- end }}
{{- else -}}
!!! info "No experiments defined"
    No experiments have been defined for this failure mode yet. See the [custom failure modes guide](custom-failure-modes.md) for how to create one.
{{- end }}

<!-- custom-start: notes -->
<!-- custom-end: notes -->
`

var overviewTmpl = `# Failure Modes Overview

Overview of all failure injection types available in Operator Chaos.

## Quick Reference

| Type | Danger | Description |
|------|--------|-------------|
{{- range .Modes }}
| [{{ .Name }}]({{ .SrcFilename }}) | {{ dangerBadge .Danger }} | {{ .Description }} |
{{- end }}

## Decision Tree

Which failure mode should I use?

` + "```mermaid" + `
graph TD
    A[What are you testing?] --> B{Pod lifecycle?}
    B -->|Yes| C[PodKill]
    A --> D{Network resilience?}
    D -->|Yes| E[NetworkPartition]
    A --> F{Config reconciliation?}
    F -->|Yes| G[ConfigDrift]
    A --> H{CR spec handling?}
    H -->|Yes| I[CRDMutation]
    A --> J{Webhook resilience?}
    J -->|Yes| K[WebhookDisrupt]
    A --> L{Permission handling?}
    L -->|Yes| M[RBACRevoke]
    A --> N{Deletion/cleanup?}
    N -->|Yes| O[FinalizerBlock]
    A --> P{API error handling?}
    P -->|Yes| Q[ClientFault]
    A --> R{Ownership/adoption?}
    R -->|Yes| S[OwnerRefOrphan]
    A --> T{Resource pressure?}
    T -->|Yes| U[QuotaExhaustion]
    A --> V{API latency?}
    V -->|Yes| W[WebhookLatency]
` + "```" + `

## Coverage by Component

| Component |{{- range .Modes }} {{ .Name }} |{{- end }} Total |
|-----------|{{- range .Modes }}--------|{{- end }}-------|
{{- range .Coverage }}
| {{ .Component }} |{{- $cells := .Cells }}{{- range $.Modes }} {{ coverageCheck $cells .Type }} |{{- end }} {{ .Total }} |
{{- end }}

<!-- custom-start: notes -->
<!-- custom-end: notes -->
`

// ---------------------------------------------------------------------------
// Main
// ---------------------------------------------------------------------------

func main() {
	metadataDir := flag.String("metadata-dir", "hack/failure-mode-metadata", "Directory containing failure mode metadata fragments")
	experimentsDir := flag.String("experiments-dir", "experiments", "Directory containing experiment subdirectories")
	outputDir := flag.String("output-dir", "site/docs/failure-modes", "Output directory for generated failure mode docs")
	flag.Parse()

	funcMap := makeFuncMap()

	tmplType := template.Must(template.New("typePage").Funcs(funcMap).Parse(typePageTmpl))
	tmplOverview := template.Must(template.New("overview").Funcs(funcMap).Parse(overviewTmpl))

	// 1. Read all metadata fragments.
	metaEntries, err := os.ReadDir(*metadataDir)
	if err != nil {
		log.Fatalf("read metadata dir: %v", err)
	}

	var allModes []failureModeMeta
	metaByFilename := make(map[string]failureModeMeta) // keyed by base filename (e.g. "podkill.md")

	for _, e := range metaEntries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		meta, err := loadMetadata(filepath.Join(*metadataDir, e.Name()))
		if err != nil {
			log.Fatalf("load metadata %s: %v", e.Name(), err)
		}
		allModes = append(allModes, *meta)
		metaByFilename[e.Name()] = *meta
	}

	// Sort modes by name for consistent output.
	sort.Slice(allModes, func(i, j int) bool {
		return allModes[i].Name < allModes[j].Name
	})

	// 2. Load all experiments grouped by injection type.
	expsByType, err := loadAllExperiments(*experimentsDir)
	if err != nil {
		log.Fatalf("load experiments: %v", err)
	}

	// 3. Generate per-type pages.
	for filename, meta := range metaByFilename {
		exps := expsByType[meta.Type]

		pageData := typePageData{
			Meta:        meta,
			Experiments: exps,
			DangerBadge: dangerBadge(meta.Danger),
		}

		outPath := filepath.Join(*outputDir, filename)
		if err := writeWithPreservation(outPath, tmplType, pageData); err != nil {
			log.Fatalf("write type page %s: %v", filename, err)
		}
		fmt.Printf("generated: failure-modes/%s (experiments=%d)\n", filename, len(exps))
	}

	// 4. Build coverage matrix.
	componentSet := make(map[string]map[string]bool) // component -> type -> true
	for injType, refs := range expsByType {
		for _, ref := range refs {
			if componentSet[ref.Component] == nil {
				componentSet[ref.Component] = make(map[string]bool)
			}
			componentSet[ref.Component][injType] = true
		}
	}

	var coverageRows []coverageRow
	for comp, cells := range componentSet {
		total := 0
		for _, v := range cells {
			if v {
				total++
			}
		}
		coverageRows = append(coverageRows, coverageRow{
			Component: comp,
			Cells:     cells,
			Total:     total,
		})
	}
	sort.Slice(coverageRows, func(i, j int) bool {
		return coverageRows[i].Component < coverageRows[j].Component
	})

	// 5. Generate overview page.
	ovData := overviewData{
		Modes:    allModes,
		ByType:   expsByType,
		Coverage: coverageRows,
	}

	if err := writeWithPreservation(filepath.Join(*outputDir, "index.md"), tmplOverview, ovData); err != nil {
		log.Fatalf("write overview: %v", err)
	}
	fmt.Printf("generated: failure-modes/index.md (modes=%d, components=%d)\n", len(allModes), len(coverageRows))
}
