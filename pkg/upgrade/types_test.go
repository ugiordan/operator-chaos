package upgrade

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"sigs.k8s.io/yaml"
)

func TestRollbackConfigDefaults(t *testing.T) {
	yamlData := `
enabled: true
snapshotCRDs: true
`
	var config RollbackConfig
	err := yaml.Unmarshal([]byte(yamlData), &config)
	require.NoError(t, err)

	assert.True(t, config.Enabled)
	assert.True(t, config.SnapshotCRDs)
	assert.Equal(t, time.Duration(0), config.MaxRollbackWait.Duration)
}

func TestRollbackConfigWithDuration(t *testing.T) {
	yamlData := `
enabled: true
snapshotCRDs: false
maxRollbackWait: 10m
`
	var config RollbackConfig
	err := yaml.Unmarshal([]byte(yamlData), &config)
	require.NoError(t, err)

	assert.True(t, config.Enabled)
	assert.False(t, config.SnapshotCRDs)
	assert.Equal(t, 10*time.Minute, config.MaxRollbackWait.Duration)
}

func TestRollbackConfigDisabled(t *testing.T) {
	yamlData := `
enabled: false
snapshotCRDs: false
maxRollbackWait: 5m
`
	var config RollbackConfig
	err := yaml.Unmarshal([]byte(yamlData), &config)
	require.NoError(t, err)

	assert.False(t, config.Enabled)
	assert.False(t, config.SnapshotCRDs)
	assert.Equal(t, 5*time.Minute, config.MaxRollbackWait.Duration)
}

func TestDurationMarshalYAML(t *testing.T) {
	tests := []struct {
		name     string
		duration Duration
		want     string
	}{
		{
			name:     "zero duration",
			duration: Duration{Duration: 0},
			want:     `""` + "\n",
		},
		{
			name:     "10 minutes",
			duration: Duration{Duration: 10 * time.Minute},
			want:     "10m0s\n",
		},
		{
			name:     "1 hour",
			duration: Duration{Duration: 1 * time.Hour},
			want:     "1h0m0s\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := yaml.Marshal(tt.duration)
			require.NoError(t, err)
			assert.Equal(t, tt.want, string(data))
		})
	}
}

func TestDurationUnmarshalYAML(t *testing.T) {
	tests := []struct {
		name    string
		yaml    string
		want    time.Duration
		wantErr bool
	}{
		{
			name: "empty string",
			yaml: `""`,
			want: 0,
		},
		{
			name: "10 minutes",
			yaml: `"10m"`,
			want: 10 * time.Minute,
		},
		{
			name: "1 hour 30 minutes",
			yaml: `"1h30m"`,
			want: 90 * time.Minute,
		},
		{
			name:    "invalid duration",
			yaml:    `"invalid"`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var d Duration
			err := yaml.Unmarshal([]byte(tt.yaml), &d)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, d.Duration)
		})
	}
}

func TestDurationMarshalJSON(t *testing.T) {
	tests := []struct {
		name     string
		duration Duration
		want     string
	}{
		{
			name:     "zero duration",
			duration: Duration{Duration: 0},
			want:     `""`,
		},
		{
			name:     "10 minutes",
			duration: Duration{Duration: 10 * time.Minute},
			want:     `"10m0s"`,
		},
		{
			name:     "1 hour",
			duration: Duration{Duration: 1 * time.Hour},
			want:     `"1h0m0s"`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := tt.duration.MarshalJSON()
			require.NoError(t, err)
			assert.Equal(t, tt.want, string(data))
		})
	}
}

func TestDurationUnmarshalJSON(t *testing.T) {
	tests := []struct {
		name    string
		json    string
		want    time.Duration
		wantErr bool
	}{
		{
			name: "empty string",
			json: `""`,
			want: 0,
		},
		{
			name: "10 minutes",
			json: `"10m"`,
			want: 10 * time.Minute,
		},
		{
			name: "1 hour 30 minutes",
			json: `"1h30m"`,
			want: 90 * time.Minute,
		},
		{
			name:    "invalid duration",
			json:    `"invalid"`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var d Duration
			err := d.UnmarshalJSON([]byte(tt.json))
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, d.Duration)
		})
	}
}
