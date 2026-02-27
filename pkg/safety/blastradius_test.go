package safety

import (
	"testing"

	v1alpha1 "github.com/opendatahub-io/odh-platform-chaos/api/v1alpha1"
	"github.com/stretchr/testify/assert"
)

func TestValidateBlastRadius(t *testing.T) {
	tests := []struct {
		name    string
		spec    v1alpha1.BlastRadiusSpec
		target  string
		wantErr bool
	}{
		{
			name: "valid blast radius",
			spec: v1alpha1.BlastRadiusSpec{
				MaxPodsAffected:   1,
				AllowedNamespaces: []string{"opendatahub"},
			},
			target:  "opendatahub",
			wantErr: false,
		},
		{
			name: "namespace not allowed",
			spec: v1alpha1.BlastRadiusSpec{
				MaxPodsAffected:   1,
				AllowedNamespaces: []string{"opendatahub"},
			},
			target:  "kube-system",
			wantErr: true,
		},
		{
			name: "zero pods allowed",
			spec: v1alpha1.BlastRadiusSpec{
				MaxPodsAffected:   0,
				AllowedNamespaces: []string{"opendatahub"},
			},
			target:  "opendatahub",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateBlastRadius(tt.spec, tt.target, "Deployment/test", 1)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidateBlastRadiusForbiddenResources(t *testing.T) {
	tests := []struct {
		name     string
		spec     v1alpha1.BlastRadiusSpec
		target   string
		resource string
		wantErr  bool
		errMsg   string
	}{
		{
			name: "resource in forbidden list",
			spec: v1alpha1.BlastRadiusSpec{
				MaxPodsAffected:    1,
				AllowedNamespaces:  []string{"opendatahub"},
				ForbiddenResources: []string{"Deployment/critical-app", "StatefulSet/etcd"},
			},
			target:   "opendatahub",
			resource: "Deployment/critical-app",
			wantErr:  true,
			errMsg:   "forbidden",
		},
		{
			name: "resource NOT in forbidden list",
			spec: v1alpha1.BlastRadiusSpec{
				MaxPodsAffected:    1,
				AllowedNamespaces:  []string{"opendatahub"},
				ForbiddenResources: []string{"Deployment/critical-app"},
			},
			target:   "opendatahub",
			resource: "Deployment/safe-app",
			wantErr:  false,
		},
		{
			name: "empty forbidden list",
			spec: v1alpha1.BlastRadiusSpec{
				MaxPodsAffected:   1,
				AllowedNamespaces: []string{"opendatahub"},
			},
			target:   "opendatahub",
			resource: "Deployment/any-app",
			wantErr:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateBlastRadius(tt.spec, tt.target, tt.resource, 1)
			if tt.wantErr {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestCheckDangerLevel(t *testing.T) {
	err := CheckDangerLevel(v1alpha1.DangerLevelHigh, false)
	assert.Error(t, err)

	err = CheckDangerLevel(v1alpha1.DangerLevelHigh, true)
	assert.NoError(t, err)

	err = CheckDangerLevel("", false)
	assert.NoError(t, err)
}
