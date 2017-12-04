package main

import (
	"bufio"
	"fmt"
	"net/http"
	"os"

	"github.com/urfave/cli"
)

func initNodeManagerCommand(c *cli.Context) error {

	workDir := getProjectDir(c)

	indexPath := fmt.Sprintf("%s/base/index.txt", workDir)

	force := c.Bool("force")

	if _, err := os.Stat(indexPath); !os.IsNotExist(err) && !force {
		fmt.Println("node-manager already initialized. Skipping. Run with --force to force overwrite.")
		return nil
	}

	err := os.MkdirAll(fmt.Sprintf("%s/images", workDir), os.ModePerm)
	if err != nil {
		return err
	}

	err = os.MkdirAll(fmt.Sprintf("%s/base/images", workDir), os.ModePerm)
	if err != nil {
		return err
	}

	err = downloadIndex("https://cloud.centos.org/centos/7/atomic/images/sha256sum.txt", workDir)

	if err != nil {
		return err
	}

	indexEntries, err := readIndex(workDir)
	if err != nil {
		return err
	}

	latest := indexEntries[len(indexEntries)-1]

	return latest.Download(workDir)
}

func downloadIndex(url, workDir string) error {

	location := fmt.Sprintf("%s/base/index.txt", workDir)
	resp, err := http.Get(url)

	if err != nil {
		return err
	}
	defer resp.Body.Close()
	scanner := bufio.NewScanner(resp.Body)
	f, err := os.Create(location)
	if err != nil {
		return err
	}
	defer f.Close()

	for scanner.Scan() {
		entry := scanner.Text()

		_, err := parseIndexEntry(entry, workDir)
		if err != nil {
			continue
		}
		_, err = fmt.Fprintln(f, entry)
		if err != nil {
			return err
		}
	}
	return scanner.Err()
}
