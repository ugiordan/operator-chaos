package injection

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestValidateK8sName_Valid(t *testing.T) {
	validNames := []string{
		"my-resource",
		"foo.bar",
		"a",
		"test-123",
		"my-config-map",
	}

	for _, name := range validNames {
		t.Run(name, func(t *testing.T) {
			err := validateK8sName("name", name)
			assert.NoError(t, err)
		})
	}
}

func TestValidateK8sName_Invalid(t *testing.T) {
	tests := []struct {
		name  string
		value string
	}{
		{"empty", ""},
		{"uppercase mixed", "My-Resource"},
		{"underscore", "foo_bar"},
		{"leading dash", "-leading-dash"},
		{"trailing dash", "trailing-dash-"},
		{"has spaces", "has spaces"},
		{"all uppercase", "UPPERCASE"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateK8sName("name", tt.value)
			assert.Error(t, err)
		})
	}
}

func TestValidateK8sName_TooLong(t *testing.T) {
	longName := strings.Repeat("a", 254)
	err := validateK8sName("name", longName)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "exceeds maximum length")
}

func TestValidateFieldName_Valid(t *testing.T) {
	validFields := []string{
		"replicas",
		"my_field",
		"fieldName",
	}

	for _, field := range validFields {
		t.Run(field, func(t *testing.T) {
			err := validateFieldName("field", field)
			assert.NoError(t, err)
		})
	}
}

func TestValidateFieldName_Invalid(t *testing.T) {
	tests := []struct {
		name  string
		value string
	}{
		{"empty", ""},
		{"starts with digit", "123start"},
		{"leading dot", ".leading-dot"},
		{"has spaces", "has spaces"},
		{"dot path", "spec.replicas"},
		{"nested dots", "a.b.c"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateFieldName("field", tt.value)
			assert.Error(t, err)
		})
	}
}
