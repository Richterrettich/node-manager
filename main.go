package main

import (
	"bufio"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"os/user"
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
			Usage:  "remove node [ID's]",
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
