package store

import (
	// Register the modernc.org/sqlite driver for tests. main.go does the
	// same; tests open databases directly via database/sql so the driver
	// must be imported in the test binary too.
	_ "modernc.org/sqlite"
)
