package postgres

import "crypto/subtle"

func equalOpaqueCredentialID(left, right []byte) bool {
	return len(left) == len(right) && subtle.ConstantTimeCompare(left, right) == 1
}
