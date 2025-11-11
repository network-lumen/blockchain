# PQC Registry

## Overview

The `x/pqc` module exposes a post-quantum (PQ) account registry so that user accounts can publish Dilithium public
keys alongside their legacy Ed25519 keys. Every transaction signed by an externally owned account (EOA) **must** include
a Dilithium signature; there is no whitelist or exemption mechanism. Module accounts never sign transactions, so they
are unaffected. The registry feeds the dual-sign ante decorator and the policy is always enforced as REQUIRED (governance
parameters remain for backwards compatibility but are clamped in-memory).

## Account Registry

- Each account may register a single Dilithium key via `MsgLinkAccountPQC`.
- The stored record contains the scheme identifier (currently `dilithium3`), the SHA-256 hash of the public key, and the
  timestamp (`added_at`). Full keys never live on-chain; they are provided alongside signatures and
  hashed/verified on demand.
- Rotation is disabled by default to avoid silent key churn; governance can flip the `allow_account_rotate` parameter.
- Linking is gated by two guards:
  1. `min_balance_for_link` – required spendable ULUMEN balance before broadcasting `MsgLinkAccountPQC`.
  2. `pow_difficulty_bits` – the transaction must include a nonce such that `sha256(pubkey || nonce)` has at least this
     many leading zero bits.

Events emitted on successful linkage include the account address, scheme, and SHA-256 hash of the public key so that
operators can index registry updates without storing full keys on-chain.

### CLI

```bash
# Link the Dilithium3 key for the --from account (min-balance + PoW enforced automatically)
lumen tx pqc link-account --scheme dilithium3 --pubkey <hex|base64>

# Query the registered key for an address
lumen q pqc account <bech32-address>

# Inspect module parameters
lumen q pqc params
```

The CLI validates that the provided scheme matches the active backend (`crypto/pqc/dilithium.Default()`), refuses to
link if the account balance is below `min_balance_for_link`, and mines an appropriate nonce to satisfy
`pow_difficulty_bits` before broadcasting.

## Module Parameters

| Parameter | Default | Description |
|-----------|---------|-------------|
| `policy` | `REQUIRED` | Runtime policy is forced to REQUIRED; other enum values are ignored and rejected in SetParams. |
| `min_scheme` | `dilithium3` | Minimum scheme identifier accepted for incoming signatures. |
| `allow_account_rotate` | `false` | Allows accounts to overwrite an existing PQC key when set to `true`. |
| `min_balance_for_link` | `1000ulmn` | Accounts must hold at least this spendable balance before linking. |
| `pow_difficulty_bits` | `21` | Required number of leading zero bits for `sha256(pubkey || nonce)` during link. |

Parameter updates are validated against the list of supported schemes exposed by the active backend. The PoW guard uses
big-endian encodings of the nonce bytes; clients increment the nonce until the digest carries enough leading zeros.

## Dual-Sign Ante Enforcement

The `PQCDualSignDecorator` runs immediately after Ed25519 signature verification. There is no whitelist—if a transaction
contains any EOA signatures, all of them must pass the PQC checks. For each signer in the transaction:

1. The account’s registered PQC key is fetched from the `x/pqc` keeper.
2. The decorator attempts to locate a matching PQC signature in the transaction’s `TxBody.extension_options`.  
   The payload must be encoded as a `lumen.pqc.v1.PQCSignatures` message where each entry contains:

   ```proto
   message PQCSignatureEntry {
     string addr      = 1; // bech32 signer address
     string scheme    = 2; // e.g. "dilithium3"
     bytes signature  = 3; // Dilithium signature over the PQC payload
     bytes pub_key    = 4; // Raw Dilithium public key (hash checked against registry)
   }
   ```

3. The signature is verified against the Dilithium backend (`crypto/pqc/dilithium.Default()`).

### Sign Bytes Format

The PQC payload reuses the standard Cosmos SDK `SignDoc`, but with a domain separator:

