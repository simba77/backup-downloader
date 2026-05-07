# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Commands

```bash
sh build.sh                          # сборка для всех платформ → ./build/
CONFIG_PATH=/path/to/config ./backuper
```

## Architecture

Один пакет `main`: `config.go` (загрузка конфига через viper, глобальные `backuperConfig`/`storagePath`), `client.go` (SSH+SFTP соединения, обход удалённой директории), `files.go` (скачивание, удаление старых файлов), `main.go` (основной цикл с пулом горутин `ants`).

## Path templates

Поле `pathTemplate` в конфиге определяет, как парсить даты и какие расширения принимать:

| Значение | Формат пути | Расширения |
|---|---|---|
| `hestia` | `/backup/admin.2023-12-25_05-11-45.tar` | `.tar` |
| `filesWithDate` | `/backups/test.20231221.sql.gz` | `.gz` |
| `pathWithDate` | `/24.12.23/test.tgz` | `.tgz`, `.bz2` |
