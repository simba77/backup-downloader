# Backup Downloader

Downloads backups from a remote SFTP server and stores them for the required number of days. 
Thus, one copy can be stored on the main server to save space, and as many copies as you want on the backup server.

This version is currently intended for backups created by the Hestia control panel.

## Build

Change the target platforms in the file: build.sh (if you need)

```
platforms=("linux/amd64" "darwin/amd64")
```

Run the build command

```shell
sh build.sh
```

The binary files will appear in the build directory.


## Run

Copy the [config.json.example](config.json.example) file to any folder you wish.

Rename file to config.json and change it.

Run a binary file with the CONFIG_PATH environment variable

```shell
CONFIG_PATH=/path/to/config ./linux-amd64
```


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

