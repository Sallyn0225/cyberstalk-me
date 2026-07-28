// Command agent is the Windows reporter client.
//
// This is a placeholder entrypoint so the module builds as part of the
// workspace during the backend task. Collection, sanitization mapping, and
// the report loop are implemented in a later child task
// (07-28-client-windows). See .trellis/tasks/07-28-cyberstalk-me/design.md
// section 5.1 for the intended design.
package main

import "fmt"

func main() {
	fmt.Println("cyberstalk-me windows agent (placeholder — not yet implemented)")
}
