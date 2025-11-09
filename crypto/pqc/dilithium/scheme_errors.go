package dilithium

import "fmt"

// ErrNotImplemented indicates that a build does not include the requested backend.
var ErrNotImplemented = fmt.Errorf("pqc: backend not implemented in this build")
