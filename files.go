package main

import (
	"fmt"
	"github.com/pkg/sftp"
	"io"
	"log"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

func checkAndCreateStorageDirectory(server Server) {
	storagePathForServer := storagePath + server.Name + "/"
	if _, err := os.Stat(storagePathForServer); os.IsNotExist(err) {
		err := os.Mkdir(storagePathForServer, 0755)
		if err != nil {
			log.Fatal(err)
			return
		}
	}
}

// partialSuffix marks a file that is still being downloaded. It gets the final
// name only after the whole file has been copied, so an interrupted download
// never looks like an existing copy.
const partialSuffix = ".part"

// localFilePath returns where the remote file is stored locally
func localFilePath(server Server, remotePath string) string {
	storagePathForServer := storagePath + server.Name + "/"

	switch server.PathTemplate {
	case PathWithDate:
		// Add date to filename
		date := pathWithDateExpression.FindString(remotePath)
		return storagePathForServer + date + "_" + path.Base(remotePath)
	case NxsBackup:
		// Mirror the remote tree, otherwise the daily/weekly/monthly copies
		// of the same backup would collide by name
		return storagePathForServer + nxsBackupRelativePath(server, remotePath)
	default:
		return storagePathForServer + path.Base(remotePath)
	}
}

// localFileExists reports whether the remote file has already been downloaded
func localFileExists(server Server, remotePath string) bool {
	_, err := os.Lstat(localFilePath(server, remotePath))
	return err == nil
}

func downloadFileFromServer(client *sftp.Client, server Server, filename string) {
	log.Printf("[%s] Starting download - %s \n", server.Name, filename)

	// Checked before opening the remote file to save a round trip to the server
	localFileName := localFilePath(server, filename)
	if localFileExists(server, filename) {
		log.Printf("[%s] File exists %s. Skip downloading\n", server.Name, path.Base(filename))
		return
	}

	remoteFile, err := client.Open(filename)
	if err != nil {
		log.Printf("[%s] %s: %v", server.Name, filename, err)
		return
	}
	defer remoteFile.Close()

	file, err := remoteFile.Stat()
	if err != nil {
		log.Printf("[%s] %s: %v", server.Name, filename, err)
		return
	}

	if err := os.MkdirAll(filepath.Dir(localFileName), 0755); err != nil {
		log.Println(err)
		return
	}

	partialFileName := localFileName + partialSuffix
	writer, err := os.OpenFile(partialFileName, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		log.Println(err)
		return
	}

	t1 := time.Now()
	n, copyErr := io.Copy(writer, remoteFile)
	closeErr := writer.Close()

	switch {
	case copyErr != nil:
		err = copyErr
	case closeErr != nil:
		err = closeErr
	case n != file.Size():
		err = fmt.Errorf("expected %v bytes, got %d", file.Size(), n)
	}
	if err != nil {
		log.Printf("[%s] Download failed - %s: %v", server.Name, filename, err)
		if removeErr := os.Remove(partialFileName); removeErr != nil {
			log.Println(removeErr)
		}
		return
	}

	if err := os.Rename(partialFileName, localFileName); err != nil {
		log.Println(err)
		return
	}

	log.Printf("[%s] Downloaded - %s - %v bytes in %s \n", server.Name, file.Name(), file.Size(), time.Since(t1))
}

// nxs-backup stores copies as <group>/<source>/<daily|weekly|monthly>/<name>_2026-08-18_02-00.<ext>
var nxsBackupPathExpression = regexp.MustCompile(`(?:^|/)(daily|weekly|monthly)/[^/]+_(\d{4}-\d{2}-\d{2}_\d{2}-\d{2})\.`)

// The date in the path is the only source of truth about the age of a copy
var (
	hestiaDateExpression    = regexp.MustCompile(`(\d{4})-(\d{1,2})-(\d{1,2})`)
	filesWithDateExpression = regexp.MustCompile(`(\d{8})`)
	pathWithDateExpression  = regexp.MustCompile(`(\d{2}).(\d{1,2}).(\d{1,2})`)
	// Matches the date of any template, used to strip it from the name when
	// grouping the copies of the same backup into one series
	seriesDateExpression = regexp.MustCompile(`\d{4}-\d{1,2}-\d{1,2}(?:[_T ]\d{1,2}[-:]\d{1,2}(?:[-:]\d{1,2})?)?|\d{8}|\d{2}\.\d{1,2}\.\d{1,2}`)
)

// defaultMinCopies is the number of the newest copies of every backup series
// kept regardless of their age when the server has no minCopies configured
const defaultMinCopies = 1

// backupCopy is a backup file recognized by the path template of the server
type backupCopy struct {
	// path as it was given: a remote path or a path relative to the local storage of the server
	path string
	// created is the moment the copy was made, taken from the path
	created time.Time
	// parsed reports whether the path template recognized the path
	parsed bool
	// expired reports that the copy is past its retention and may be deleted
	expired bool
	// protected reports that the copy is past its retention but is kept
	// because it is among the newest copies of its series
	protected bool
}

// parseNxsBackupPath extracts the copy period and its creation time from an
// nxs-backup path. Works both with remote absolute paths and with paths
// relative to the local storage directory of the server.
func parseNxsBackupPath(filePath string) (string, time.Time, bool) {
	matches := nxsBackupPathExpression.FindStringSubmatch(filePath)
	if matches == nil {
		return "", time.Time{}, false
	}

	created, err := time.Parse("2006-01-02_15-04", matches[2])
	if err != nil {
		return "", time.Time{}, false
	}

	return matches[1], created, true
}

// parseBackupPath extracts the creation time of a copy from its path according
// to the path template of the server. The returned period is the nxs-backup
// retention period of the copy, it is empty for the other templates.
func parseBackupPath(filePath string, server Server) (created time.Time, period string, ok bool) {
	switch server.PathTemplate {
	case Hestia:
		date := hestiaDateExpression.FindString(filePath)
		if parsedTime, err := time.Parse(time.DateOnly, date); err == nil {
			return parsedTime, "", true
		}
	case FilesWithDate:
		date := filesWithDateExpression.FindString(filePath)
		if parsedTime, err := time.Parse("20060102", date); err == nil {
			return parsedTime, "", true
		}
	case PathWithDate:
		date := pathWithDateExpression.FindString(filePath)
		if parsedTime, err := time.Parse("02.01.06", date); err == nil {
			return parsedTime, "", true
		}
	case NxsBackup:
		if parsedPeriod, parsedTime, parsed := parseNxsBackupPath(filePath); parsed {
			return parsedTime, parsedPeriod, true
		}
	}

	return time.Time{}, "", false
}

// nxsBackupRelativePath returns the remote file path relative to the backups
// root of the server, so the remote tree can be mirrored locally.
func nxsBackupRelativePath(server Server, remotePath string) string {
	root := strings.TrimSuffix(path.Clean(server.BackupsPath), "/")
	return strings.TrimPrefix(strings.TrimPrefix(path.Clean(remotePath), root), "/")
}

// retentionDays returns how many days the copies of the given period are kept.
// Falls back to the DaysCount of the server when the period is not configured.
func retentionDays(server Server, period string) int {
	var days int
	switch period {
	case "daily":
		days = server.Retention.Daily
	case "weekly":
		days = server.Retention.Weekly
	case "monthly":
		days = server.Retention.Monthly
	}

	if days <= 0 {
		return server.DaysCount
	}
	return days
}

// retentionCutoff returns the moment before which the copies of the given period expire
func retentionCutoff(server Server, period string) time.Time {
	return time.Now().Add(-(time.Hour * 24 * time.Duration(retentionDays(server, period))))
}

// minCopies returns how many of the newest copies of every series are kept
// regardless of their age
func minCopies(server Server) int {
	if server.MinCopies > 0 {
		return server.MinCopies
	}
	return defaultMinCopies
}

// backupSeries returns the key identifying the series of copies of the same
// backup: the same source of the same server, without the date. The copies are
// protected from deletion per series, otherwise a server with many sources
// would keep only the newest copy of the whole storage.
func backupSeries(filePath string, server Server) string {
	filePath = filepath.ToSlash(filePath)

	// <group>/<source>/<daily|weekly|monthly> already identifies the series
	if server.PathTemplate == NxsBackup {
		return path.Dir(filePath)
	}

	return strings.Trim(seriesDateExpression.ReplaceAllString(path.Base(filePath), ""), "_-.")
}

// classifyCopies parses the given backup paths and marks the copies that are
// past their retention. The newest minCopies of every series are never marked
// as expired: when the source server stops making backups (out of disk space,
// a broken cron and so on), no new copies appear while the existing ones keep
// ageing, and deleting by date alone would silently empty the storage.
func classifyCopies(paths []string, server Server) []backupCopy {
	copies := make([]backupCopy, 0, len(paths))
	series := make(map[string][]int)

	for _, filePath := range paths {
		created, period, ok := parseBackupPath(filePath, server)
		item := backupCopy{path: filePath, parsed: ok}
		if ok {
			item.created = created
			item.expired = created.Before(retentionCutoff(server, period))

			key := backupSeries(filePath, server)
			series[key] = append(series[key], len(copies))
		}
		copies = append(copies, item)
	}

	for _, indexes := range series {
		// The newest copies come first
		sort.SliceStable(indexes, func(i, j int) bool {
			return copies[indexes[i]].created.After(copies[indexes[j]].created)
		})

		for i := 0; i < len(indexes) && i < minCopies(server); i++ {
			if copies[indexes[i]].expired {
				copies[indexes[i]].expired = false
				copies[indexes[i]].protected = true
			}
		}
	}

	return copies
}

func deleteOldFiles(server Server) {
	log.Printf("[%s] Deleting old files from storage \n", server.Name)

	serverStoragePath := strings.TrimSuffix(storagePath, "/") + "/" + server.Name

	if _, err := os.Stat(serverStoragePath); err != nil {
		fmt.Println(err)
		fmt.Println("Skip this server")
		return
	}

	// Walk recursively: the nxsBackup template mirrors the remote tree locally
	var storedFiles []string
	err := filepath.WalkDir(serverStoragePath, func(filePath string, entry os.DirEntry, err error) error {
		if err != nil {
			log.Println(err)
			return nil
		}
		if entry.IsDir() {
			return nil
		}

		// Leftover of a download interrupted by a crash, it will be downloaded again
		if strings.HasSuffix(filePath, partialSuffix) {
			if removeErr := os.Remove(filePath); removeErr != nil {
				log.Println(removeErr)
			} else {
				log.Printf("[%s] Delete partially downloaded file: %s", server.Name, filePath)
			}
			return nil
		}

		relativePath, relErr := filepath.Rel(serverStoragePath, filePath)
		if relErr != nil {
			log.Println(relErr)
			return nil
		}

		storedFiles = append(storedFiles, relativePath)
		return nil
	})
	if err != nil {
		log.Println(err)
		return
	}

	// The retention is applied to the whole storage at once, because whether a
	// copy may be deleted depends on how many newer copies of the same backup exist
	for _, item := range classifyCopies(storedFiles, server) {
		if item.protected {
			log.Printf("[%s] Keep File: %s - the last copies of this backup, no newer ones", server.Name, item.path)
			continue
		}
		if !item.expired {
			log.Printf("[%s] Skip File: %s", server.Name, item.path)
			continue
		}

		// Delete file from storage
		if removeErr := os.Remove(filepath.Join(serverStoragePath, item.path)); removeErr != nil {
			log.Println(removeErr)
			continue
		}
		log.Printf("[%s] Delete File: %s", server.Name, item.path)
	}

	removeEmptyDirs(serverStoragePath)

	log.Printf("[%s] Old files have been deleted", server.Name)
}

// removeEmptyDirs cleans up the directories left empty after the deletion.
// The root directory itself is kept.
func removeEmptyDirs(root string) {
	var dirs []string
	walkErr := filepath.WalkDir(root, func(dirPath string, entry os.DirEntry, err error) error {
		if err == nil && entry.IsDir() && dirPath != root {
			dirs = append(dirs, dirPath)
		}
		return nil
	})
	if walkErr != nil {
		log.Println(walkErr)
		return
	}

	// WalkDir walks in lexical order, so going backwards removes the deepest directories first
	for i := len(dirs) - 1; i >= 0; i-- {
		if entries, err := os.ReadDir(dirs[i]); err == nil && len(entries) == 0 {
			if removeErr := os.Remove(dirs[i]); removeErr != nil {
				log.Println(removeErr)
			}
		}
	}
}

func deleteOldLogs() {
	logPath := storagePath + "logs"
	f, err := os.Open(logPath)
	if err != nil {
		log.Printf("Cannot open logs directory: %v", err)
		return
	}
	defer f.Close()

	files, err := f.Readdir(0)
	if err != nil {
		log.Printf("Cannot read logs directory: %v", err)
		return
	}

	cutoff := time.Now().Add(-(time.Hour * 24 * time.Duration(backuperConfig.LogRetentionDays)))
	for _, file := range files {
		if file.IsDir() || !strings.HasSuffix(file.Name(), ".log") {
			continue
		}
		if file.ModTime().Before(cutoff) {
			err := os.Remove(logPath + "/" + file.Name())
			if err != nil {
				log.Printf("Cannot delete log file %s: %v", file.Name(), err)
			} else {
				log.Printf("Deleted old log file: %s", file.Name())
			}
		}
	}
}
