package main

import (
	"fmt"
	"github.com/pkg/sftp"
	"io"
	"log"
	"os"
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
		log.Fatal(err)
	}
	defer remoteFile.Close()

	file, _ := remoteFile.Stat()

	storagePathForServer := storagePath + server.Name + "/"

	var localFileName string
	if server.PathTemplate == PathWithDate {
		// Add date to filename
		expression := regexp.MustCompile(`(\d{2}).(\d{1,2}).(\d{1,2})`)
		date := expression.FindString(filename)
		localFileName = storagePathForServer + date + "_" + file.Name()
	} else {
		localFileName = storagePathForServer + file.Name()
	}

	if _, err := os.Lstat(localFileName); err == nil {
		log.Printf("[%s] File exists %s. Skip downloading\n", server.Name, file.Name())
		return
	}

	writer, err := os.OpenFile(localFileName, syscall.O_CREAT|syscall.O_WRONLY, 0644)
	if err != nil {
		log.Fatal(err)
		return
	}
	defer writer.Close()

	t1 := time.Now()
	n, err := io.Copy(writer, io.LimitReader(remoteFile, file.Size()))
	if err != nil {
		log.Fatal(err)
		return
	}
	if n != file.Size() {
		log.Fatalf("[%s] copy: expected %v bytes, got %d \n", server.Name, file.Size(), n)
	}

	log.Printf("[%s] Downloaded - %s - %v bytes in %s \n", server.Name, file.Name(), file.Size(), time.Since(t1))
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

	return false
}

func deleteOldFiles(server Server) {
	log.Printf("[%s] Deleting old files from storage \n", server.Name)

	f, err := os.Open(storagePath + server.Name)
	if err != nil {
		fmt.Println(err)
		fmt.Println("Skip this server")
		return
	}

	files, err := f.Readdir(0)
	if err != nil {
		fmt.Println(err)
		return
	}

	for _, file := range files {
		if !file.IsDir() && isOldFile(file.Name(), server) {
			// Delete file from storage
			err := os.Remove(strings.TrimSuffix(storagePath, "/") + "/" + server.Name + "/" + file.Name())
			if err != nil {
				log.Println(err)
				return
			}
			log.Printf("[%s] Delete File: %s", server.Name, file.Name())
		} else {
			log.Printf("[%s] Skip File: %s", server.Name, file.Name())
		}
	}

	log.Printf("[%s] Old files have been deleted", server.Name)
}