```
PQCv1: || SignDoc{
  body_bytes:     sanitized_body_bytes, // TxBody with any PQC signature extensions stripped
  auth_info_bytes: tx_raw.auth_info_bytes,
  chain_id:        ctx.ChainID(),
  account_number:  signer_account_number,
}
```

Transactions should first be assembled without the PQC signature, the payload computed as above, and only then
should the final signature be inserted into the `PQCSignatures` extension.

### Enforcement Outcomes

Runtime policy is always REQUIRED:

| Situation | Behaviour |
|-----------|-----------|
| Missing key | Reject (`ErrPQCRequired`) |
| Missing signature | Reject (`ErrMissingExtension` / `ErrPQCRequired`) |
| Invalid signature | Reject (`ErrPQCVerifyFailed`) |

Successful verifications emit a `pqc.verified` event containing the signer address and scheme.

## Client Setup

A lightweight keystore stores Dilithium testing keys under `~/.lumen/pqc_keys`. The keystore is intentionally
plaintext (for development only) and the associated CLI commands are compiled only when the binary is built with the
`dev` or `test` tags. In those builds, the following helpers are available:

```bash
# Import a Dilithium keypair (private key provided as hex/base64)
lumend keys pqc-import --name local-dilithium --scheme dilithium3 \
  --pubkey <hex-or-base64> --privkey <hex-or-base64>

# Show the imported keys and local bindings
lumend keys pqc-list

# Display a single key (prints the fingerprint and raw bytes)
lumend keys pqc-show local-dilithium

# Link a cosmos keyring entry to the imported PQC key
lumend keys pqc-link --from validator --pqc local-dilithium
```

During transaction assembly, the CLI signs Ed25519 first and then augments the tx with PQC signatures. PQC signing is
enabled by default and controlled globally via the `tx` command’s persistent flags:

| Flag | Default | Purpose |
|------|---------|---------|
| `--pqc-enable` | `true` | Toggle PQC signing for the current transaction. |
| `--pqc-scheme` | `dilithium3` | Select the target scheme (must match on-chain params). |
| `--pqc-from` + `--pqc-key` | `[]` | Optional override mapping of signer addresses to local PQC key names. |

Example bank transfer (the PQC signature is embedded automatically; no manual extension management is required):

```bash
lumend tx bank send alice bob 10ulmn \
  --pqc-enable \
  --pqc-key local-dilithium
```

The signer-specific overrides are useful for multi-signer flows; otherwise the CLI falls back to the local binding
established via `keys pqc-link`. When `--pqc-enable=false`, the CLI omits the PQC extension entirely.

### End-to-end PQC test

```bash
make build
HOME=$(mktemp -d) make e2e

# or run the single flow
make e2e-pqc
```

This script generates a Dilithium3 key via CIRCL, imports it into the local PQC keystore, links the key on-chain,
sends a PQC-signed bank transaction (expected to pass), then retries the same transfer with `--pqc-enable=false`
which should fail when `PqcPolicy=REQUIRED`. Disable the client-side injector explicitly only when you need to
exercise failure paths or when the chain policy is permissive.

## Production Backends

Two production-grade backends are available under `crypto/pqc/dilithium`:

- **Circl (default)** – pure Go implementation provided by `github.com/cloudflare/circl/sign/dilithium`.  
- **PQClean fallback** – builds on the AES-enhanced Dilithium3 variant (compiled when `-tags pqc_oqs` is supplied).  

`Default()` tries Circl first; if that backend is unavailable it falls back to the PQClean path when `pqc_oqs`
is enabled, and otherwise panics. CI workflows enforce that `pqc_testonly`/noop symbols never appear in release
binaries, and `cmd/lumend` panics during `init()` if an unexpected backend name is detected.

## End-to-end Tests

All E2E scripts are PQC-enabled by default. Genesis and gentx phases temporarily export `LUMEN_PQC_DISABLE=1`, but the
runtime decorator is always active once the node starts (and transactions that pass `--pqc-enable=false` fail with an
explicit PQC signature error).
