package types

import (
	"cosmossdk.io/errors"
)

var (
	ErrInvalidSigner         = errors.Register(ModuleName, 1100, "expected gov account as only signer for proposal message")
	ErrUnauthorizedPublisher = errors.Register(ModuleName, 1101, "unauthorized publisher")
	ErrInvalidRequest        = errors.Register(ModuleName, 1102, "invalid request")
	ErrNotPending            = errors.Register(ModuleName, 1103, "release not in pending state")
	ErrNotAuthorized         = errors.Register(ModuleName, 1104, "not authorized")
)
