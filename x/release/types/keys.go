package types

import "cosmossdk.io/collections"

const (
	ModuleName = "release"

	StoreKey = ModuleName

	GovModuleName = "gov"
)

var ParamsKey = collections.NewPrefix("p_release")

var (
	ReleaseKey    = collections.NewPrefix("release/value/")
	ReleaseSeqKey = collections.NewPrefix("release/seq")
	ByVersionKey  = collections.NewPrefix("release/by_version")
	// ByTripleKey indexes releases by channel|platform|kind -> release id.
	// Example composite key: {channel:"beta", platform:"linux-amd64", kind:"daemon"} → releaseID.
	// Used to fetch the unique tuple without scanning every stored release.
	ByTripleKey = collections.NewPrefix("release/by_cpk")

	// Escrow and expiry state for strict release lifecycle hardening.
	EscrowAmountKey    = collections.NewPrefix("release/escrow_amount")
	EscrowPublisherKey = collections.NewPrefix("release/escrow_publisher")
	ExpiryByIDKey      = collections.NewPrefix("release/expiry_by_id")
	ExpiryQueueKey     = collections.NewPrefix("release/expiry_queue")
	StateVersionKey    = collections.NewPrefix("release/state_version")
	ExpiryTTLKey       = collections.NewPrefix("release/expiry_ttl")
)
