package main

import (
	"fmt"
	"github.com/pkg/sftp"
	"io"
	"log"
	"os"
	"regexp"
	"syscall"
	"time"
)

func main() {
	fmt.Println("Started Backuper")

	// Удаляем старые файлы
	deleteOldFiles()

	config, _ := readConfig()
	client, err := Connect(&config)
	if err != nil {
		log.Fatal(err)
	}
	defer CloseConnect()

	files := getRemoteFiles(client, config.BackupPath)

	channel := make(chan string, 100)
	for _, filename := range files {
		// fmt.Println("Path:", filename, "IsOld:", isOldFile(filename))
		go downloadFileFromServer(client, filename, channel)
	}

	for range files {
		fmt.Println(<-channel)
	}
}

func downloadFileFromServer(client *sftp.Client, filename string, channel chan string) {
	// Получаем файл с удаленного сервера
	remoteFile, err := client.Open(filename)
	if err != nil {
		log.Fatal(err)
	}
	defer remoteFile.Close()

	file, _ := remoteFile.Stat()
	// log.Println("Loading file: ", file.Name(), "Size:", file.Size())

	// Проверяем существует ли файл. Если да, то пропускаем
	localFileName := rootPath + "/backups/" + file.Name()
	if _, err := os.Lstat(localFileName); err == nil {
		// log.Println("File exists:", file.Name(), "skip downloading")
		channel <- fmt.Sprintf("File exists %s", file.Name())
		return
	}

	// Открываем файл на запись
	writer, err := os.OpenFile(rootPath+"/backups/"+file.Name(), syscall.O_CREAT|syscall.O_WRONLY, 0644)
	if err != nil {
		log.Fatal(err)
	}
	defer writer.Close()

	t1 := time.Now()
	n, err := io.Copy(writer, io.LimitReader(remoteFile, file.Size()))
	if err != nil {
		log.Fatal(err)
	}
	if n != file.Size() {
		log.Fatalf("copy: expected %v bytes, got %d", file.Size(), n)
	}
	// log.Printf("Downloaded %v bytes in %s", file.Size(), time.Since(t1))

	channel <- fmt.Sprintf("Downloaded %s - %v bytes in %s", file.Name(), file.Size(), time.Since(t1))
}

func isOldFile(filePath string) bool {
	// Получаем дату из названия файла
	expression := regexp.MustCompile(`(\d{4})-(\d{1,2})-(\d{1,2})`)
	date := expression.FindString(filePath)

	if parsedTime, err := time.Parse(time.DateOnly, date); err == nil {
		// Проверяем дату
		// TODO: Вынести в конфиг количество дней в течение которых хранятся файлы
		oldDate := time.Now().Add(-(time.Hour * 24 * 5))
		return parsedTime.Before(oldDate)
	}

	return false
}

func deleteOldFiles() {
	fmt.Println("Deleting old files...")

	backupsDir := rootPath + "/backups"

	f, err := os.Open(backupsDir)
	if err != nil {
		fmt.Println(err)
		return
	}

	files, err := f.Readdir(0)
	if err != nil {
		fmt.Println(err)
		return
	}

	for _, v := range files {
		if !v.IsDir() && isOldFile(v.Name()) {
			// Удаляем файл
			err := os.Remove(backupsDir + "/" + v.Name())
			if err != nil {
				log.Println(err)
				return
			}
			fmt.Println("Delete File:", v.Name())
		} else {
			fmt.Println("Skip File:", v.Name())
		}
	}

	fmt.Println("Old files have been deleted")
}
