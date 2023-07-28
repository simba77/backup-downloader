package main

import (
	"encoding/json"
	"fmt"
	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
	"io"
	"log"
	"os"
	"path"
	"regexp"
	"syscall"
	"time"
)

var configPath string
var rootPath string

func main() {
	fmt.Println("Started Backuper")

	// Устанавливаем пути к корневой директории и конфигу
	if cwd, err := os.Getwd(); err == nil {
		rootPath = cwd + string(os.PathSeparator)
		configPath = rootPath + "config.json"
	}

	// Удаляем старые файлы
	deleteOldFiles()

	// Читаем конфиг
	config, _ := readConfig()

	var auths []ssh.AuthMethod

	/*	if aconn, err := net.Dial("unix", os.Getenv("SSH_AUTH_SOCK")); err == nil {
		auths = append(auths, ssh.PublicKeysCallback(agent.NewClient(aconn).Signers))
	}*/

	auths = append(auths, ssh.Password(config.SFTP.Password))

	configClient := ssh.ClientConfig{
		User:            config.SFTP.User,
		Auth:            auths,
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
	}
	addr := fmt.Sprintf("%s:%d", config.SFTP.Server, config.SFTP.Port)
	conn, err := ssh.Dial("tcp", addr, &configClient)
	if err != nil {
		log.Fatalf("unable to connect to [%s]: %v", addr, err)
	}
	defer conn.Close()

	// open an SFTP session over an existing ssh connection.
	client, err := sftp.NewClient(conn)
	if err != nil {
		log.Fatal(err)
	}
	defer client.Close()

	// walk a directory
	w := client.Walk(config.BackupPath)
	for w.Step() {
		if w.Err() != nil {
			fmt.Println(w.Err())
			continue
		}

		// Skip directories and files without .tar extension
		if w.Stat().IsDir() || path.Ext(w.Path()) != ".tar" {
			continue
		}

		downloadFileFromServer(&*client, w.Path())

		// fmt.Println("Path:", w.Path(), "IsOld:", isOldFile(w.Path()))
	}
}

type Config struct {
	BackupPath string
	SFTP       struct {
		Server   string
		User     string
		Password string
		Port     int
	}
}

func readConfig() (config Config, err error) {
	var m Config
	file, err := os.ReadFile(configPath)
	if err != nil {
		log.Println(err)
		return m, err
	}

	jsonerr := json.Unmarshal(file, &m)
	if jsonerr != nil {
		log.Println(err)
		return m, err
	}

	return m, err
}

func downloadFileFromServer(client *sftp.Client, filename string) {
	// Получаем файл с удаленного сервера
	remoteFile, err := client.Open(filename)
	if err != nil {
		log.Fatal(err)
	}
	defer remoteFile.Close()

	file, _ := remoteFile.Stat()
	log.Println("Loading file: ", file.Name(), "Size:", file.Size())

	// Проверяем существует ли файл. Если да, то пропускаем
	localFileName := rootPath + "/backups/" + file.Name()
	if _, err := os.Lstat(localFileName); err == nil {
		log.Println("File exists:", file.Name(), "skip downloading")
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
	log.Printf("Downloaded %v bytes in %s", file.Size(), time.Since(t1))
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
