package handler

import (
	"encoding/json"
	"errors"
	"io"
)

var errInvalidEmailPasswordRequest = errors.New("identity email/password request is invalid")

// decodeEmailPasswordRequestJSON requires an "email" field. It also requires
// a "password" field when requirePassword is true and rejects it otherwise.
// The decoder rejects duplicate or unknown keys, non-string values, nested
// JSON and multiple top-level documents.
func decodeEmailPasswordRequestJSON(body io.Reader, requirePassword bool) (string, string, error) {
	dec := json.NewDecoder(body)
	first, err := dec.Token()
	if err != nil || first != json.Delim('{') {
		return "", "", errInvalidEmailPasswordRequest
	}
	seen := make(map[string]bool, 2)
	var email, password string
	for dec.More() {
		tok, err := dec.Token()
		if err != nil {
			return "", "", errInvalidEmailPasswordRequest
		}
		name, ok := tok.(string)
		if !ok || seen[name] || (name != "email" && (!requirePassword || name != "password")) {
			return "", "", errInvalidEmailPasswordRequest
		}
		seen[name] = true
		var value string
		if err := dec.Decode(&value); err != nil {
			return "", "", errInvalidEmailPasswordRequest
		}
		switch name {
		case "email":
			if len(value) == 0 || len(value) > 254 {
				return "", "", errInvalidEmailPasswordRequest
			}
			email = value
		case "password":
			if len(value) == 0 || len(value) > 512 {
				return "", "", errInvalidEmailPasswordRequest
			}
			password = value
		}
	}
	end, err := dec.Token()
	if err != nil || end != json.Delim('}') || !seen["email"] ||
		(requirePassword && !seen["password"]) {
		return "", "", errInvalidEmailPasswordRequest
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		return "", "", errInvalidEmailPasswordRequest
	}
	return email, password, nil
}
