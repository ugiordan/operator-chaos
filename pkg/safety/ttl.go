package safety

import "time"

// TTLAnnotationKey is the annotation key used to store the expiration timestamp on chaos-injected resources.
const TTLAnnotationKey = "chaos.opendatahub.io/expires"

// TTLExpiry returns the TTL annotation value for the given duration from the provided time.
func TTLExpiry(now time.Time, d time.Duration) string {
	return now.Add(d).Format(time.RFC3339)
}

// IsExpired returns true if the TTL annotation value has expired relative to the provided time.
func IsExpired(now time.Time, annotation string) bool {
	t, err := time.Parse(time.RFC3339, annotation)
	if err != nil {
		return true // malformed = treat as expired
	}
	return now.After(t)
}
