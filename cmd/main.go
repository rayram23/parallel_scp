package main

import (
	"bufio"
	"fmt"
	"io/ioutil"
	"os"
	"strconv"
	"strings"
	"sync"

	"golang.org/x/crypto/ssh"

	"github.com/howeyc/gopass"
	"github.com/ianschenck/envflag"
)

var (
	host        = envflag.String("HOST", "localhost", "The remote host to connect to")
	user        = envflag.String("USER", "root", "The user to connect with")
	localPath   = envflag.String("LOCAL_PATH", ".", "The path to save the data to")
	remotePath  = envflag.String("REMOTE_PATH", "", "The remote path to read data from")
	maxRoutines = envflag.Int("MAX_ROUTINES", 2, "The max number of go routines to run")
	recursive   = envflag.Bool("RECURSIVE", true, "Whether to read recursively")
	password    string
)

func main() {
	envflag.Parse()
	fmt.Print("Password: ")
	password = string(gopass.GetPasswd())

	fmt.Printf("Connecting\n")
	client, err := connect(*user, password, *host)
	if err != nil {
		fmt.Printf("Error: %s\n", err)
		return
	}
	defer client.Close()
	fmt.Print("Connected successfully\n")
	count, err := fileCount(client, *remotePath)
	if err != nil {
		fmt.Printf("error: %s\n", err)
		return
	}

	fileList := make([]string, count)
	err = listFiles(client, *remotePath, fileList)
	if err != nil {
		fmt.Printf("Error %s\n", err)
		return
	}

	fmt.Printf("file count %d\n", count)
	// start a worker pool
	var wg sync.WaitGroup
	sem := make(chan bool, *maxRoutines)
	for _, v := range fileList {
		sem <- true
		wg.Add(1)
		fmt.Printf("Processing file %s\n", v)
		go downloadFile(v, sem, wg)
	}
	wg.Wait()
}
func downloadFile(file string, sem chan bool, wg sync.WaitGroup) {
	defer func() {
		_ = <-sem
		wg.Done()
	}()
	client, err := connect(*user, password, *host)
	if err != nil {
		fmt.Printf("Error: %s\n", err)
		return
	}
	defer client.Close()
	session, err := client.NewSession()
	if err != nil {
		fmt.Printf("unable to start session for file %s\n", file)
		return
	}
	defer session.Close()
	cmd := fmt.Sprintf("cat %s/%s", *remotePath, file)
	data, err := session.Output(cmd)
	if err != nil {
		fmt.Printf("unable to download file %s %s\n", cmd, err)
		return
	}
	fileName := fmt.Sprintf("%s/%s", *localPath, file)
	if err = ioutil.WriteFile(fileName, data, os.ModePerm); err != nil {
		fmt.Printf("could not save file %s %s \n", fileName, err)
	}
	fmt.Printf("completed %s\n", file)
}
func listFiles(client *ssh.Client, dir string, buff []string) error {
	session, err := client.NewSession()
	if err != nil {
		return err
	}
	cmd := fmt.Sprintf(`find %s -type f -printf "%s\n"`, dir, "%f")
	r, err := session.StdoutPipe()
	err = session.Run(cmd)
	if err != nil {
		return err
	}
	reader := bufio.NewReader(r)
	for i := 0; i < len(buff); i++ {
		line, _, err := reader.ReadLine()
		if err != nil {
			return err
		}
		buff[i] = string(line)
	}
	return nil
}
func fileCount(client *ssh.Client, dir string) (int64, error) {
	session, err := client.NewSession()
	if err != nil {
		return 0, err
	}
	r, err := session.StdoutPipe()
	if err != nil {
		return 0, err
	}
	err = session.Run(fmt.Sprintf(`find  %s -type f -printf "%s\n"| wc -l;`, dir, "%f"))
	if err != nil {
		return 0, err
	}
	buff := make([]byte, 32)
	_, err = r.Read(buff)
	if err != nil {
		return 0, err
	}
	data := string(buff)
	data = strings.Trim(data, "\x00")
	data = strings.TrimSpace(data)
	return strconv.ParseInt(data, 10, 32)
}
func connect(user, password, host string) (client *ssh.Client, err error) {
	clientConfig := &ssh.ClientConfig{
		User: user,
		Auth: []ssh.AuthMethod{
			ssh.Password(password),
		},
	}
	client, err = ssh.Dial("tcp", fmt.Sprintf("%s:22", host), clientConfig)
	if err != nil {
		return nil, err
	}
	return
}
