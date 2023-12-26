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

	for {
		file, err := openLogFile()
		if err != nil {
			log.Fatal(err)
		}
		log.SetOutput(file)
		log.SetFlags(log.LstdFlags | log.Lshortfile | log.Lmicroseconds)

		log.Println("Started Backuper")

		// Delete old files
		for _, server := range backuperConfig.Servers {
			if !server.Active {
				continue
			}
			checkAndCreateStorageDirectory(server)
			deleteOldFiles(server)
		}

		channel := make(chan string, 100)
		for _, server := range backuperConfig.Servers {
			if !server.Active {
				continue
			}
			go downloadBackupsForServer(server, channel)
		}

		for _, server := range backuperConfig.Servers {
			if !server.Active {
				continue
			}
			log.Println(<-channel)
		}

		// Sleep to wait for next time to create a backup
		current := time.Now()
		// TODO: Move the date to config
		startBackupDate := time.Date(current.Year(), current.Month(), current.Day()+1, 6, 0, 0, 0, time.Local)
		toBackup := time.Until(startBackupDate)
		time.Sleep(toBackup)
	}
}

func downloadBackupsForServer(server Server, channel chan<- string) {
	var wg sync.WaitGroup
	client, err := Connect(server)
	if err != nil {
		log.Fatal(err)
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

	channel <- fmt.Sprintf("[%s] All files have been downloaded", server.Name)
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
