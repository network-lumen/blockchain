package types

import errorsmod "cosmossdk.io/errors"

var (
	ErrInvalidRequest    = errorsmod.Register(ModuleName, 1, "invalid request")
	ErrUnauthorized      = errorsmod.Register(ModuleName, 2, "unauthorized")
	ErrNotFound          = errorsmod.Register(ModuleName, 3, "not found")
	ErrOverflow          = errorsmod.Register(ModuleName, 4, "overflow")
	ErrInsufficientFunds = errorsmod.Register(ModuleName, 5, "insufficient funds")
	ErrOutOfBounds       = errorsmod.Register(ModuleName, 6, "out of bounds")
)
