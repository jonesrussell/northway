// Package identity owns authenticated principals, scopes and key lifecycle.
package identity

import (
	"encoding/hex"
	"errors"
	"strings"
)

// TenantID identifies ownership; it is not proof of authentication. Corpus
// operations derive it from a Principal, never a request's tenant field.
type TenantID string

func (id TenantID) Validate() error { return ValidateID(string(id)) }

func ValidateID(id string) error {
	if len(id) != 36 || id[8] != '-' || id[13] != '-' || id[18] != '-' || id[23] != '-' || id != strings.ToLower(id) {
		return errors.New("ID must be a canonical lowercase UUID")
	}
	raw := strings.ReplaceAll(id, "-", "")
	if len(raw) != 32 {
		return errors.New("invalid UUID")
	}
	if _, err := hex.DecodeString(raw); err != nil {
		return errors.New("invalid UUID")
	}
	return nil
}
