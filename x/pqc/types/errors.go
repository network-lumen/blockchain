package types

import errorsmod "cosmossdk.io/errors"

var (
	ErrSchemeUnsupported       = errorsmod.Register(ModuleName, 1, "unsupported pqc scheme")
	ErrAccountAlreadyLinked    = errorsmod.Register(ModuleName, 2, "pqc account already linked")
	ErrAccountRotationDisabled = errorsmod.Register(ModuleName, 3, "account pqc rotation disabled")
	ErrAccountNotFound         = errorsmod.Register(ModuleName, 4, "pqc account not found")
	ErrPQCRequired             = errorsmod.Register(ModuleName, 5, "pqc signature required")
	ErrPQCVerifyFailed         = errorsmod.Register(ModuleName, 6, "pqc signature verification failed")
	ErrInvalidScheme           = errorsmod.Register(ModuleName, 7, "invalid pqc scheme")
	ErrMissingExtension        = errorsmod.Register(ModuleName, 8, "missing pqc signature extension")
)
