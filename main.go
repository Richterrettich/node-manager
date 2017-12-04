package main

import (
	"log"
	"os"

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
