package diff

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	apiextv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"sigs.k8s.io/yaml"
)

// ComputeCRDDiff loads CRD YAML files from two directories and compares them.
func ComputeCRDDiff(sourceDir, targetDir string) (*CRDDiffReport, error) {
	sourceCRDs, err := loadCRDs(sourceDir)
	if err != nil {
		return nil, fmt.Errorf("loading source CRDs from %s: %w", sourceDir, err)
	}
	targetCRDs, err := loadCRDs(targetDir)
	if err != nil {
		return nil, fmt.Errorf("loading target CRDs from %s: %w", targetDir, err)
	}
	report := diffCRDSets(sourceCRDs, targetCRDs)
	return report, nil
}

// loadCRDs reads a directory, unmarshals YAML files, and filters to actual CRDs.
func loadCRDs(dir string) (map[string]*apiextv1.CustomResourceDefinition, error) {
	crds := make(map[string]*apiextv1.CustomResourceDefinition)

	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("reading directory %s: %w", dir, err)
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		ext := filepath.Ext(entry.Name())
		if ext != ".yaml" && ext != ".yml" {
			continue
		}

		data, err := os.ReadFile(filepath.Join(dir, entry.Name()))
		if err != nil {
			return nil, fmt.Errorf("reading file %s: %w", entry.Name(), err)
		}

		var crd apiextv1.CustomResourceDefinition
		if err := yaml.Unmarshal(data, &crd); err != nil {
			continue // skip files that don't unmarshal as CRDs
		}

		if crd.Kind != "CustomResourceDefinition" || crd.Name == "" {
			continue
		}

		crds[crd.Name] = &crd
	}

	return crds, nil
}

// diffCRDSets matches CRDs by name and detects added/removed/modified.
func diffCRDSets(source, target map[string]*apiextv1.CustomResourceDefinition) *CRDDiffReport {
	report := &CRDDiffReport{}

	// Check source CRDs against target
	for name, srcCRD := range source {
		tgtCRD, exists := target[name]
		if !exists {
			report.CRDs = append(report.CRDs, CRDDiff{
				CRDName:    name,
				ChangeType: DiffRemoved,
			})
			continue
		}

		diff := compareCRDVersions(name, srcCRD.Spec.Versions, tgtCRD.Spec.Versions)
		if len(diff.APIVersions) > 0 {
			diff.ChangeType = DiffModified
			report.CRDs = append(report.CRDs, diff)
		}
	}

	// Check for added CRDs
	for name := range target {
		if _, exists := source[name]; !exists {
			report.CRDs = append(report.CRDs, CRDDiff{
				CRDName:    name,
				ChangeType: DiffAdded,
			})
		}
	}

	// Sort for deterministic output
	sort.Slice(report.CRDs, func(i, j int) bool {
		return report.CRDs[i].CRDName < report.CRDs[j].CRDName
	})

	return report
}

// compareCRDVersions compares API versions between source and target CRD versions.
func compareCRDVersions(crdName string, source, target []apiextv1.CustomResourceDefinitionVersion) CRDDiff {
	diff := CRDDiff{CRDName: crdName}

	srcVersions := make(map[string]*apiextv1.CustomResourceDefinitionVersion)
	for i := range source {
		srcVersions[source[i].Name] = &source[i]
	}
	tgtVersions := make(map[string]*apiextv1.CustomResourceDefinitionVersion)
	for i := range target {
		tgtVersions[target[i].Name] = &target[i]
	}

	// Check source versions against target
	for vName, srcVer := range srcVersions {
		tgtVer, exists := tgtVersions[vName]
		if !exists {
			diff.APIVersions = append(diff.APIVersions, APIVersionDiff{
				Version:    vName,
				ChangeType: DiffRemoved,
			})
			continue
		}

		avDiff := APIVersionDiff{
			Version:    vName,
			ChangeType: DiffModified,
		}

		// Check served/storage flag changes
		if srcVer.Served != tgtVer.Served {
			served := tgtVer.Served
			avDiff.ServedChange = &served
		}
		if srcVer.Storage != tgtVer.Storage {
			storage := tgtVer.Storage
			avDiff.StorageChange = &storage
		}

		// Compare schemas
		if srcVer.Schema != nil && tgtVer.Schema != nil &&
			srcVer.Schema.OpenAPIV3Schema != nil && tgtVer.Schema.OpenAPIV3Schema != nil {
			avDiff.SchemaChanges = diffSchemas(srcVer.Schema.OpenAPIV3Schema, tgtVer.Schema.OpenAPIV3Schema, "")
		}

		if len(avDiff.SchemaChanges) > 0 || avDiff.ServedChange != nil || avDiff.StorageChange != nil {
			diff.APIVersions = append(diff.APIVersions, avDiff)
		}
	}

	// Check for added versions
	for vName := range tgtVersions {
		if _, exists := srcVersions[vName]; !exists {
			diff.APIVersions = append(diff.APIVersions, APIVersionDiff{
				Version:    vName,
				ChangeType: DiffAdded,
			})
		}
	}

	// Sort for deterministic output
	sort.Slice(diff.APIVersions, func(i, j int) bool {
		return diff.APIVersions[i].Version < diff.APIVersions[j].Version
	})

	return diff
}

