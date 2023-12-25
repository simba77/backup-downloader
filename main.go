package main

import (
	"fmt"
	"github.com/panjf2000/ants/v2"
	"log"
	"sync"
	"time"
)

func main() {
	log.Println("Started Backuper")

	// for {
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

	// Sleep to wait for connections to be closed
	time.Sleep(time.Second * 2) // TODO: Change the duration to the next night
	//}
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
