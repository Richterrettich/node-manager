package main

import (
	"bufio"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"os/user"
	"strconv"
	"strings"

	libvirt "github.com/libvirt/libvirt-go"
	"github.com/urfave/cli"
)

func main() {

	app := cli.NewApp()

	app.Name = "node-manager"

	app.Commands = []cli.Command{
		{
			Name:   "add",
			Usage:  "add a new node",
			Action: addNode,
		},
		{
			Name:   "rm",
			Usage:  "remove node [ID]",
			Action: removeNodeCommand,
		},
	}

	app.Flags = []cli.Flag{
		cli.StringFlag{
			Name:  "dir, d",
			Usage: "Specify the working directory. Defaults to $PWD/kvm.",
		},
	}

	err := app.Run(os.Args)

	if err != nil {
		log.Fatal(err)
	}

}

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
	err := dom.Destroy()
	if err != nil {
		return err
	}

	err = dom.Undefine()
	if err != nil {
		return err
	}
	nodePath := fmt.Sprintf("%s/images/%s", workDir, name)
	return os.RemoveAll(nodePath)
}

func getProjectDir(c *cli.Context) string {
	dir := c.String("dir")
	if dir == "" {
		usr, err := user.Current()
		if err != nil {
			log.Println("could not determine working directory")
			log.Fatal(err)
		}
		dir = fmt.Sprintf("%s/.local/share/node-manager", usr.HomeDir)
	}

	err := os.MkdirAll(dir, os.ModePerm)
	if err != nil {
		log.Println("could not create working directory")
		log.Fatal(err)
	}
	return dir
}

func removeAllNodes(workingDirectory string, conn *libvirt.Connect) error {
	return forEachNode(conn, func(dom *libvirt.Domain, name, nodeNumber string) error {
		return removeNode(dom, name, workingDirectory)
	})
}

func addNode(c *cli.Context) error {
	conn, err := libvirt.NewConnect("qemu:///session")

	if err != nil {
		log.Fatal(err)
	}
	defer conn.Close()

	workerCount := 1
	err = forEachNode(conn, func(dom *libvirt.Domain, name, nodeNumber string) error {
		if strings.HasPrefix(name, "atomic-host") {
			workerCount++
		}
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
		"--graphics", fmt.Sprintf("vnc,listen=127.0.0.1,port=590%d", workerCount),
		"--cdrom", fmt.Sprintf("%s/init.iso", nodeDir),
		"--noautoconsole",
	)
	return nil
}

func execOrFail(cmdStr string, args ...string) {
	cmd := exec.Command(cmdStr, args...)
	stdoutAndErr, err := cmd.CombinedOutput()
	fmt.Printf("%s\n", stdoutAndErr)
	if err != nil {
		log.Fatal(err)
	}
}

func writeOrFail(location string, lines ...string) {
	f, err := os.Create(location)
	if err != nil {
		log.Fatal(err)
	}

	w := bufio.NewWriter(f)

	for _, line := range lines {
		fmt.Fprintln(w, line)
	}

	err = w.Flush()
	if err != nil {
		log.Fatal(err)
	}
}

func forEachNode(conn *libvirt.Connect, callback nodeCallback) error {
	domains, err := conn.ListAllDomains(libvirt.CONNECT_LIST_DOMAINS_ACTIVE | libvirt.CONNECT_LIST_DOMAINS_INACTIVE)
	if err != nil {
		return err
	}
	for _, dom := range domains {
		defer dom.Free()
		name, err := dom.GetName()
		if err != nil {
			return err
		}

		if strings.HasPrefix(name, "atomic-host") {
			nodeNumber := strings.TrimLeft(name, "atomic-host")
			nodeName := fmt.Sprintf("atomic-host%s", nodeNumber)
			err = callback(&dom, nodeName, nodeNumber)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

func cp(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, in)
	if err != nil {
		return err
	}
	return out.Close()
}

type nodeCallback func(*libvirt.Domain, string, string) error
