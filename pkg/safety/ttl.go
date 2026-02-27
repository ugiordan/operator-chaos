package safety

import "time"

const TTLAnnotationKey = "chaos.opendatahub.io/expires"

func TTLExpiry(ttl time.Duration) string {
	return time.Now().Add(ttl).Format(time.RFC3339)
}

func IsExpired(expiryStr string) bool {
	expiry, err := time.Parse(time.RFC3339, expiryStr)
	if err != nil {
		return true // malformed = treat as expired
	}
	return time.Now().After(expiry)
}
