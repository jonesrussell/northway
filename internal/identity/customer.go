package identity

import "time"

type KeyMetadata struct {
	ID         string     `json:"id"`
	Scopes     string     `json:"scopes"`
	CreatedAt  time.Time  `json:"created_at"`
	ExpiresAt  time.Time  `json:"expires_at"`
	LastUsedAt *time.Time `json:"last_used_at"`
	RevokedAt  *time.Time `json:"revoked_at"`
}
type FeedMetadata struct {
	ID    string `json:"id"`
	Title string `json:"title"`
}

func (s Scopes) Names() string {
	switch s {
	case FeedsRead:
		return "feeds:read"
	case FeedbackWrite:
		return "feedback:write"
	case FeedsRead | FeedbackWrite:
		return "feeds:read,feedback:write"
	}
	return ""
}
