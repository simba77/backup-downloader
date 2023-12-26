package main

import (
	"fmt"
	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
	"log"
	"path"
	"regexp"
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

func getRemoteFiles(sftpClient *sftp.Client, server Server) []string {
	var remoteFiles []string
	var filesExpression *regexp.Regexp

	if len(server.FilePattern) > 0 {
		filesExpression = regexp.MustCompile(server.FilePattern)
	}

	// walk a directory
	walker := sftpClient.Walk(server.BackupsPath)
	for walker.Step() {
		if walker.Err() != nil {
			log.Println(walker.Err())
			continue
		}

		if walker.Stat().IsDir() || isOldFile(walker.Path(), server) || (filesExpression != nil && !filesExpression.MatchString(walker.Path())) {
			continue
		}

		// Hestia CP path structure. /backup/admin.2023-12-25_05-11-45.tar
		if server.PathTemplate == Hestia {
			// Skip directories and files without .tar extension
			if path.Ext(walker.Path()) != ".tar" {
				continue
			}
			remoteFiles = append(remoteFiles, walker.Path())
		}

		// Files with date /backups/test.20231221.sql.gz
		if server.PathTemplate == FilesWithDate {
			if path.Ext(walker.Path()) != ".gz" {
				continue
			}
			remoteFiles = append(remoteFiles, walker.Path())
		}

		// Files in paths /24.12.23/test.tgz, /24.12.23/test.sql.bz2
		if server.PathTemplate == PathWithDate {
			// TODO: Move extensions to config file
			extension := path.Ext(walker.Path())
			if extension != ".tgz" && extension != ".bz2" {
				continue
			}
			remoteFiles = append(remoteFiles, walker.Path())
		}
	}

	return remoteFiles
}
