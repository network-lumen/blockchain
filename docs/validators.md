# Validator & Operator Guide

This guide covers the essentials for running a Lumen node in production or staging environments.

## Building the Binary

```bash
# Local build (produces build/lumend)
make

# Cross-platform release artifacts (dist/<version>/…)
make build-release
```

Release artifacts contain Linux amd64/arm64 and macOS arm64 binaries plus `SHA256SUMS`.

## Systemd Service

Generate or install the unit file using the helper script:

```bash
# Dry run (prints unit to stdout)
make install-service ARGS="--print-unit"

# Install under /etc/systemd/system/lumend.service (requires sudo)
sudo make install-service
sudo systemctl enable --now lumend
```

The default unit starts the node with:

- `--minimum-gas-prices 0ulmn` (gasless operation)
- REST (`:2327`), gRPC (`:9190`), and gRPC-Web enabled on localhost
- Rate-limit env vars exported to the process (see below)

## Rate-Limit Environment Variables

Set these before launching `lumend` (or edit the systemd unit environment block):

| Variable | Default | Meaning |
|----------|---------|---------|
| `LUMEN_RL_PER_BLOCK` | `5` | Max gasless transactions per block per sender |
| `LUMEN_RL_PER_WINDOW` | `20` | Max gasless transactions within the sliding window |
| `LUMEN_RL_WINDOW_SEC` | `10` | Sliding-window length in seconds |

Example overrides:

```bash
export LUMEN_RL_PER_BLOCK=10
export LUMEN_RL_PER_WINDOW=40
export LUMEN_RL_WINDOW_SEC=30
lumend start --minimum-gas-prices 0ulmn
```

## Useful REST Queries

```bash
API=http://127.0.0.1:2327

# DNS
curl -s "$API/lumen/dns/v1/params" | jq
curl -s "$API/lumen/dns/v1/domain/example.lumen" | jq

# Gateways
curl -s "$API/lumen/gateway/v1/params" | jq
curl -s "$API/lumen/gateway/v1/gateways" | jq '.gateways[] | {id, operator, active}'

# Release metadata
curl -s "$API/lumen/release/params" | jq
curl -s "$API/lumen/release/latest?channel=stable&platform=linux-amd64&kind=daemon" | jq
```

## Networking & Security

- **Ports** : par défaut CometBFT écoute sur `26656` (P2P), `26657` (RPC), `2327` (REST), `9190` (gRPC), `9091` (gRPC-Web). Expose uniquement ce qui est nécessaire ; `26657` et REST peuvent rester en local si un reverse proxy est utilisé.
- **Pare-feu** : autorise `26656/tcp` depuis tes pairs/seed, bloque l’accès public à `26657` et `2327` sauf si tu ajoutes un proxy HTTPS avec limite de débit.
- **IPv4/IPv6** : configure `persistent_peers` avec les IP mixtes et valide que chaque peer annonce son `external_address`. Si tu utilises IPv6 only, renseigne `[addr]:port`.
- **DoS soft** : conserve `--minimum-gas-prices 0ulmn` (le decorateur rate-limit s’occupe d’empêcher les abus) et ajoute un proxy type nginx/Caddy pour forcer TLS + fail2ban/limit_req.
- **Accès SSH** : désactive root, utilise IPv6 + UFW `default deny`, et limite les connexions admin au strict nécessaire.
- **Clés** : privilégie un keyring chiffré (file/os) voire un HSM. Le backend `test` n’est acceptable qu’en labo.

## Bootstrap Automatisé du Validateur

Pour enchaîner toutes les étapes (init, clé Ed25519, clé PQC, gentx, service systemd) en une commande, utilisez le script
`devtools/scripts/bootstrap_validator.sh` :

```bash
./devtools/scripts/bootstrap_validator.sh \
  --moniker mon-validator \
  --chain-id lumen-testnet \
  --home /var/lib/lumen \
  --stake 1ulmn \
  --balance 1000ulmn \
  --pqc-passphrase-file ~/.config/lumen/pqc_pass
```

Le script :
- exécute `lumend init`, crée la clé `validator`, et crédite le compte dans le genesis ;
- génère une clé Dilithium locale (chiffrée si `--pqc-passphrase-file` est fourni) et insère l’entrée dans `genesis.json` ;
- produit le `gentx` et lance `collect-gentxs` ;
- installe optionnellement le service systemd (`--install-service`).

Conservez soigneusement la mnemonic Ed25519 et la passphrase PQC imprimées par le script.

## Sauvegarde & Restauration

Sauvegardez hors-ligne (USB chiffrée, coffre) les fichiers suivants :

- `config/priv_validator_key.json`
- `config/node_key.json`
- `config/priv_validator_state.json` (pouvant être recréé mais utile pour reprendre sans double-sign)
- `pqc_keys/keys.json` et `pqc_keys/links.json` + la passphrase associée
- Les exports de portefeuille (`lumend keys export` ou `keyring` sécurisé)

Pour restaurer un nœud :
1. Prépare un nouveau `$HOME` (ex. `/var/lib/lumen`), exécute `lumend init` pour générer les dossiers.
2. Remplace les fichiers listés ci-dessus par leurs sauvegardes (respecter les permissions 600).
3. Vérifie que l’adresse renvoyée par `lumend keys show validator` correspond bien à celle attendue.
4. Si le keystore PQC est chiffré, remets la passphrase via `--pqc-passphrase-file`.
5. Relance le service (`systemctl restart lumend`) et surveille les logs pour confirmer qu’aucune erreur PQC n’apparaît.

Perdre la clé PQC ou la clé Ed25519 rend impossible la reprise des signatures ; assure-toi donc d’avoir au minimum deux copies chiffrées et testées de ces fichiers critiques.

## Upgrades & Releases

- Use `make build-release` to produce reproducible artifacts.
- Tag releases (`git tag vX.Y.Z && git push origin vX.Y.Z`) after running the validation checklist in [`docs/releases.md`](releases.md).
- Update operators with parameter changes from governance proposals (see [`docs/params.md`](params.md)).
