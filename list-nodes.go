package main

import (
	"fmt"

	libvirt "github.com/libvirt/libvirt-go"
	"github.com/urfave/cli"
)

func listNodesCommand(c *cli.Context) error {
	conn, err := libvirt.NewConnect("qemu:///session")
	if err != nil {
		return err
	}
	fmt.Printf("id\tname\tactive\n")
	return forEachNode(conn, func(dom *libvirt.Domain, name, number string) error {
		active, err := dom.IsActive()
		if err != nil {
			return err
		}
		activeIndicator := "\u2713"
		if !active {
			activeIndicator = "\u2717"
		}
		fmt.Printf("%s\t%s\t%s\n", number, name, activeIndicator)
		return nil
	})
}
