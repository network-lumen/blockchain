package app

import (
	errorsmod "cosmossdk.io/errors"
)

const (
	errSendAmountTooSmall uint32 = 1
)

var (
	ErrSendAmountTooSmall = errorsmod.Register(Name, errSendAmountTooSmall, "amount too small to cover transfer tax")
)
