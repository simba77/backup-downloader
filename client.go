package main

import (
	"fmt"
	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
	"log"
	"path"
	"regexp"
)

// Archive extensions produced by nxs-backup depending on the configured compression
var nxsBackupExtensions = map[string]bool{
	".gz":  true,
	".tgz": true,
	".tar": true,
	".bz2": true,
	".xz":  true,
	".zst": true,
	".zip": true,
}

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
	var candidates []string
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

		if walker.Stat().IsDir() || (filesExpression != nil && !filesExpression.MatchString(walker.Path())) {
			continue
		}

		// Hestia CP path structure. /backup/admin.2023-12-25_05-11-45.tar
		if server.PathTemplate == Hestia {
			// Skip directories and files without .tar extension
			if path.Ext(walker.Path()) != ".tar" {
				continue
			}
			candidates = append(candidates, walker.Path())
		}

		// Files with date /backups/test.20231221.sql.gz
		if server.PathTemplate == FilesWithDate {
			if path.Ext(walker.Path()) != ".gz" {
				continue
			}
			candidates = append(candidates, walker.Path())
		}

		// Files in paths /24.12.23/test.tgz, /24.12.23/test.sql.bz2
		if server.PathTemplate == PathWithDate {
			// TODO: Move extensions to config file
			extension := path.Ext(walker.Path())
			if extension != ".tgz" && extension != ".bz2" {
				continue
			}
			candidates = append(candidates, walker.Path())
		}

		// nxs-backup path structure. /backups/configs/acme/daily/acme_2026-08-18_02-00.tar.gz
		if server.PathTemplate == NxsBackup {
			// Skip everything that is not a <daily|weekly|monthly>/<name>_<date>.<ext> copy
			if _, _, ok := parseNxsBackupPath(walker.Path()); !ok {
				continue
			}
			if !nxsBackupExtensions[path.Ext(walker.Path())] {
				continue
			}
			// Symlinks (daily copies pointing to the weekly/monthly ones) are kept in
			// the list on purpose: SFTP resolves them on open, so the original file
			// gets downloaded instead of a dangling link.
			candidates = append(candidates, walker.Path())
		}
	}

	// The retention is applied to the whole listing at once: the newest copies of
	// every series are downloaded even when they are past their retention. Without
	// it a server that stopped making backups would have nothing left to download,
	// and the local storage could never recover the copies it deleted by age.
	var remoteFiles []string
	for _, item := range classifyCopies(candidates, server) {
		if item.expired {
			continue
		}
		if item.protected {
			log.Printf("[%s] Downloading an outdated copy %s: there are no newer ones on the server", server.Name, item.path)
		}
		remoteFiles = append(remoteFiles, item.path)
	}

	return remoteFiles
}
