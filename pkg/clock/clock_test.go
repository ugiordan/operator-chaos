package clock

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestRealClockReturnsCurrentTime(t *testing.T) {
	c := RealClock{}
	before := time.Now()
	now := c.Now()
	after := time.Now()

	assert.False(t, now.Before(before))
	assert.False(t, now.After(after))
}

func TestFakeClockReturnsFixedTime(t *testing.T) {
	fixed := time.Date(2026, 3, 20, 12, 0, 0, 0, time.UTC)
	c := NewFakeClock(fixed)

	assert.Equal(t, fixed, c.Now())
	assert.Equal(t, fixed, c.Now())
}

func TestFakeClockAdvance(t *testing.T) {
	fixed := time.Date(2026, 3, 20, 12, 0, 0, 0, time.UTC)
	c := NewFakeClock(fixed)

	c.Advance(5 * time.Minute)
	assert.Equal(t, fixed.Add(5*time.Minute), c.Now())
}
