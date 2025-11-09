package types

import (
	errorsmod "cosmossdk.io/errors"
)

const (
	errUnauthorized uint32 = 1
)

var (
	ErrUnauthorized = errorsmod.Register(ModuleName, errUnauthorized, "unauthorized")
)
