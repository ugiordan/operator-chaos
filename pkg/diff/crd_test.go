package diff

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	apiextv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
)

func TestDiffCRDSchemas_FieldRemoved(t *testing.T) {
	source := apiextv1.JSONSchemaProps{
		Type: "object",
		Properties: map[string]apiextv1.JSONSchemaProps{
			"fieldA": {Type: "string"},
			"fieldB": {Type: "integer"},
		},
	}
	target := apiextv1.JSONSchemaProps{
		Type: "object",
		Properties: map[string]apiextv1.JSONSchemaProps{
			"fieldA": {Type: "string"},
		},
	}
	changes := diffSchemas(&source, &target, ".spec")
	require.Len(t, changes, 1)
	assert.Equal(t, FieldRemoved, changes[0].Type)
	assert.Equal(t, ".spec.fieldB", changes[0].Path)
	assert.Equal(t, SeverityBreaking, changes[0].Severity)
}

func TestDiffCRDSchemas_FieldAdded(t *testing.T) {
	source := apiextv1.JSONSchemaProps{
		Type:       "object",
		Properties: map[string]apiextv1.JSONSchemaProps{},
	}
	target := apiextv1.JSONSchemaProps{
		Type: "object",
		Properties: map[string]apiextv1.JSONSchemaProps{
			"newField": {Type: "string"},
		},
	}
	changes := diffSchemas(&source, &target, ".spec")
	require.Len(t, changes, 1)
	assert.Equal(t, FieldAdded, changes[0].Type)
	assert.Equal(t, SeverityInfo, changes[0].Severity)
}

func TestDiffCRDSchemas_TypeChanged(t *testing.T) {
	source := apiextv1.JSONSchemaProps{
		Type: "object",
		Properties: map[string]apiextv1.JSONSchemaProps{
			"replicas": {Type: "string"},
		},
	}
	target := apiextv1.JSONSchemaProps{
		Type: "object",
		Properties: map[string]apiextv1.JSONSchemaProps{
			"replicas": {Type: "integer"},
		},
	}
	changes := diffSchemas(&source, &target, ".spec")
	require.Len(t, changes, 1)
	assert.Equal(t, TypeChanged, changes[0].Type)
	assert.Equal(t, SeverityBreaking, changes[0].Severity)
}

func TestDiffCRDSchemas_EnumValueRemoved(t *testing.T) {
	source := apiextv1.JSONSchemaProps{
		Type: "object",
		Properties: map[string]apiextv1.JSONSchemaProps{
			"mode": {
				Type: "string",
				Enum: []apiextv1.JSON{
					{Raw: []byte(`"ModeA"`)},
					{Raw: []byte(`"ModeB"`)},
					{Raw: []byte(`"ModeC"`)},
				},
			},
		},
	}
	target := apiextv1.JSONSchemaProps{
		Type: "object",
		Properties: map[string]apiextv1.JSONSchemaProps{
			"mode": {
				Type: "string",
				Enum: []apiextv1.JSON{
					{Raw: []byte(`"ModeA"`)},
					{Raw: []byte(`"ModeC"`)},
				},
			},
		},
	}
	changes := diffSchemas(&source, &target, ".spec")
	require.Len(t, changes, 1)
	assert.Equal(t, EnumValueRemoved, changes[0].Type)
	assert.Contains(t, changes[0].Detail, "ModeB")
}

func TestDiffCRDSchemas_RequiredAdded(t *testing.T) {
	source := apiextv1.JSONSchemaProps{
		Type:     "object",
		Required: []string{"name"},
		Properties: map[string]apiextv1.JSONSchemaProps{
			"name":    {Type: "string"},
			"runtime": {Type: "string"},
		},
	}
	target := apiextv1.JSONSchemaProps{
		Type:     "object",
		Required: []string{"name", "runtime"},
		Properties: map[string]apiextv1.JSONSchemaProps{
			"name":    {Type: "string"},
			"runtime": {Type: "string"},
		},
	}
	changes := diffSchemas(&source, &target, ".spec")
	require.Len(t, changes, 1)
	assert.Equal(t, RequiredAdded, changes[0].Type)
	assert.Equal(t, SeverityBreaking, changes[0].Severity)
}

