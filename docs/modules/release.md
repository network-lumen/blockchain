# Module `x/release`

## Overview

`x/release` stores authoritative metadata for downloadable artifacts (binaries, installers, container images, etc.).
Artifacts remain off-chain (HTTP/IPFS), but hashes, sizes, channels, mirrors, and validation status are committed to the
chain so clients can verify downloads. Publishers are allowlisted via params and governance controls the emergency flag.

## Messages (`lumend tx release …`)

- `publish --msg <json>` – Create a new release (semver version, channel, notes, artifacts). Requires membership in
  `allowed_publishers` and optionally escrows `publish_fee_ulmn`.
- `mirror --msg <json>` – Append additional URLs to an existing artifact (same publisher set).
- `yank --msg <json>` – Mark a release as yanked (metadata retained, but clients should avoid fetching it).
- `validate-release [id] --decision approve/reject --notes …` – Authority-only; marks a release as validated so it may
  be used in channels that require validation.
- `reject-release [id] --reason …` – Authority-only; rejects a pending release (optionally refunding fees according to
  `reject_refund_bps`).
- `set-emergency [on|off]` – Authority-only; toggles the emergency switch used to pause publishing flows.
- `update-params --authority …` – Governance updates the parameter set below.

All transactions accept AutoCLI JSON payloads; see `make e2e-release` (wraps `devtools/tests/e2e_release.sh`) for end-to-end examples.

## Queries

```sh
API=http://127.0.0.1:2327

curl -s "$API/lumen/release/params" | jq
curl -s "$API/lumen/release/releases?page=1&limit=10" | jq
curl -s "$API/lumen/release/latest?channel=stable&platform=linux-amd64&kind=daemon" | jq
curl -s "$API/lumen/release/by_version/1.0.0" | jq
curl -s "$API/lumen/release/1" | jq
```

REST endpoints mirror the gRPC service:

- `GET /lumen/release/params`
- `GET /lumen/release/releases`
- `GET /lumen/release/latest?channel=&platform=&kind=`
- `GET /lumen/release/by_version/{semver}`
- `GET /lumen/release/{id}`

## Parameters

- `allowed_publishers`, `dao_publishers`
- `channels` (whitelisted channel names)
- `max_artifacts`, `max_urls_per_art`, `max_sigs_per_art`, `max_notes_len`
- `publish_fee_ulmn`, `max_pending_ttl`
- `reject_refund_bps` (basis points refunded on rejection)
- `require_validation_for_stable` (or any other channel that governance designates)

Governance adjusts these via `MsgUpdateParams`.

## Operational Notes

- Versions must follow semantic versioning and are immutable once published.
- Artifact URLs accept `http(s)` and `ipfs://`. Each entry records `{platform, kind, size, sha256_hex, urls[], signatures[]}`.
- Only DAO/authority addresses may validate, reject, or toggle emergency mode.
- When `require_validation_for_stable=true`, clients must wait for `MsgValidateRelease` before serving the release on the
  `stable` channel.
- Yanking does not delete history; clients can choose whether to honor yanked releases.
