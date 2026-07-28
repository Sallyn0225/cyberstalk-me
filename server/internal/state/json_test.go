package state

import "encoding/json"

// jsonMarshalImpl marshals v to JSON. Tiny helper so tests don't pull
// encoding/json directly for one call.
func jsonMarshalImpl(v any) ([]byte, error) {
	return json.Marshal(v)
}
