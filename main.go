package main

import (
	"fmt"
	"github.com/panjf2000/ants/v2"
	"log"
	"os"
	"sync"
	"time"
)

func main() {
	fmt.Println("Started Backuper")
	var wg sync.WaitGroup

	for {
		file, err := openLogFile()
		if err != nil {
			log.Fatal(err)
		}
		log.SetOutput(file)
		log.SetFlags(log.LstdFlags | log.Lshortfile | log.Lmicroseconds)

		log.Println("Started Backuper")

		current := time.Now()
		startBackupDate := time.Date(current.Year(), current.Month(), current.Day()+1, backuperConfig.StartBackupsHour, 0, 0, 0, time.Local)
		// startBackupDate := time.Now().Add(time.Second * 30)
		log.Printf("The next backup is scheduled for %v", startBackupDate.Format("02.01.2006 15:04:05"))

		// Delete old files
		for _, server := range backuperConfig.Servers {
			if !server.Active {
				continue
			}
			checkAndCreateStorageDirectory(server)
			deleteOldFiles(server)
		}

		pool, _ := ants.NewPoolWithFunc(100, func(i interface{}) {
			downloadBackupsForServer(i.(Server))
			wg.Done()
		})

		for _, server := range backuperConfig.Servers {
			if !server.Active {
				continue
			}

			wg.Add(1)
			_ = pool.Invoke(server)
		}

		wg.Wait()
		pool.Release()

		// Sleep to wait for next time to create a backup
		if startBackupDate.After(time.Now()) {
			log.Printf("Sleep until: %v", startBackupDate)
			toBackup := time.Until(startBackupDate)
			time.Sleep(toBackup)
		} else {
			log.Printf("Start the next backup without sleep %v", startBackupDate)
		}

		closeError := file.Close()
		if closeError != nil {
			log.Printf("%v", closeError)
			return
		}
	}
}

func downloadBackupsForServer(server Server) {
	var wg sync.WaitGroup
	client, err := Connect(server)
	if err != nil {
		log.Printf("Connection error: %v", err)
		return
	}
	defer CloseConnect(server)

	poolWithFunc, _ := ants.NewPoolWithFunc(server.MaxParallelDownloads, func(filename interface{}) {
		downloadFileFromServer(client, server, filename.(string))
		wg.Done()
	})
	defer poolWithFunc.Release()

	files := getRemoteFiles(client, server)
	for _, filename := range files {
		wg.Add(1)
		_ = poolWithFunc.Invoke(filename)
	}

	wg.Wait()

	log.Printf("[%s] All files have been downloaded", server.Name)
}

func openLogFile() (*os.File, error) {
	logPath := storagePath + "logs"
	if _, err := os.Stat(logPath); os.IsNotExist(err) {
		err := os.Mkdir(logPath, 0755)
		if err != nil {
			log.Fatal(err)
			return nil, nil
		}
	}

	path := logPath + "/log-" + time.Now().Format("02-01-2006") + ".log"

	logFile, err := os.OpenFile(path, os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0644)
	if err != nil {
		return nil, err
	}
	return logFile, nil
}
