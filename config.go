package main

import (
	"github.com/spf13/viper"
	"log"
	"os"
	"strings"
)

type PathTemplate string

const (
	Hestia        PathTemplate = "hestia"
	FilesWithDate PathTemplate = "filesWithDate"
	PathWithDate  PathTemplate = "pathWithDate"
	NxsBackup     PathTemplate = "nxsBackup"
)

// Retention holds the number of days to keep each kind of nxs-backup copy.
// A zero value falls back to the DaysCount of the server.
type Retention struct {
	Daily   int
	Weekly  int
	Monthly int
}

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
	Retention            Retention
	// MinCopies is the number of the newest copies of every backup kept even
	// when they are past the retention. Defaults to defaultMinCopies.
	MinCopies int
	// SequentialReads reads every file with one request at a time instead of
	// many concurrent ones. Needed when the remote path cannot serve reads out
	// of order, e.g. an FTP share mounted with curlftpfs.
	SequentialReads bool
}

type NewConfig struct {
	StoragePath      string
	StartBackupsHour int
	LogRetentionDays int
	Servers          []Server
}

var storagePath string
var backuperConfig NewConfig

func init() {
	if len(os.Getenv("CONFIG_PATH")) > 0 {
		viper.AddConfigPath(os.Getenv("CONFIG_PATH"))
	} else {
		viper.AddConfigPath(".")
	}
	viper.SetConfigName("config")
	viper.SetConfigType("json")
	viper.SetDefault("logretentiondays", 14)
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
	if strings.HasPrefix(backuperConfig.StoragePath, "/") {
		storagePath = backuperConfig.StoragePath
	} else {
		if cwd, err := os.Getwd(); err == nil {
			storagePath = cwd + string(os.PathSeparator) + backuperConfig.StoragePath
		}
	}
}
