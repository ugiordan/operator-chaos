package safety

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestTTLAnnotation(t *testing.T) {
	now := time.Now()
	expiry := TTLExpiry(now, 5*time.Minute)
	assert.False(t, IsExpired(now, expiry))

	pastExpiry := now.Add(-1 * time.Minute).Format(time.RFC3339)
	assert.True(t, IsExpired(now, pastExpiry))
}

func TestTTLAnnotationKey(t *testing.T) {
	assert.Equal(t, "chaos.opendatahub.io/expires", TTLAnnotationKey)
}

func TestTTLMalformedExpiry(t *testing.T) {
	assert.True(t, IsExpired(time.Now(), "not-a-date"))
}
