# Backup Downloader

Downloads backups from a remote SFTP server and stores them for the required number of days.
Thus, one copy can be stored on the main server to save space, and as many copies as you want on the backup server.

## Installation

Prebuilt binaries for `linux-amd64`, `linux-arm64` and `darwin-amd64` are attached to every
[release](https://github.com/simba77/backup-downloader/releases). The steps below install the latest one on Linux
into `/backups`, the directory that also holds `config.json` in the service example — use your own if you like.

Download the binary for the server architecture and make it executable:

```shell
mkdir -p /backups
ARCH=$(uname -m | sed -e 's/x86_64/amd64/' -e 's/aarch64/arm64/')
curl -fL -o /backups/backuper https://github.com/simba77/backup-downloader/releases/latest/download/linux-$ARCH
chmod +x /backups/backuper
```

Put the configuration next to it, then fill it in as described in [Configuration](#configuration):

```shell
curl -fL -o /backups/config.json https://raw.githubusercontent.com/simba77/backup-downloader/main/config.json.example
```

Then run it as a service, see [Service example](#service-example).

### Updating

Replace the binary with the new one and restart the service. The download goes to a temporary file so that
a failed one does not leave the service with a broken binary:

```shell
ARCH=$(uname -m | sed -e 's/x86_64/amd64/' -e 's/aarch64/arm64/')
curl -fL -o /backups/backuper.new https://github.com/simba77/backup-downloader/releases/latest/download/linux-$ARCH
chmod +x /backups/backuper.new && mv /backups/backuper.new /backups/backuper
systemctl restart backuper
```

To install a specific version, replace `latest/download` with `download/<tag>`, e.g. `download/v1.1.0`.

If the service fails with `status=203/EXEC`, systemd could not execute the binary: check that it is executable
(`ls -la`) and built for the server architecture (`file /backups/backuper` must say `ELF`, and match `uname -m`).

## Build from source

Change the target platforms in the file: build.sh (if you need)

```
platforms=("linux/amd64" "darwin/amd64" "linux/arm64")
```

Run the build command

```shell
sh build.sh
```

The binary files will appear in the build directory.

### Releases

Pushing a tag that starts with `v` runs [.github/workflows/release.yml](.github/workflows/release.yml): it builds
the same platforms through `build.sh` and attaches the binaries to a GitHub release named after the tag.

```shell
git tag -a v1.1.0 -m v1.1.0 && git push origin v1.1.0
```

Note that `build.sh` needs bash for its array of platforms — run it as `bash build.sh` on systems where `sh` is dash.


## Run

Run a binary file with the CONFIG_PATH environment variable pointing to the directory with `config.json`.
Binaries in releases and in `build/` are named after their target platform (`linux-amd64`, `linux-arm64`,
`darwin-amd64`); the installation above renames it to `backuper`.
When `CONFIG_PATH` is not set, `config.json` is looked up in the current working directory.

```shell
CONFIG_PATH=/path/to/config ./linux-amd64
```

The process runs in a loop: on every pass it deletes the outdated local files, downloads the current ones and
sleeps until `startBackupsHour` of the next day. Logs are written to `storagePath/logs/log-DD-MM-YYYY.log`,
one file per day, kept for `logRetentionDays` days.


## Configuration

Copy [config.json.example](config.json.example) to your config directory and rename it to `config.json`.

| Field | Description |
|---|---|
| `storagePath` | Local directory where downloaded backups are stored. A path not starting with `/` is resolved against the current working directory |
| `startBackupsHour` | Hour of the day (0–23) when the backup download starts |
| `logRetentionDays` | How many days to keep log files (default: 14) |
| `servers` | List of remote servers to download from |

Each server entry:

| Field | Description |
|---|---|
| `active` | Enable or disable the server without removing it from config |
| `name` | Unique server name, used as a subdirectory name in `storagePath` |
| `daysCount` | How many days to keep downloaded files locally. For the `nxsBackup` template it is only the fallback for periods missing from `retention` |
| `maxParallelDownloads` | Number of files downloaded simultaneously from this server |
| `backupsPath` | Remote path to walk for backup files |
| `type` | Reserved for future transports, currently ignored — the connection is always SSH + SFTP |
| `server` | Remote server hostname or IP |
| `user` / `password` | SSH credentials |
| `port` | SSH port (usually 22) |
| `pathTemplate` | File naming convention on the remote server (see below) |
| `filePattern` | Optional regexp to filter remote files by path |
| `retention` | Per-period retention in days for the `nxsBackup` template: `daily`, `weekly`, `monthly`. Any period left out falls back to `daysCount` |
| `sequentialReads` | Read every file with one request at a time instead of many concurrent ones (default: `false`). Slower, but required when `backupsPath` cannot serve reads out of order, e.g. an FTP share mounted with `curlftpfs` — otherwise large files fail with `sftp: "Failure" (SSH_FX_FAILURE)` |
| `minCopies` | How many of the newest copies of every backup are kept and downloaded even when they are past the retention (default: 1) |

### Path templates

| Value | Remote path format | Extensions |
|---|---|---|
| `hestia` | `/backup/admin.2023-12-25_05-11-45.tar` | `.tar` |
| `filesWithDate` | `/backups/test.20231221.sql.gz` | `.gz` |
| `pathWithDate` | `/24.12.23/test.tgz` | `.tgz`, `.bz2` |
| `nxsBackup` | `/backups/configs/acme/daily/acme_2026-08-18_02-00.tar.gz` | `.gz`, `.tgz`, `.tar`, `.bz2`, `.xz`, `.zst`, `.zip` |

The template determines how the date is extracted from the path, which extensions are accepted and how the file
is laid out locally. The date is what makes a copy outdated: files older than the retention are neither
downloaded nor kept in the storage — except for the newest copies protected by `minCopies`.

### minCopies

The age of a copy is taken from its path, not from the state of the remote server. If backups stop being made
(no disk space left, a broken cron, a dead service), no new copies appear while the existing ones keep ageing,
and a retention applied by date alone would delete the last backups the storage has.

`minCopies` (default: 1) is the guard against that: the newest `minCopies` copies of every backup series are
never deleted and are downloaded even when they are older than the retention, so a source server that stopped
making backups leaves the last copies in place instead of silently emptying the storage. As soon as fresh
copies appear again, the outdated ones are cleaned up on the next pass as usual.

A series is a single backup over time, not the whole server — the protection is per series, so one broken
source does not stop the cleanup of the others:

| Template | Series |
|---|---|
| `hestia`, `filesWithDate` | The file name with the date removed, e.g. all copies of `admin` |
| `pathWithDate` | The file name, e.g. all copies of `test.tgz` |
| `nxsBackup` | The `<group>/<source>/<period>` directory, e.g. `configs/acme/daily` |

Raising `minCopies` also raises the guaranteed depth of the history: with `minCopies: 2` the storage always
keeps at least two copies of every backup. The copies protected this way are logged as
`Keep File: ... - the last copies of this backup, no newer ones`.

### nxsBackup

[nxs-backup](https://github.com/nixys/nxs-backup) lays copies out as
`<group>/<source>/<daily|weekly|monthly>/<source>_<YYYY-MM-DD>_<HH-MM>.<ext>`. Only paths matching that
layout are downloaded, everything else under `backupsPath` is ignored.

Two things are specific to this template:

* **The remote tree is mirrored locally**, e.g. `storagePath/mail/configs/acme/daily/acme_2026-08-18_02-00.tar.gz`.
  A flat layout is not possible here: the daily, weekly and monthly copies of the same backup share one file name.
* **Retention is counted per period** via the `retention` field, because monthly copies are older than any sane
  `daysCount` and would be deleted right after being downloaded. Empty directories are removed after the cleanup.

nxs-backup replaces a daily copy with a symlink to the weekly or monthly one
(`daily/acme_2026-08-23_02-01.tar.gz -> ../weekly/acme_2026-08-23_02-01.tar.gz`). Such links are downloaded as
regular files — SFTP resolves them on open — so every local copy is self-contained and deleting a weekly file
by its own retention never leaves a dangling link in `daily/`.

### Local storage layout

Every server gets its own directory named after `name` inside `storagePath`. What happens inside depends on
the template:

| Template | Local path |
|---|---|
| `hestia`, `filesWithDate` | `storagePath/<name>/<file>` |
| `pathWithDate` | `storagePath/<name>/<date>_<file>` — the date from the remote directory is prepended to keep the names unique |
| `nxsBackup` | `storagePath/<name>/<remote path relative to backupsPath>` — the remote tree is mirrored |

A file that already exists locally is never downloaded again. A file is downloaded to `<file>.part` and gets
its final name only when it has been copied completely, so an interrupted download is retried on the next pass;
leftover `.part` files are cleaned up automatically.

## Service example

Create the unit:

```shell
systemctl edit --full --force backuper.service
```

Change `ExecStart` to the binary path and `CONFIG_PATH` to the directory with `config.json`:

```
[Unit]
Description=Backuper
Wants=network-online.target
After=network-online.target
[Service]
Environment="CONFIG_PATH=/backups"
User=root
Group=root
Type=simple
ExecStart=/backups/backuper
[Install]
WantedBy=multi-user.target
```

Start it and enable autostart:

```shell
systemctl daemon-reload
systemctl enable --now backuper
systemctl status backuper
```

Errors that happen before the log file is opened, such as a broken `config.json`, go to the journal
(`journalctl -u backuper`); everything else is logged to `storagePath/logs/`.

