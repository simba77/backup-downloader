package main

import (
	"fmt"
	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
	"log"
	"path"
)

var connection *ssh.Client
var client *sftp.Client

func Connect(config *Config) (*sftp.Client, error) {

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

	if conn, err := ssh.Dial("tcp", addr, &configClient); err == nil {
		connection = conn
	} else {
		log.Fatalf("unable to connect to [%s]: %v", addr, err)
		return nil, fmt.Errorf("unable to connect to [%s]: %v", addr, err)
	}

	// open an SFTP session over an existing ssh connection.
	if sftpClient, err := sftp.NewClient(connection); err == nil {
		client = sftpClient
		return client, nil
	} else {
		log.Fatal(err)
		return nil, err
	}
}

func CloseConnect() {
	connection.Close()
	client.Close()
	fmt.Println("Close connection")
}

func getRemoteFiles(sftpClient *sftp.Client, backupPath string) []string {
	var remoteFiles []string
	// walk a directory
	w := sftpClient.Walk(backupPath)
	for w.Step() {
		if w.Err() != nil {
			fmt.Println(w.Err())
			continue
		}

		// Skip directories and files without .tar extension
		if w.Stat().IsDir() || path.Ext(w.Path()) != ".tar" {
			continue
		}

		remoteFiles = append(remoteFiles, w.Path())
		// downloadFileFromServer(&*client, w.Path())
	}

	return remoteFiles
}
