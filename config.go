package main

import (
	"encoding/json"
	"log"
	"os"
)

type Config struct {
	BackupPath string
	SFTP       struct {
		Server   string
		User     string
		Password string
		Port     int
	}
}

var configPath string
var rootPath string

func init() {
	// Устанавливаем пути к корневой директории и конфигу
	if cwd, err := os.Getwd(); err == nil {
		rootPath = cwd + string(os.PathSeparator)
		configPath = rootPath + "config.json"
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
