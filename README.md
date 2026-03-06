# GnoDuty

Real-time validator monitoring for **Gno.land (TM2)** networks.

Hard fork of [Tenderduty](https://github.com/blockpane/tenderduty) by [blockpane](https://github.com/blockpane), rewritten for Gno.land's Tendermint2 (TM2) architecture.

## Why GnoDuty?

Tenderduty was designed for Cosmos SDK / Tendermint chains. Gno.land uses Tendermint2 (TM2), which has a different RPC response format, different validator address handling, and no WebSocket subscription support on public endpoints.

GnoDuty solves this by:

- Parsing TM2 block format (`precommits` instead of `signatures`)
- Using HTTP polling instead of WebSocket subscriptions
- Supporting bech32 `g1...` validator addresses natively
- Querying TM2-specific endpoints (`/validators`, `/block`)

## Features

- **Real-time block monitoring** - Track block signing with visual canvas display
- **Validator status** - Bonded, jailed, tombstoned, missed blocks, uptime percentage
- **Multi-chain support** - Monitor multiple Gno.land networks from a single instance
- **Alert notifications** - Discord, Telegram, Slack, PagerDuty
- **Web dashboard** - Dark theme, responsive, real-time updates
- **Prometheus exporter** - Metrics for Grafana integration
- **Light/Dark mode** - Toggle from the dashboard footer

## Screenshot

![GnoDuty Dashboard](readme-screenshot.jpg)

---

## Installation — Linux (Ubuntu/Debian)

### Requirements

- Go 1.21 or later (tested with 1.26)
- Linux (tested on Ubuntu 24.04)
- Ports 8889 (dashboard) and 28687 (Prometheus) must be open

If you use UFW, open the required ports before starting:
```bash
sudo ufw allow 8889/tcp comment "GnoDuty Dashboard"
sudo ufw allow 28687/tcp comment "GnoDuty Prometheus"
```

### Step 1 — Create a dedicated system user

A dedicated system user improves security by isolating GnoDuty from other services. Run this from your regular user account (with sudo access):
```bash
sudo useradd -r -s /bin/false -m -d /var/lib/gnoduty gnoduty
```

### Step 2 — Install GnoDuty
```bash
sudo -u gnoduty bash -c 'cd ~ && git clone https://github.com/AviaOne/gnoduty && cd gnoduty && go install'
```

### Step 3 — Configure
```bash
sudo -u gnoduty mkdir -p /var/lib/gnoduty/.gnoduty
sudo -u gnoduty cp /var/lib/gnoduty/gnoduty/example-config.yml /var/lib/gnoduty/.gnoduty/config.yml
```

Edit the configuration file:
```bash
sudo -u gnoduty nano /var/lib/gnoduty/.gnoduty/config.yml
```

The most important settings to change:
```yaml
chains:
  # Display name shown on the dashboard (can be anything)
  "Gno.land (test11)":
    # Must match the chain ID from the RPC /status endpoint
    chain_id: test11
    # Your validator address in bech32 format (g1...)
    valoper_address: g1_YOUR_VALIDATOR_ADDRESS
    nodes:
      - url: https://rpc.test11.testnets.gno.land:443
        alert_if_down: yes
```

To verify your chain_id:
```bash
curl -s https://rpc.test11.testnets.gno.land/status | python3 -c "import json,sys; print(json.load(sys.stdin)['result']['node_info']['network'])"
```

See `example-config.yml` for a complete configuration reference with all options.

### Step 4 — Configure alerts

GnoDuty supports alerts via Telegram, Discord, Slack, and PagerDuty when your validator misses blocks, goes offline, or gets jailed.

**Important:** Both global AND per-chain alert settings must be enabled. If either is disabled, no alerts will be sent.

**Global settings** (top of `config.yml`):
```yaml
telegram:
  enabled: yes
  api_key: 'YOUR_BOT_API_KEY'
  channel: "-YOUR_CHANNEL_ID"
```

**Per-chain settings** (inside each chain's `alerts:` section):
```yaml
    alerts:
      # ... alert thresholds ...
      telegram:
        enabled: yes
        api_key: ""  # leave empty to use global settings
        channel: ""  # leave empty to use global settings
```

If the per-chain section is missing or `enabled: no`, alerts will NOT be sent for that chain even if the global setting is enabled.

### Step 5 — Create the systemd service

Create `/etc/systemd/system/gnoduty.service`:
```ini
[Unit]
Description=GnoDuty - Gno.land Validator Monitor
After=network-online.target
Wants=network-online.target

[Service]
User=gnoduty
Group=gnoduty
Type=simple
WorkingDirectory=/var/lib/gnoduty/.gnoduty
ExecStart=/var/lib/gnoduty/go/bin/gnoduty -f /var/lib/gnoduty/.gnoduty/config.yml
Restart=always
RestartSec=5
LimitNOFILE=65535

[Install]
WantedBy=multi-user.target
```
```bash
sudo systemctl daemon-reload
sudo systemctl enable --now gnoduty
```

The dashboard will be available at `http://YOUR_SERVER_IP:8889`

### Service management
```bash
# Start
sudo systemctl start gnoduty

# Stop
sudo systemctl stop gnoduty

# Restart
sudo systemctl restart gnoduty

# Status
sudo systemctl status gnoduty

# Live logs
sudo journalctl -u gnoduty --no-hostname -f
```

---

## Installation — Docker

### Requirements

- Docker and Docker Compose installed
- Ports 8889 (dashboard) and 28687 (Prometheus) must be open

### Step 1 — Set up the project directory
```bash
mkdir gnoduty && cd gnoduty
git clone https://github.com/AviaOne/gnoduty .
```

### Step 2 — Create configuration files
```bash
cp example-config.yml config.yml
cp example-docker-compose.yml docker-compose.yml
```

### Step 3 — Edit config.yml

Edit `config.yml` with your validator address, RPC endpoint, and alert settings (see the "Configure alerts" section above).

### Step 4 — Build and start
```bash
docker-compose up -d
docker-compose logs -f --tail 20
```

The dashboard will be available at `http://YOUR_SERVER_IP:8889`

### Docker management
```bash
# Stop
docker-compose down

# Restart
docker-compose restart

# View logs
docker-compose logs -f --tail 20

# Rebuild after update
docker-compose up -d --build
```

---

## Configuration

### Runtime options

GnoDuty accepts command-line flags to override default file paths. This is useful if you want to store configuration or state files in custom locations:
```
$ gnoduty -h
Usage of gnoduty:
  -f string
        configuration file to use (default "config.yml")
  -state string
        file for storing state between restarts (default ".gnoduty-state.json")
  -cc string
        directory containing additional chain specific configurations (default "chains.d")
```

---

## What Changed from Tenderduty

GnoDuty is a hard fork with significant modifications to support Gno.land's TM2:

| Component | Tenderduty (Cosmos SDK) | GnoDuty (Gno.land TM2) |
|-----------|------------------------|------------------------|
| Block data | `signatures` field | `precommits` field |
| Connectivity | WebSocket subscriptions | HTTP polling (5s interval) |
| Validator address | Hex / valoper | Bech32 `g1...` |
| Validator lookup | ABCI queries | `/validators` RPC endpoint |
| Signing check | `last_commit.signatures` | `last_commit.precommits` |

**Fork statistics:**
- **2 new files created** (462 lines) — TM2 polling engine and validator provider, written from scratch
- **8 files significantly rewritten** (1,894 → 976 lines) — Over 900 lines removed and replaced with TM2-compatible code
- **20+ files removed** — Cosmos SDK documentation, Docker configs, and assets replaced
- **Directory restructured** — `td2/` → `core/`, full module path rewrite
- **~40% of the original codebase was rewritten or removed**
- **60% unchanged** — Alert system, Prometheus, encryption, and dashboard inherited from Tenderduty

See [CHANGELOG.md](CHANGELOG.md) for a detailed breakdown of every file created, modified, and removed.

## Credits

- **Original**: [Tenderduty v2](https://github.com/blockpane/tenderduty) by [Todd G (blockpane)](https://github.com/blockpane), sponsored by the [Osmosis Grants Program](https://grants.osmosis.zone/)
- **Fork**: [GnoDuty](https://github.com/AviaOne/gnoduty) by [AviaOne.com](https://aviaone.com) — Adapted for Gno.land TM2

## Disclaimer

The original Tenderduty project was developed [with a $10,000 grant](https://grants.osmosis.zone/grants/tenderduty-v2-validator-monitoring-tool) from the Osmosis Grants Program.
GnoDuty is currently maintained only by [AviaOne.com](https://aviaone.com) contribution with no funding or sponsorship.

While we strive for quality, we cannot guarantee the same level of support as a funded project. If you encounter bugs, please open an issue on GitHub, we will do our best to address them.

## License

MIT License - See [LICENSE](LICENSE) for details.

Original © 2021 Todd G (blockpane) | Fork © 2026 AviaOne.com