// diffSchemas recursively walks two OpenAPI v3 schemas and reports changes.
func diffSchemas(source, target *apiextv1.JSONSchemaProps, path string) []SchemaChange {
	var changes []SchemaChange

	if source == nil || target == nil {
		return changes
	}

	// Check properties in source
	for name, srcProp := range source.Properties {
		fieldPath := path + "." + name
		tgtProp, exists := target.Properties[name]
		if !exists {
			changes = append(changes, SchemaChange{
				Path:     fieldPath,
				Type:     FieldRemoved,
				Severity: SeverityBreaking,
				Detail:   fmt.Sprintf("field %s was removed", name),
			})
			continue
		}

		// Type changed
		if srcProp.Type != tgtProp.Type {
			changes = append(changes, SchemaChange{
				Path:     fieldPath,
				Type:     TypeChanged,
				Severity: SeverityBreaking,
				Detail:   fmt.Sprintf("type changed from %s to %s", srcProp.Type, tgtProp.Type),
			})
			continue
		}

		// Enum changes
		changes = append(changes, diffEnums(srcProp.Enum, tgtProp.Enum, fieldPath)...)

		// Default changes
		changes = append(changes, diffDefaults(srcProp.Default, tgtProp.Default, fieldPath)...)

		// Recurse for nested objects
		if srcProp.Type == "object" && len(srcProp.Properties) > 0 || len(tgtProp.Properties) > 0 {
			changes = append(changes, diffSchemas(&srcProp, &tgtProp, fieldPath)...)
		}

		// Recurse for array items
		if srcProp.Items != nil && tgtProp.Items != nil &&
			srcProp.Items.Schema != nil && tgtProp.Items.Schema != nil {
			changes = append(changes, diffSchemas(srcProp.Items.Schema, tgtProp.Items.Schema, fieldPath+"[*]")...)
		}
	}

	// Check for new properties in target
	for name := range target.Properties {
		if _, exists := source.Properties[name]; !exists {
			fieldPath := path + "." + name
			severity := SeverityInfo
			changeType := FieldAdded

			if isRequired(name, target.Required) && !isRequired(name, source.Required) {
				severity = SeverityBreaking
				changeType = RequiredAdded
			}

			changes = append(changes, SchemaChange{
				Path:     fieldPath,
				Type:     changeType,
				Severity: severity,
				Detail:   fmt.Sprintf("field %s was added", name),
			})
		}
	}

	// Check required changes for existing fields
	for _, req := range target.Required {
		if !isRequired(req, source.Required) {
			// Only report if the field already existed (new required fields on new properties are handled above)
			if _, existsInSource := source.Properties[req]; existsInSource {
				changes = append(changes, SchemaChange{
					Path:     path + "." + req,
					Type:     RequiredAdded,
					Severity: SeverityBreaking,
					Detail:   fmt.Sprintf("field %s is now required", req),
				})
			}
		}
	}

	// Sort for deterministic output
	sort.Slice(changes, func(i, j int) bool {
		if changes[i].Path != changes[j].Path {
			return changes[i].Path < changes[j].Path
		}
		return changes[i].Type < changes[j].Type
	})

	return changes
}

// diffEnums compares enum values between source and target schemas.
func diffEnums(source, target []apiextv1.JSON, path string) []SchemaChange {
	var changes []SchemaChange

	if len(source) == 0 && len(target) == 0 {
		return changes
	}

	srcSet := enumSet(source)
	tgtSet := enumSet(target)

	for val := range srcSet {
		if !tgtSet[val] {
			changes = append(changes, SchemaChange{
				Path:     path,
				Type:     EnumValueRemoved,
				Severity: SeverityBreaking,
				Detail:   fmt.Sprintf("enum value %s was removed", val),
			})
		}
	}

	for val := range tgtSet {
		if !srcSet[val] {
			changes = append(changes, SchemaChange{
				Path:     path,
				Type:     EnumValueAdded,
				Severity: SeverityInfo,
				Detail:   fmt.Sprintf("enum value %s was added", val),
			})
		}
	}

	return changes
}

// diffDefaults compares default values between source and target schemas.
func diffDefaults(source, target *apiextv1.JSON, path string) []SchemaChange {
	var changes []SchemaChange

	if source == nil && target == nil {
		return changes
	}

	if source == nil || target == nil || !bytes.Equal(source.Raw, target.Raw) {
		changes = append(changes, SchemaChange{
			Path:     path,
			Type:     DefaultChanged,
			Severity: SeverityWarning,
			Detail:   "default value changed",
		})
	}

	return changes
}

// enumSet converts a slice of JSON enum values to a set of strings, stripping quotes.
func enumSet(enums []apiextv1.JSON) map[string]bool {
	set := make(map[string]bool, len(enums))
	for _, e := range enums {
		val := strings.Trim(string(e.Raw), `"`)
		set[val] = true
	}
	return set
}

// isRequired checks if a field name is in the required list.
func isRequired(name string, required []string) bool {
	for _, r := range required {
		if r == name {
			return true
		}
	}
	return false
}
