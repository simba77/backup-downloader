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
	"strings"
	"syscall"
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

func downloadFileFromServer(client *sftp.Client, server Server, filename string) {
	log.Printf("[%s] Starting download - %s \n", server.Name, filename)
	remoteFile, err := client.Open(filename)
	if err != nil {
		log.Printf("%v", err)
		return
	}
	defer remoteFile.Close()

	file, _ := remoteFile.Stat()

	storagePathForServer := storagePath + server.Name + "/"

	var localFileName string
	switch server.PathTemplate {
	case PathWithDate:
		// Add date to filename
		expression := regexp.MustCompile(`(\d{2}).(\d{1,2}).(\d{1,2})`)
		date := expression.FindString(filename)
		localFileName = storagePathForServer + date + "_" + file.Name()
	case NxsBackup:
		// Mirror the remote tree, otherwise the daily/weekly/monthly copies
		// of the same backup would collide by name
		localFileName = storagePathForServer + nxsBackupRelativePath(server, filename)
	default:
		localFileName = storagePathForServer + file.Name()
	}

	if err := os.MkdirAll(filepath.Dir(localFileName), 0755); err != nil {
		log.Println(err)
		return
	}

	if _, err := os.Lstat(localFileName); err == nil {
		log.Printf("[%s] File exists %s. Skip downloading\n", server.Name, file.Name())
		return
	}

	writer, err := os.OpenFile(localFileName, syscall.O_CREAT|syscall.O_WRONLY, 0644)
	if err != nil {
		log.Println(err)
		return
	}
	defer writer.Close()

	t1 := time.Now()
	n, err := io.Copy(writer, remoteFile)
	if err != nil {
		log.Println(err)
		return
	}
	if n != file.Size() {
		log.Printf("[%s] copy: expected %v bytes, got %d \n", server.Name, file.Size(), n)
	}

	log.Printf("[%s] Downloaded - %s - %v bytes in %s \n", server.Name, file.Name(), file.Size(), time.Since(t1))
}

// nxs-backup stores copies as <group>/<source>/<daily|weekly|monthly>/<name>_2026-08-18_02-00.<ext>
var nxsBackupPathExpression = regexp.MustCompile(`(?:^|/)(daily|weekly|monthly)/[^/]+_(\d{4}-\d{2}-\d{2}_\d{2}-\d{2})\.`)

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

func isOldFile(filePath string, server Server) bool {
	if server.PathTemplate == Hestia {
		// Get date from file path
		expression := regexp.MustCompile(`(\d{4})-(\d{1,2})-(\d{1,2})`)
		date := expression.FindString(filePath)

		if parsedTime, err := time.Parse(time.DateOnly, date); err == nil {
			// Check the date
			oldDate := time.Now().Add(-(time.Hour * 24 * time.Duration(server.DaysCount)))
			return parsedTime.Before(oldDate)
		}
	}

	if server.PathTemplate == FilesWithDate {
		// Get date from file path
		expression := regexp.MustCompile(`(\d{8})`)
		date := expression.FindString(filePath)
		if parsedTime, err := time.Parse("20060102", date); err == nil {
			// Check the date
			oldDate := time.Now().Add(-(time.Hour * 24 * time.Duration(server.DaysCount)))
			return parsedTime.Before(oldDate)
		}
	}

	if server.PathTemplate == PathWithDate {
		expression := regexp.MustCompile(`(\d{2}).(\d{1,2}).(\d{1,2})`)
		date := expression.FindString(filePath)
		if parsedTime, err := time.Parse("02.01.06", date); err == nil {
			// Check the date
			oldDate := time.Now().Add(-(time.Hour * 24 * time.Duration(server.DaysCount)))
			return parsedTime.Before(oldDate)
		}
	}

	if server.PathTemplate == NxsBackup {
		if period, created, ok := parseNxsBackupPath(filePath); ok {
			oldDate := time.Now().Add(-(time.Hour * 24 * time.Duration(retentionDays(server, period))))
			return created.Before(oldDate)
		}
	}

	return false
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
	err := filepath.WalkDir(serverStoragePath, func(filePath string, entry os.DirEntry, err error) error {
		if err != nil {
			log.Println(err)
			return nil
		}
		if entry.IsDir() {
			return nil
		}

		relativePath, relErr := filepath.Rel(serverStoragePath, filePath)
		if relErr != nil {
			log.Println(relErr)
			return nil
		}

		if !isOldFile(relativePath, server) {
			log.Printf("[%s] Skip File: %s", server.Name, relativePath)
			return nil
		}

		// Delete file from storage
		if removeErr := os.Remove(filePath); removeErr != nil {
			log.Println(removeErr)
			return nil
		}
		log.Printf("[%s] Delete File: %s", server.Name, relativePath)
		return nil
	})
	if err != nil {
		log.Println(err)
		return
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
