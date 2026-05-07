# Backup Downloader

Downloads backups from a remote SFTP server and stores them for the required number of days.
Thus, one copy can be stored on the main server to save space, and as many copies as you want on the backup server.

## Build

Change the target platforms in the file: build.sh (if you need)

```
platforms=("linux/amd64" "darwin/amd64" "linux/arm64")
```

Run the build command

```shell
sh build.sh
```

The binary files will appear in the build directory.


## Run

Run a binary file with the CONFIG_PATH environment variable pointing to the directory with `config.json`

```shell
CONFIG_PATH=/path/to/config ./linux-amd64
```


## Configuration

Copy [config.json.example](config.json.example) to your config directory and rename it to `config.json`.

| Field | Description |
|---|---|
| `storagePath` | Local directory where downloaded backups are stored |
| `startBackupsHour` | Hour of the day (0–23) when the backup download starts |
| `servers` | List of remote servers to download from |

Each server entry:

| Field | Description |
|---|---|
| `active` | Enable or disable the server without removing it from config |
| `name` | Unique server name, used as a subdirectory name in `storagePath` |
| `daysCount` | How many days to keep downloaded files locally |
| `maxParallelDownloads` | Number of files downloaded simultaneously from this server |
| `backupsPath` | Remote path to walk for backup files |
| `server` | Remote server hostname or IP |
| `user` / `password` | SSH credentials |
| `port` | SSH port (usually 22) |
| `pathTemplate` | File naming convention on the remote server (see below) |
| `filePattern` | Optional regexp to filter remote files by path |

### Path templates

| Value | Remote path format | Extensions |
|---|---|---|
| `hestia` | `/backup/admin.2023-12-25_05-11-45.tar` | `.tar` |
| `filesWithDate` | `/backups/test.20231221.sql.gz` | `.gz` |
| `pathWithDate` | `/24.12.23/test.tgz` | `.tgz`, `.bz2` |

The template determines how the date is extracted from the path (used to detect and delete files older than `daysCount` days).

## Service example

```shell
systemctl edit --full --force backuper.service
```

Config example

Change the ExecStart and Environment parameters

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

```shell
systemctl daemon-reload
```

```shell
systemctl start backuper
```

Enable autostart

```shell
systemctl enable backuper
```

