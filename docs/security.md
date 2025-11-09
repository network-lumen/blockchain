# Security Checklist

- **Gasless ante:** the custom rate-limit decorator relies on:
  - `LUMEN_RL_PER_BLOCK` (default 5)
  - `LUMEN_RL_PER_WINDOW` (default 20)
  - `LUMEN_RL_WINDOW_SEC` (default 10)

  The binary refuses to start unless `--minimum-gas-prices` is unset or exactly `0ulmn`, guaranteeing that gasless transactions remain valid while ante decorators enforce quotas. Any non-zero fee is rejected at ante-time.

- **Operator guard:** Startup aborts when `--minimum-gas-prices` is non-zero; this prevents accidental deployment of fee-bearing configurations.

- **DNS payload bounds:** `MsgRegister`/`MsgUpdate` enforce ≤64 records and ≤16 KiB combined payload. Oversized requests are rejected early.

- **Auctions & fees:** all DNS and gateways fees route through the fee collector and treasury module accounts; monitor balances to detect anomalies.

- **Release spam:** only addresses listed in `Params.allowed_publishers` (or `dao_publishers`) may publish or mirror releases. Tune `publish_fee_ulmn`, `max_pending_ttl`, and `reject_refund_bps` to discourage abusive submissions.

- **REST exposure:** place a TLS-enabled reverse proxy with per-IP rate limiting in front of `:1317` if the API is exposed to the public internet.

- **Keys:** avoid the test keyring backend on production nodes. Prefer OS keyrings, KMS, or HSM integration for validator and publisher keys.

- **Governance:** parameter changes go through `MsgUpdateParams` (per-module authority). Document decisions and keep defaults version-controlled to aid audits.

---
## PQC policy (REQUIRED)

- PQC is **mandatory** for all EOA-signed transactions. There is no whitelist or TypeURL exemption—missing PQC signatures are rejected.
- Approved backends: **Circl** and **PQClean**. Release binaries panic at init if a non-approved backend is linked.
- Account rotate is **disabled** by design (no public CLI verb). Use `link-account` to attach initial PQC material.
- See `docs/modules/pqc.md` for signing, parameters, and CLI usage.

## Rate-limit clamps
- The ante decorator enforces per-account **per-block** caps, **per-window** quotas, and a **global** sliding window.
- Environment knobs: `LUMEN_RL_PER_BLOCK`, `LUMEN_RL_PER_WINDOW`, `LUMEN_RL_WINDOW_SEC`, `LUMEN_RL_GLOBAL_MAX`.
- Values are **clamped** at runtime: you can only tighten, not disable.

## Tx input caps
- **DNS:** labels ≤63 chars, FQDN ≤255, ≤64 records per tx, max 16 KiB combined record payload.
- **Gateways:** operator metadata ≤1 KiB, contract metadata ≤1 KiB, endpoint strings ≤64 chars.
- **Release:** version ≤64 chars, channel ≤32 chars, notes ≤8 KiB (UTF‑8, only `\n`/`\t` control chars). Artifact URLs ≤2048 chars (http/https), signature blobs ≤2048 bytes.
- **PQC:** scheme identifiers ≤32 chars, Dilithium public keys ≤4 KiB.

## Release safety guard
- CI workflow “Release Safety” and `make pre-release` verify:
  - No test-only/noop PQC symbols in the binary.
  - `go vet`, unit tests, static analysis, vuln scan, and preflight tests.

## Node hardening (prod)
- Do not expose `:1317` directly; put TLS reverse proxy + IP rate-limits in front.
- Avoid the test keyring in production; prefer OS keyrings/KMS/HSM for validators & publishers.
- Keep defaults and param changes versioned; review governance updates before rollout.

## Reporting vulnerabilities
- Please open a **private** security advisory on the repository (GitHub Security → “Report a vulnerability”),
  or contact maintainers via the email listed in the repository profile if available.
