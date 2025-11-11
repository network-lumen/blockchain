# Release Process

Lumen uses a conventional Git-based release flow augmented by on-chain metadata in `x/release`. This document outlines how to cut a new binary release and how that metadata is managed.

## Checklist Before Tagging

Run the scripted checks instead of reimplementing them manually:

1. `make pre-release`  
   Runs `go mod tidy`, `go vet`, `go test ./...`, `make preflight`, lint/staticcheck, govulncheck, and builds `./build/lumend`
   with the PQC guard. Append `ARGS="--fast"` to skip the clean-tree check or `ARGS="--no-vuln"` to skip the vuln scan.
2. `HOME=$(mktemp -d) make e2e`  
   Executes the unit suite plus every E2E flow (DNS, Gateways, Release, PQC). This mirrors the CI pipeline and must pass
   before tagging.
3. (Recommended) `make simulate-network ARGS="--fast --clean"`  
   Boots the Docker simulator, links PQC keys, exercises DNS/Gateways/Release flows end-to-end, and exports logs/artifacts.
4. Update [`CHANGELOG.md`](../CHANGELOG.md) and any relevant operator docs.

If any of the scripts above fail, fix the issue and rerun the same command until it completes cleanly.

## Cutting a Release

```bash
git checkout main
git pull
git tag vX.Y.Z
git push origin vX.Y.Z

# Produce artifacts locally if needed
make build-release
ls dist/vX.Y.Z
```

Verify the generated binaries against `dist/vX.Y.Z/SHA256SUMS`, then publish release notes (GitHub or other distribution channel) summarising changes, new parameters, and migration steps for operators.

## On-Chain Release Metadata (`x/release`)

Publishing metadata is optional but recommended so clients can verify binaries:

1. Verify your operator address is listed in `Params.allowed_publishers`.
2. Construct a `MsgPublishRelease` containing:
   - `version` (semver)
   - `channel` (e.g. `stable`, `beta`)
   - `artifacts[]` (platform, kind, size, sha256, urls[], signatures[])
   - Optional release notes (bounded by `max_notes_len`)
3. Broadcast the message (AutoCLI: `lumend tx release publish-release ...`).
4. Add extra mirrors with `MsgMirrorRelease`; yank with `MsgYankRelease`.

### Supported artifact URLs

Each artifact `urls[]` entry must be one of:

- `https://…` or `http://…` (host must be present)
- `ipfs://<CID>` (base32/58 alphanumeric CID)
- `lumen://ipfs/<CID>` (shortcut that resolves through the Lumen gateway)
- `lumen://ipns/<name>` (`[-._a-z0-9]{1,255}`)
- `lumen://domain.ext` where `domain.ext` obeys the same validation rules as `x/dns`

URLs longer than 1024 bytes are rejected up front so that clients can safely cache them.

Governance/DAO workflows:

- `MsgValidateRelease` and `MsgRejectRelease` can only be called by the release module authority (typically governance). They update the release status to `VALIDATED` or `REJECTED` respectively and are idempotent.
- `MsgSetEmergency` is also authority-only and toggles the per-release `emergency_ok` flag. When enabling the flag you may optionally supply a TTL (seconds) to record until when the release remains approved for emergency deployment.
- Parameters such as `publish_fee_ulmn`, `max_pending_ttl`, `reject_refund_bps`, and `require_validation_for_stable` control the lifecycle.

Use the REST endpoints (see [`docs/modules/release.md`](modules/release.md)) to confirm metadata after publishing.

## Post-Release

- Announce the new tag and artifact hashes to validators/operators.
- Monitor `FeeCollector` and module accounts for expected tax/fee flows.
- Track governance proposals that adjust parameters impacting the release (e.g., channel policy changes).
