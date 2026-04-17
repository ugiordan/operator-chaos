package diff

import (
	"encoding/json"
	"fmt"
	"io"
	"text/tabwriter"

	"sigs.k8s.io/yaml"
)

// FormatUpgradeDiff formats an UpgradeDiff as table (default), json, or yaml.
func FormatUpgradeDiff(w io.Writer, d *UpgradeDiff, format string) error {
	switch format {
	case "json":
		return formatUpgradeDiffJSON(w, d)
	case "yaml":
		return formatUpgradeDiffYAML(w, d)
	case "table", "":
		return formatUpgradeDiffTable(w, d)
	default:
		return fmt.Errorf("unknown format: %s", format)
	}
}

// FormatCRDDiffReport formats CRD diff report as table (default), json, or yaml.
func FormatCRDDiffReport(w io.Writer, r *CRDDiffReport, format string) error {
	switch format {
	case "json":
		return formatCRDDiffReportJSON(w, r)
	case "yaml":
		return formatCRDDiffReportYAML(w, r)
	case "table", "":
		return formatCRDDiffReportTable(w, r)
	default:
		return fmt.Errorf("unknown format: %s", format)
	}
}

func formatUpgradeDiffJSON(w io.Writer, d *UpgradeDiff) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(d)
}

func formatUpgradeDiffYAML(w io.Writer, d *UpgradeDiff) error {
	data, err := yaml.Marshal(d)
	if err != nil {
		return err
	}
	_, err = w.Write(data)
	return err
}

func formatUpgradeDiffTable(w io.Writer, d *UpgradeDiff) error {
	// Header
	_, _ = fmt.Fprintf(w, "Upgrade Diff: %s v%s → v%s\n\n", d.Platform, d.SourceVersion, d.TargetVersion)

	// COMPONENT CHANGES section
	_, _ = fmt.Fprintf(w, "COMPONENT CHANGES\n")
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintf(tw, "Operator\tComponent\tChange\tDetails\n")

	for _, c := range d.Components {
		details := ""
		if c.ChangeType == ComponentRenamed {
			details = fmt.Sprintf("%s → %s", c.RenamedFrom, c.Component)
		}
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", c.Operator, c.Component, c.ChangeType, details)

		// Show namespace change on separate line
		if c.NamespaceChange != nil {
			_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s → %s\n",
				c.Operator, c.Component, "Namespace Move",
				c.NamespaceChange.From, c.NamespaceChange.To)
		}
	}
	_ = tw.Flush()
	_, _ = fmt.Fprintf(w, "\n")

	// RESOURCE CHANGES section
	_, _ = fmt.Fprintf(w, "RESOURCE CHANGES\n")
	tw = tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintf(tw, "Component\tResource (Kind/Name)\tChange\tDetails\n")

	hasResources := false
	for _, c := range d.Components {
		for _, r := range c.ResourceDiffs {
			hasResources = true
			details := ""
			switch r.ChangeType {
			case ResourceRenamed:
				details = fmt.Sprintf("%s → %s", r.RenamedFrom, r.Name)
			case ResourceMoved:
				details = fmt.Sprintf("%s → (current)", r.MovedFrom)
			}
			resource := fmt.Sprintf("%s/%s", r.Kind, r.Name)
			_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", c.Component, resource, r.ChangeType, details)
		}
	}

	if !hasResources {
		_, _ = fmt.Fprintf(tw, "(none)\t\t\t\n")
	}
	_ = tw.Flush()
	_, _ = fmt.Fprintf(w, "\n")

	// WEBHOOK CHANGES section
	_, _ = fmt.Fprintf(w, "WEBHOOK CHANGES\n")
	tw = tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintf(tw, "Component\tWebhook\tChange\n")

	hasWebhooks := false
	for _, c := range d.Components {
		for _, wh := range c.WebhookDiffs {
			hasWebhooks = true
			_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\n", c.Component, wh.Name, wh.ChangeType)
		}
	}

	if !hasWebhooks {
		_, _ = fmt.Fprintf(tw, "(none)\t\t\n")
	}
	_ = tw.Flush()
	_, _ = fmt.Fprintf(w, "\n")

	// DEPENDENCY CHANGES section (if any)
	hasDependencies := false
	for _, c := range d.Components {
		if len(c.DependencyDiffs) > 0 {
			hasDependencies = true
			break
		}
	}

	if hasDependencies {
		_, _ = fmt.Fprintf(w, "DEPENDENCY CHANGES\n")
		tw = tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
		_, _ = fmt.Fprintf(tw, "Component\tDependency\tChange\n")

		for _, c := range d.Components {
			for _, dep := range c.DependencyDiffs {
				_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\n", c.Component, dep.Dependency, dep.ChangeType)
			}
		}
		_ = tw.Flush()
		_, _ = fmt.Fprintf(w, "\n")
	}

	// SUMMARY line
	_, _ = fmt.Fprintf(w, "SUMMARY\n")
	_, _ = fmt.Fprintf(w, "  Namespace moves: %d\n", d.Summary.NamespaceMoves)
	_, _ = fmt.Fprintf(w, "  Renames: %d\n", d.Summary.ComponentsRenamed)
	_, _ = fmt.Fprintf(w, "  Webhook changes: %d\n", d.Summary.WebhookChanges)
	_, _ = fmt.Fprintf(w, "  Dependency changes: %d\n", d.Summary.DependencyChanges)
	_, _ = fmt.Fprintf(w, "\n")

	// BREAKING CHANGES count
	_, _ = fmt.Fprintf(w, "BREAKING CHANGES: %d\n", d.Summary.BreakingChanges)

	return nil
}

func formatCRDDiffReportJSON(w io.Writer, r *CRDDiffReport) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(r)
}

func formatCRDDiffReportYAML(w io.Writer, r *CRDDiffReport) error {
	data, err := yaml.Marshal(r)
	if err != nil {
		return err
	}
	_, err = w.Write(data)
	return err
}

func formatCRDDiffReportTable(w io.Writer, r *CRDDiffReport) error {
	// Header
	_, _ = fmt.Fprintf(w, "CRD Schema Diff\n\n")

	// Tabwriter
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintf(tw, "CRD\tVersion\tChange\tSeverity\tPath\n")

	breakingCount := 0
	warningCount := 0
	infoCount := 0

	for _, crd := range r.CRDs {
		for _, apiVer := range crd.APIVersions {
			for _, sc := range apiVer.SchemaChanges {
				_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n",
					crd.CRDName, apiVer.Version, sc.Type, sc.Severity, sc.Path)

				switch sc.Severity {
				case SeverityBreaking:
					breakingCount++
				case SeverityWarning:
					warningCount++
				case SeverityInfo:
					infoCount++
				}
			}
		}
	}
	_ = tw.Flush()

	// Footer
	_, _ = fmt.Fprintf(w, "\nBREAKING: %d  WARNING: %d  INFO: %d\n",
		breakingCount, warningCount, infoCount)

	return nil
}
