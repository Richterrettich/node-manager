package main

import (
	"compress/gzip"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"os/user"
	"strconv"
	"sync"

	libvirt "github.com/libvirt/libvirt-go"
	"github.com/urfave/cli"
)

func addNode(c *cli.Context) error {
	conn, err := libvirt.NewConnect("qemu:///session")

	if err != nil {
		log.Fatal(err)
	}
	defer conn.Close()
	workerCount := 1
	err = forEachNode(conn, func(dom *libvirt.Domain, name, nodeNumber string) error {
		number, err := strconv.Atoi(nodeNumber)
		if err != nil {
			return err
		}
		if number >= workerCount {
			workerCount = number + 1
		}
		return nil
	})

	if err != nil {
		log.Fatal(err)
	}

	errors := make(chan error, 2)
	var wg sync.WaitGroup
	wg.Add(2)

	dir := getProjectDir(c)

	nodeDir := fmt.Sprintf("%s/images/atomic-host%d", dir, workerCount)

	err = os.MkdirAll(nodeDir, os.ModePerm)
	if err != nil {
		log.Println("could not create node directory " + nodeDir)
		log.Fatal(err)
	}

	destPath := fmt.Sprintf("%s/image.qcow2", nodeDir)

	go unpackDisk(dir, destPath, &wg, errors)
	go prepareIso(nodeDir, workerCount, &wg, errors)

	wg.Wait()
	close(errors)
	for err := range errors {
		if err != nil {
			return err
		}
	}

	err = run("virt-install",
		"--name", "atomic-host"+strconv.Itoa(workerCount),
		"--ram", "4096",
		"--vcpus", "4",
		"--disk", fmt.Sprintf("path=%s", destPath),
		"--os-type", "linux",
		"--os-variant", "rhel-atomic-7.2",
		"--network", "bridge=bridge0",
		"--network", "default",
		"--virt-type", "kvm",
		"--graphics", fmt.Sprintf("vnc,listen=127.0.0.1,port=590%d", workerCount),
		"--cdrom", fmt.Sprintf("%s/init.iso", nodeDir),
		"--noautoconsole",
	)
	if err != nil {
		//cleanup
		cleanupErr := os.RemoveAll(nodeDir)
		if cleanupErr != nil {
			log.Println("could not clean up:", cleanupErr)
		}
	}
	return err
}

func prepareIso(nodeDir string, workerCount int, wg *sync.WaitGroup, errorChannel chan<- error) {
	defer wg.Done()
	usr, err := user.Current()
	if err != nil {
		errorChannel <- err
		return
	}
	sshPublicKeyFile := fmt.Sprintf("%s/.ssh/id_rsa.pub", usr.HomeDir)
	sshPublicKey, err := ioutil.ReadFile(sshPublicKeyFile)

	userDataFile := fmt.Sprintf("%s/user-data", nodeDir)
	err = writeFile(userDataFile,
		"#cloud-config",
		"password: atomic",
		"ssh_pwauth: True",
		"chpasswd: { expire: False }",
		"ssh_authorized_keys:",
		fmt.Sprintf("  - %s", sshPublicKey),
	)
	if err != nil {
		errorChannel <- err
		return
	}

	metaDataFile := fmt.Sprintf("%s/meta-data", nodeDir)

	err = writeFile(metaDataFile,
		fmt.Sprintf("instance-id: atomic-host%d", workerCount),
		fmt.Sprintf("local-hostname: atomic%d", workerCount),
	)

	if err != nil {
		errorChannel <- err
		return
	}

	isoPath := fmt.Sprintf("%s/init.iso", nodeDir)
	err = run("genisoimage",
		"-output", isoPath,
		"-volid", "cidata", "-joliet", "-rock",
		userDataFile, metaDataFile,
	)
	if err != nil {
		errorChannel <- err
		return
	}
}

func unpackDisk(workDir, destPath string, wg *sync.WaitGroup, errorChannel chan<- error) {
	defer wg.Done()

	entries, err := readIndex(workDir)

	if err != nil {
		errorChannel <- err
		return
	}
	latest := entries[len(entries)-1]
	basePath := fmt.Sprintf("%s/base/images/%s", workDir, latest.fileName)

	f, err := os.Open(basePath)

	if err != nil {
		errorChannel <- err
		return
	}
	defer f.Close()
	gzipReader, err := gzip.NewReader(f)
	if err != nil {
		errorChannel <- err
		return
	}

	errorChannel <- writeToFile(gzipReader, destPath)
}
