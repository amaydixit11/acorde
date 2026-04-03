# Setup Guide

This guide focuses on the current CLI and daemon workflow.

## Build From Source

Requirements:

- Go
- GCC or another C compiler for `go-sqlite3`

Build:

```bash
git clone https://github.com/amaydixit11/acorde.git
cd acorde
go build -o acorde ./cmd/acorde
```

## Default Data Location

If `--data` is omitted, Acorde uses:

```text
~/.acorde
```

Useful files inside a data dir:

- `acorde.db` - SQLite data store
- `node_id` - local peer ID
- `node_key` - libp2p identity key
- `peer_addrs.json` - daemon's persisted live addresses
- `peers.json` - allowlisted peers

## Start A Node

```bash
./acorde daemon --data /tmp/acorde-a --port 4001 --api-port 7331
```

This starts:

- the local engine
- the p2p sync service
- the REST API on `localhost:7331`

## Single-Node Smoke Test

```bash
./acorde add --data /tmp/acorde-a --type note --content "hello"
./acorde list --data /tmp/acorde-a
./acorde get --data /tmp/acorde-a <entry-id>
./acorde update --data /tmp/acorde-a <entry-id> --content "updated hello"
./acorde delete --data /tmp/acorde-a <entry-id>
curl -s http://localhost:7331/entries
```

## Two-Node Sync Test

Terminal 1:

```bash
./acorde daemon --data /tmp/acorde-a --port 4001 --api-port 7331
```

Terminal 2:

```bash
./acorde daemon --data /tmp/acorde-b --port 4002 --api-port 7332
```

Terminal 3:

```bash
./acorde add --data /tmp/acorde-a --type note --content "hello from A"
sqlite3 /tmp/acorde-b/acorde.db 'select id,type,updated_at,deleted from entries order by id;'
```

Authorize B to write:

```bash
cat /tmp/acorde-b/node_id
./acorde authorize --data /tmp/acorde-a <entry-id> <peer-id-from-b>
```

Update from B:

```bash
./acorde update --data /tmp/acorde-b <entry-id> --content "updated from B"
```

If you want to verify the remote tombstone after deletion:

```bash
./acorde delete --data /tmp/acorde-b <entry-id>
sqlite3 /tmp/acorde-a/acorde.db "select id,deleted from entries where id='<entry-id>';"
```

## Direct Pairing Without mDNS

Device A:

```bash
./acorde daemon --data /tmp/acorde-pair-a --port 4021 --api-port 7351 --mdns=false
./acorde invite --data /tmp/acorde-pair-a
```

Device B:

```bash
./acorde pair --data /tmp/acorde-pair-b 'acorde://...'
./acorde daemon --data /tmp/acorde-pair-b --port 4022 --api-port 7352 --mdns=false
```

Notes:

- `pair` stores the peer in the allowlist.
- After pairing, start the daemon if it is not already running.
- `invite` uses the daemon's live addresses when available.

## Encrypted Vaults

Initialize:

```bash
./acorde init --data /tmp/acorde-enc-a
```

Then use the normal commands:

```bash
./acorde add --data /tmp/acorde-enc-a --type note --content "secret"
./acorde list --data /tmp/acorde-enc-a
./acorde get --data /tmp/acorde-enc-a <entry-id>
```

## Operational Notes

- Private synced entries may exist in SQLite on another peer without appearing in `list` or `/entries` until that peer has read access.
- Writers can read the entries they are allowed to write.
- CLI writes made against the same vault while a daemon is already running are picked up and synced by that daemon.
