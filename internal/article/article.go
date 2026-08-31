package article

import "time"

type Article struct {
	ID, SourceID, OriginID, URL, Title, Body, ContentHash string
	PublishedAt                                           *time.Time
	ObservedAt                                            time.Time
}
