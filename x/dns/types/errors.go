package types

import (
	"cosmossdk.io/errors"
)

var (
	ErrInvalidSigner = errors.Register(ModuleName, 1101, "expected gov account as only signer for proposal message")
	ErrInvalidFqdn   = errors.Register(ModuleName, 1102, "invalid fqdn")
	ErrDomainExists  = errors.Register(ModuleName, 1103, "domain already exists")
	ErrNotOwner      = errors.Register(ModuleName, 1104, "not domain owner")

	ErrAuctionNotOpen  = errors.Register(ModuleName, 1105, "auction not open")
	ErrInsufficientBid = errors.Register(ModuleName, 1106, "insufficient bid")

	ErrInsufficientFee = errors.Register(ModuleName, 1108, "insufficient fee")

	ErrInvalidRequest = errors.Register(ModuleName, 1109, "invalid request")
)
