package main

import (
	"fmt"
	"github.com/panjf2000/ants/v2"
	"github.com/pkg/sftp"
	"io"
	"log"
	"os"
	"regexp"
	"sync"
	"syscall"
	"time"
)

func main() {
	log.Println("Started Backuper")

	// Delete old files
	for _, server := range backuperConfig.Servers {
		deleteOldFiles(server)
	}

	channel := make(chan string, 100)
	for _, server := range backuperConfig.Servers {
		go downloadBackupsForServer(server, channel)
	}

	for range backuperConfig.Servers {
		log.Println(<-channel)
	}

	// Sleep to wait for connections to be closed
	time.Sleep(time.Second * 2)
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

	files := getRemoteFiles(client, server.BackupsPath)
	for _, filename := range files {
		wg.Add(1)
		_ = poolWithFunc.Invoke(filename)
	}

	wg.Wait()

	channel <- fmt.Sprintf("[%s] All files have been downloaded", server.Name)
}

func downloadFileFromServer(client *sftp.Client, server Server, filename string) {
	log.Printf("[%s] Starting download - %s \n", server.Name, filename)
	remoteFile, err := client.Open(filename)
	if err != nil {
		log.Fatal(err)
	}
	defer remoteFile.Close()

	file, _ := remoteFile.Stat()

	// Check dir and file existence
	storagePathForServer := storagePath + server.Name + "/"
	if _, err := os.Stat(storagePathForServer); os.IsNotExist(err) {
		err := os.Mkdir(storagePathForServer, 0755)
		if err != nil {
			log.Fatal(err)
			return
		}
	}

	localFileName := storagePathForServer + file.Name()
	if _, err := os.Lstat(localFileName); err == nil {
		log.Printf("[%s] File exists %s. Skip downloading\n", server.Name, file.Name())
		return
	}

	// Открываем файл на запись
	writer, err := os.OpenFile(storagePathForServer+file.Name(), syscall.O_CREAT|syscall.O_WRONLY, 0644)
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

func isOldFile(filePath string, daysCount time.Duration) bool {
	// Получаем дату из названия файла
	expression := regexp.MustCompile(`(\d{4})-(\d{1,2})-(\d{1,2})`)
	date := expression.FindString(filePath)

	if parsedTime, err := time.Parse(time.DateOnly, date); err == nil {
		// Проверяем дату
		oldDate := time.Now().Add(-(time.Hour * 24 * daysCount))
		return parsedTime.Before(oldDate)
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
		if !file.IsDir() && isOldFile(file.Name(), 5) {
			// Delete file from storage
			err := os.Remove(storagePath + "/" + file.Name())
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
