package safety

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestTTLAnnotation(t *testing.T) {
	expiry := TTLExpiry(5 * time.Minute)
	assert.False(t, IsExpired(expiry))

	pastExpiry := time.Now().Add(-1 * time.Minute).Format(time.RFC3339)
	assert.True(t, IsExpired(pastExpiry))
}

func TestTTLAnnotationKey(t *testing.T) {
	assert.Equal(t, "chaos.opendatahub.io/expires", TTLAnnotationKey)
}

func TestTTLMalformedExpiry(t *testing.T) {
	assert.True(t, IsExpired("not-a-date"))
}
