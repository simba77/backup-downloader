package main

import (
	"fmt"
	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
	"log"
	"path"
)

var connection map[string]*ssh.Client
var client map[string]*sftp.Client

func init() {
	connection = make(map[string]*ssh.Client)
	client = make(map[string]*sftp.Client)
}

func Connect(config Server) (*sftp.Client, error) {
	log.Printf("[%s] Connectiong to server \n", config.Name)
	var auths []ssh.AuthMethod

	/*	if aconn, err := net.Dial("unix", os.Getenv("SSH_AUTH_SOCK")); err == nil {
		auths = append(auths, ssh.PublicKeysCallback(agent.NewClient(aconn).Signers))
	}*/

	auths = append(auths, ssh.Password(config.Password))

	configClient := ssh.ClientConfig{
		User:            config.User,
		Auth:            auths,
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
	}

	addr := fmt.Sprintf("%s:%d", config.Server, config.Port)

	if conn, err := ssh.Dial("tcp", addr, &configClient); err == nil {
		connection[config.Name] = conn
	} else {
		log.Fatalf("unable to connect to [%s]: %v", addr, err)
		return nil, fmt.Errorf("unable to connect to [%s]: %v", addr, err)
	}

	// open an SFTP session over an existing ssh connection.
	if sftpClient, err := sftp.NewClient(connection[config.Name]); err == nil {
		client[config.Name] = sftpClient
		return client[config.Name], nil
	} else {
		log.Fatal(err)
		return nil, err
	}
}

func CloseConnect(config Server) {
	connection[config.Name].Close()
	client[config.Name].Close()

	// Удаляем значения из карты
	delete(connection, config.Name)
	delete(client, config.Name)

	log.Printf("[%s] Close connection\n", config.Name)
}

func getRemoteFiles(sftpClient *sftp.Client, backupPath string) []string {
	var remoteFiles []string
	// walk a directory
	w := sftpClient.Walk(backupPath)
	for w.Step() {
		if w.Err() != nil {
			log.Println(w.Err())
			continue
		}

		// Skip directories and files without .tar extension
		if w.Stat().IsDir() || path.Ext(w.Path()) != ".tar" {
			continue
		}

		remoteFiles = append(remoteFiles, w.Path())
	}

	return remoteFiles
}
