// Package usage defines integer-only, operator-controlled spending limits.
package usage

import "errors"

var ErrLimit = errors.New("spend limit would be exceeded")

// Budget is a cumulative USD-micro cap, not a billing balance. There is no
// implicit daily reset: outstanding holds survive restarts and limit changes.
type Budget struct{ LimitMicros, SpentMicros, HeldMicros int64 }
