package types

const (
	// GatewayMetadataMaxLen bounds free-form operator metadata blobs so they
	// cannot be abused to stuff megabytes of JSON in a tx memo.
	GatewayMetadataMaxLen = 1024
	// GatewayEndpointMaxLen caps future endpoint strings; endpoints are human
	// readable hostnames so 64 chars covers typical subdomains.
	GatewayEndpointMaxLen = 64
	// ContractMetadataMaxLen clamps per-contract metadata (client supplied)
	// to 1 KiB to avoid bloating events and state.
	ContractMetadataMaxLen = 1024
)
