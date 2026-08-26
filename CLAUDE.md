# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Commands

```bash
sh build.sh                          # сборка для всех платформ → ./build/
CONFIG_PATH=/path/to/config ./backuper
```

## Architecture

Один пакет `main`: `config.go` (загрузка конфига через viper, глобальные `backuperConfig`/`storagePath`), `client.go` (SSH+SFTP соединения, обход удалённой директории), `files.go` (скачивание, удаление старых файлов), `main.go` (основной цикл с пулом горутин `ants`).

Поток одного прохода: `deleteOldFiles` чистит хранилище → `getRemoteFiles` обходит удалённое дерево и отсеивает неподходящие пути → `downloadFileFromServer` качает то, чего ещё нет локально. Дата из пути — единственный источник истины о возрасте копии, `isOldFile` используется и при обходе, и при чистке.

`deleteOldFiles` рекурсивный (`filepath.WalkDir`) и передаёт в `isOldFile` путь относительно `storagePath/<name>`, потому что `nxsBackup` раскладывает файлы по вложенным каталогам; после удаления `removeEmptyDirs` подчищает опустевшие каталоги. Существующий локальный файл никогда не перекачивается.

## Path templates

Поле `pathTemplate` в конфиге определяет, как парсить даты и какие расширения принимать:

| Значение | Формат пути | Расширения |
|---|---|---|
| `hestia` | `/backup/admin.2023-12-25_05-11-45.tar` | `.tar` |
| `filesWithDate` | `/backups/test.20231221.sql.gz` | `.gz` |
| `pathWithDate` | `/24.12.23/test.tgz` | `.tgz`, `.bz2` |
| `nxsBackup` | `/backups/configs/acme/daily/acme_2026-08-18_02-00.tar.gz` | `.gz`, `.tgz`, `.tar`, `.bz2`, `.xz`, `.zst`, `.zip` |

`nxsBackup` зеркалит удалённое дерево в локальном хранилище и использует ретеншен по периодам (`retention.daily/weekly/monthly`, дни), с фолбэком на `daysCount`. Формат: `<группа>/<источник>/<daily|weekly|monthly>/<имя>_YYYY-MM-DD_HH-MM.<ext>`, всё остальное под `backupsPath` игнорируется (`parseNxsBackupPath` в `files.go`).

Симлинки daily → weekly/monthly **намеренно не фильтруются** в `getRemoteFiles`: SFTP резолвит их при `Open()`, поэтому скачивается оригинал, а имя берётся из пути самого симлинка. Иначе удаление weekly по своему ретеншену оставило бы битую ссылку в `daily/`.
