package store

import "encoding/json"

// jsonMarshalErr marshals p to JSON, returning the bytes and any error. Kept
// here so tests can build payloads without panicking on the rare encode
// failure.
func jsonMarshalErr(p any) ([]byte, error) {
	return json.Marshal(p)
}