func TestDiffCRDSchemas_DefaultChanged(t *testing.T) {
	source := apiextv1.JSONSchemaProps{
		Type: "object",
		Properties: map[string]apiextv1.JSONSchemaProps{
			"replicas": {Type: "integer", Default: &apiextv1.JSON{Raw: []byte("1")}},
		},
	}
	target := apiextv1.JSONSchemaProps{
		Type: "object",
		Properties: map[string]apiextv1.JSONSchemaProps{
			"replicas": {Type: "integer", Default: &apiextv1.JSON{Raw: []byte("2")}},
		},
	}
	changes := diffSchemas(&source, &target, ".spec")
	require.Len(t, changes, 1)
	assert.Equal(t, DefaultChanged, changes[0].Type)
	assert.Equal(t, SeverityWarning, changes[0].Severity)
}

func TestDiffCRDSchemas_Nested(t *testing.T) {
	source := apiextv1.JSONSchemaProps{
		Type: "object",
		Properties: map[string]apiextv1.JSONSchemaProps{
			"predictor": {
				Type: "object",
				Properties: map[string]apiextv1.JSONSchemaProps{
					"tensorflow": {Type: "object"},
					"model":      {Type: "object"},
				},
			},
		},
	}
	target := apiextv1.JSONSchemaProps{
		Type: "object",
		Properties: map[string]apiextv1.JSONSchemaProps{
			"predictor": {
				Type: "object",
				Properties: map[string]apiextv1.JSONSchemaProps{
					"model": {Type: "object"},
				},
			},
		},
	}
	changes := diffSchemas(&source, &target, ".spec")
	require.Len(t, changes, 1)
	assert.Equal(t, FieldRemoved, changes[0].Type)
	assert.Equal(t, ".spec.predictor.tensorflow", changes[0].Path)
}

func TestCompareCRDFiles(t *testing.T) {
	sourceVersions := []apiextv1.CustomResourceDefinitionVersion{
		{
			Name: "v1beta1", Served: true, Storage: false,
			Schema: &apiextv1.CustomResourceValidation{
				OpenAPIV3Schema: &apiextv1.JSONSchemaProps{Type: "object"},
			},
		},
		{
			Name: "v1", Served: true, Storage: true,
			Schema: &apiextv1.CustomResourceValidation{
				OpenAPIV3Schema: &apiextv1.JSONSchemaProps{
					Type: "object",
					Properties: map[string]apiextv1.JSONSchemaProps{
						"spec": {Type: "object", Properties: map[string]apiextv1.JSONSchemaProps{
							"fieldA": {Type: "string"},
						}},
					},
				},
			},
		},
	}
	targetVersions := []apiextv1.CustomResourceDefinitionVersion{
		{
			Name: "v1", Served: true, Storage: true,
			Schema: &apiextv1.CustomResourceValidation{
				OpenAPIV3Schema: &apiextv1.JSONSchemaProps{
					Type: "object",
					Properties: map[string]apiextv1.JSONSchemaProps{
						"spec": {Type: "object", Properties: map[string]apiextv1.JSONSchemaProps{
							"fieldA": {Type: "string"},
							"fieldB": {Type: "integer"},
						}},
					},
				},
			},
		},
	}

	diff := compareCRDVersions("test.example.io", sourceVersions, targetVersions)
	assert.Equal(t, "test.example.io", diff.CRDName)

	var v1beta1Found bool
	for _, av := range diff.APIVersions {
		if av.Version == "v1beta1" {
			v1beta1Found = true
			assert.Equal(t, DiffRemoved, av.ChangeType)
		}
	}
	assert.True(t, v1beta1Found, "expected v1beta1 removal")
}
