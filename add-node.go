package main

import (
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"os/user"
	"strconv"

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
		workerCount++
		return nil
	})

	if err != nil {
		log.Fatal(err)
	}

	dir, err := os.Getwd()
	if err != nil {
		panic(err)
	}

	nodeDir := fmt.Sprintf("%s/images/atomic-host%d", dir, workerCount)
	err = os.MkdirAll(nodeDir, os.ModePerm)
	if err != nil {
		log.Println("could not create node directory " + nodeDir)
		log.Fatal(err)
	}

	basePath := fmt.Sprintf("%s/base/CentOS-Atomic-Host-7-GenericCloud.qcow2", dir)
	destPath := fmt.Sprintf("%s/image.qcow2", nodeDir)
	err = cp(basePath, destPath)
	if err != nil {
		log.Println("could not copy base image")
		log.Fatal(err)
	}

	usr, err := user.Current()
	if err != nil {
		log.Fatal(err)
	}
	sshPublicKeyFile := fmt.Sprintf("%s/.ssh/id_rsa.pub", usr.HomeDir)
	sshPublicKey, err := ioutil.ReadFile(sshPublicKeyFile)

	userDataFile := fmt.Sprintf("%s/user-data", nodeDir)
	writeOrFail(userDataFile,
		"#cloud-config",
		"password: atomic",
		"ssh_pwauth: True",
		"chpasswd: { expire: False }",
		"ssh_authorized_keys:",
		fmt.Sprintf("  - %s", sshPublicKey),
	)

	metaDataFile := fmt.Sprintf("%s/meta-data", nodeDir)

	writeOrFail(metaDataFile,
		fmt.Sprintf("instance-id: atomic-host%d", workerCount),
		fmt.Sprintf("local-hostname: atomic%d", workerCount),
	)

	isoPath := fmt.Sprintf("%s/init.iso", nodeDir)
	execOrFail("genisoimage",
		"-output", isoPath,
		"-volid", "cidata", "-joliet", "-rock",
		userDataFile, metaDataFile,
	)
	execOrFail("virt-install",
		"--name", "atomic-host"+strconv.Itoa(workerCount),
		"--ram", "4096",
		"--vcpus", "4",
		"--disk", fmt.Sprintf("path=%s", destPath),
		"--os-type", "linux",
		"--os-variant", "rhel-atomic-7.2",
		"--network", "bridge=bridge0",
		"--network", "default",
		"--graphics", fmt.Sprintf("vnc,listen=127.0.0.1,port=590%d", workerCount),
		"--cdrom", fmt.Sprintf("%s/init.iso", nodeDir),
		"--noautoconsole",
	)
	return nil
}
