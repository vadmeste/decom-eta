# decom-eta

A CLI tool that monitors MinIO pool decommissioning and estimates the time remaining.

## Install

```
go install github.com/vadmeste/decom-eta@latest
```

## Usage

```
decom-eta [-config-dir <path>] [-watch] <alias>
```

- `<alias>` — the mc alias name for your MinIO cluster
- `-config-dir` — path to the mc config directory (default: `~/.mc`)
- `-watch` — continuously monitor decommission status, refreshing every 10 seconds

The tool reads the alias credentials from mc's `config.json` and queries the MinIO admin API for pool decommission status.

## Example

```
$ decom-eta mycluster
Pool #1: http://minio1/data/disk{1...4}
  Started: 2026-02-16T20:08:43Z (1 minute ago)
  Progress: 219 MiB / 237 MiB freed (92.4%)
  Current usage: 18 MiB / 1.0 GiB (1.8%)
  Speed: 2.7 MiB/sec
  ETA: 2026-02-16T20:10:09Z (< 1m remaining)
```

When no pools are being decommissioned:

```
$ decom-eta mycluster
No pools are currently being decommissioned.
```
