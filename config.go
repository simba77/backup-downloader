package main

import (
	"github.com/spf13/viper"
	"log"
	"os"
)

type PathTemplate string

const (
	Hestia        PathTemplate = "hestia"
	FilesWithDate PathTemplate = "filesWithDate"
	PathWithDate  PathTemplate = "pathWithDate"
)

type Server struct {
	Active               bool
	Name                 string
	Type                 string
	DaysCount            int
	MaxParallelDownloads int
	BackupsPath          string
	Server               string
	User                 string
	Password             string
	Port                 int
	PathTemplate         PathTemplate
	FilePattern          string
}

type NewConfig struct {
	StoragePath string
	Servers     []Server
}

var storagePath string
var backuperConfig NewConfig

func init() {
	viper.AddConfigPath(".")
	viper.SetConfigName("config")
	viper.SetConfigType("json")
	readConfigErr := viper.ReadInConfig()
	if readConfigErr != nil {
		log.Fatalf("Unable to read config file, %v", readConfigErr)
		return
	}

	err := viper.Unmarshal(&backuperConfig)
	if err != nil {
		log.Fatalf("Unable to decode into struct, %v", err)
	}

	// Set the storage path
	if cwd, err := os.Getwd(); err == nil {
		storagePath = cwd + string(os.PathSeparator) + backuperConfig.StoragePath
	}
}
