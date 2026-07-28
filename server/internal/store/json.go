package store

import "encoding/json"

// decodeJSON is a tiny wrapper kept local so store.go reads cleanly. It is
// the single place JSON decoding happens in the store layer.
func decodeJSON(s string, v any) error {
	return json.Unmarshal([]byte(s), v)
}
