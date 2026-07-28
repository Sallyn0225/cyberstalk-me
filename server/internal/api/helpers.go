package api

import "strings"

// strings_isspace reports whether s is empty or only whitespace. Kept local
// to the api package because the handlers are its only callers.
func strings_isspace(s string) bool {
	return strings.TrimSpace(s) == ""
}
