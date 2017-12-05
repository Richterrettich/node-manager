package main

import (
	"fmt"
	"log"
	"os"

	libvirt "github.com/libvirt/libvirt-go"
	"github.com/urfave/cli"
)

func removeNodeCommand(c *cli.Context) error {
	argsPresent := c.Args().Present()
	workingDirectory := getProjectDir(c)

	conn, err := libvirt.NewConnect("qemu:///session")

	if err != nil {
		log.Fatal(err)
	}
	defer conn.Close()

	if !argsPresent {
		return removeAllNodes(workingDirectory, conn)
	}

	i := 0
	arg := c.Args().Get(i)
	nodeNumbers := make(map[string]bool)
	for arg != "" {
		nodeNumbers[arg] = true
	}
	return removeNodes(conn, workingDirectory, nodeNumbers)
}

func removeNodes(conn *libvirt.Connect, workDir string, nodeNumbers map[string]bool) error {
	return forEachNode(conn, func(dom *libvirt.Domain, name, nodeNumber string) error {
		if _, ok := nodeNumbers[nodeNumber]; !ok {
			return nil
		}
		return removeNode(dom, name, workDir)
	})
}

func removeNode(dom *libvirt.Domain, name, workDir string) error {
	active, err := dom.IsActive()
	if err != nil {
		return err
	}
	if active {
		err := dom.Destroy()
		if err != nil {
			return err
		}
	}

	fmt.Printf("undefining %s\n", name)
	err = dom.UndefineFlags(libvirt.DOMAIN_UNDEFINE_MANAGED_SAVE)
	if err != nil {
		return err
	}
	nodePath := fmt.Sprintf("%s/images/%s", workDir, name)
	return os.RemoveAll(nodePath)
}

func removeAllNodes(workingDirectory string, conn *libvirt.Connect) error {
	return forEachNode(conn, func(dom *libvirt.Domain, name, nodeNumber string) error {
		return removeNode(dom, name, workingDirectory)
	})
}
