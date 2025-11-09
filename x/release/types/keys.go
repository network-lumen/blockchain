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
)
