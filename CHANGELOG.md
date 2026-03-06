# Changelog

## GnoDuty v1.0.0 — Hard Fork of Tenderduty v2.2.1

GnoDuty is a hard fork of [Tenderduty](https://github.com/blockpane/tenderduty) (MIT License, by Todd G / blockpane), rewritten to support Gno.land's Tendermint2 (TM2) architecture.

This document details every modification made to justify the hard fork classification.

---

### Why a Hard Fork?

Tenderduty was built for Cosmos SDK chains using standard Tendermint. Gno.land uses Tendermint2 (TM2), which introduces breaking incompatibilities:

- **No WebSocket subscriptions** on public RPC endpoints — Tenderduty's core connectivity model (`ws.go`) relies entirely on WebSocket event subscriptions (`subscribe` method) to watch new blocks. TM2 public endpoints do not support this.
- **Different block structure** — TM2 uses `precommits` in `last_commit` instead of `signatures`. Tenderduty's signing detection fails silently on TM2 blocks.
- **Different validator address format** — TM2 uses bech32 `g1...` addresses in both `/validators` and `/block` responses, while Cosmos SDK uses hex addresses in blocks and `valoper` addresses in staking queries.
- **No ABCI staking queries** — Tenderduty uses `abci_query` to fetch validator info from the staking module. TM2 has no staking module with the same query paths.
- **Different RPC response schema** — Multiple fields in `/status`, `/validators`, and `/block` responses have different names or structures, causing JSON unmarshalling failures.

These are not configuration differences — they are protocol-level incompatibilities that required rewriting the core monitoring engine.

---

### New Files (not in Tenderduty)

| File | Lines | Description |
|------|-------|-------------|
| `core/provider_gnoland.go` | 292 | Complete TM2 validator status provider. Queries `/validators` and `/block` endpoints with TM2-specific JSON parsing. Handles bech32 `g1...` address matching, validator set detection (bonded/jailed/tombstoned), and signing window calculation. |
| `core/ws_poll.go` | 170 | HTTP polling engine replacing WebSocket subscriptions. Polls `/block` every 5 seconds, parses TM2 `precommits` array (with null entry handling), matches validator signatures, and feeds results to the dashboard and alert system. |

**Total new code: 462 lines**

---

### Modified Files

| File | Original (td2/) | Modified (core/) | Changes |
|------|-----------------|-------------------|---------|
| `chain-details.go` | 157 lines | 14 lines | Gutted. Cosmos directory registry removed — not applicable to Gno.land. |
| `rpc.go` | 240 lines | 209 lines | Rewritten endpoint health checking for TM2. Removed Cosmos-specific WebSocket upgrade detection. Added HTTP-based health verification using TM2 `/status` endpoint. |
| `types.go` | 581 lines | 435 lines | Removed Cosmos SDK type definitions. Added TM2-specific types for validator responses and block parsing. |
| `run.go` | 183 lines | 185 lines | Modified chain initialization to use TM2 provider instead of Cosmos ABCI queries. Routes to HTTP polling instead of WebSocket subscriptions. |
| `ws.go` | 437 lines | 31 lines | Reduced to a minimal stub. Original WebSocket subscription logic entirely replaced by `ws_poll.go`. |
| `validator.go` | 204 lines | 29 lines | Simplified. Removed Cosmos SDK valoper-to-valcons address conversion. TM2 uses bech32 addresses directly. |
| `init.go` | 42 lines | 42 lines | Module path updated from `blockpane/tenderduty` to `aviaone/gnoduty`. |
| `main.go` | — | 4 lines changed | Module import path updated. State file default changed to `.gnoduty-state.json`. |

---

### Modified Frontend Files

| File | Changes |
|------|---------|
| `core/static/index.html` | Complete redesign. AdminLTE + Bootstrap 5 dark theme with orange accent. Responsive layout. SEO meta tags, Open Graph, Schema.org. Footer with Tenderduty attribution. |
| `core/static/status.js` | Fixed chain name display to avoid duplicate chain_id when name already contains it. |
| `core/static/grid.js` | Row height adjusted for single-chain display compatibility. |
| `core/static/automatic-date.js` | Added. Dashboard UI utility script. |

---

### Adapted Files (from Tenderduty)

| File | Changes |
|------|--------|
| `Dockerfile` | Multi-stage build adapted for GnoDuty. golang:1.19 → 1.21, debian:11 → 12, user/paths renamed from tenderduty to gnoduty, upx compression removed. |
| `example-docker-compose.yml` | Ports updated (8889/28687). Caddy service and monitor-net network removed — reverse proxy is now independent of Docker. |
| `.dockerignore` | Updated to exclude `.gnoduty-state.json` and GnoDuty-specific files. |

---

### Removed Files and Directories

| Path | Reason |
|------|--------|
| `td2/` (entire directory) | Replaced by `core/` with TM2-compatible code. |
| `docs/` | Tenderduty-specific documentation (Cosmos SDK setup, Akash deployment, etc.) — not applicable to GnoDuty. |
| `caddy/` | Tenderduty-specific Caddy configuration. |
| `td2/static/bp-logo-text.svg` | Blockpane logo — not applicable to GnoDuty. |

---

### Structural Changes

| Aspect | Tenderduty | GnoDuty |
|--------|-----------|---------|
| Go module | `github.com/blockpane/tenderduty/v2` | `github.com/aviaone/gnoduty/v2` |
| Source directory | `td2/` | `core/` |
| Binary name | `tenderduty` | `gnoduty` |
| State file | `.tenderduty-state.json` | `.gnoduty-state.json` |
| Config directory | `.tenderduty/` | `.gnoduty/` |
| Default dashboard port | 8888 | 8889 |
| Default Prometheus port | 28686 | 28687 |

---

### Code Statistics

| Metric | Value |
|--------|-------|
| Total Go source lines | 2,677 |
| New files created | 2 (462 lines) |
| Files significantly rewritten | 6 |
| Files removed | 20+ |
| Directories restructured | `td2/` → `core/` |

---

### Unchanged from Tenderduty

The following components are inherited from Tenderduty with minimal or no modification:

- `core/alert.go` — Alert dispatch system (Discord, Telegram, Slack, PagerDuty)
- `core/prometheus.go` — Prometheus metrics exporter
- `core/encryption.go` — Config file encryption/decryption
- `core/dashboard/` — Dashboard WebSocket server for real-time UI updates
- `core/static/css/` — UIKit CSS framework
- `core/static/js/` — UIKit JS, lodash
- `core/static/grid.js` — Canvas block visualization (minor adjustments)

These components are framework-level code that works independently of the blockchain protocol. The Tenderduty alert and dashboard architecture is solid and did not require changes for TM2 compatibility.

---

### License

GnoDuty is released under the MIT License, same as Tenderduty.

- Original: Copyright (c) 2021 Todd G (blockpane)
- Fork: Copyright (c) 2026 AviaOne.com
