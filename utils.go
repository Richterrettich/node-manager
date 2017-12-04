package main

import (
	"bufio"
	"crypto/sha256"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"os/user"
	"strconv"
	"strings"

	libvirt "github.com/libvirt/libvirt-go"
	"github.com/urfave/cli"
)

const BASE_URL = "http://cloud.centos.org/centos/7/atomic/images/"

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

type nodeCallback func(*libvirt.Domain, string, string) error

type IndexEntry struct {
	version   int
	shaSum    string
	fileName  string
	isPresent bool
}

func (i *IndexEntry) Download(workDir string) error {
	path := fmt.Sprintf("%s/base/images/%s", workDir, i.fileName)
	downloadURL := fmt.Sprintf("%s/%s", BASE_URL, i.fileName)

	resp, err := http.Get(downloadURL)

	if err != nil {
		return err
	}

	defer resp.Body.Close()

	baseImageFile, err := os.Create(path)

	if err != nil {
		return err
	}

	shaSink := sha256.New()
	teeReader := io.TeeReader(resp.Body, shaSink)
	_, err = io.Copy(baseImageFile, teeReader)
	actualSha := shaSink.Sum(nil)
	if string(actualSha) != i.shaSum {
		err := os.Remove(path)
		if err != nil {
			log.Printf("could not delete file %s. Manual cleanup necessary.\n", path)
		}
		return fmt.Errorf("Downloaded has a different sha value then the index suggests.\n This means, someone has tempered with the image.\n actual sha: %s\n expected sha: %s", string(actualSha), i.shaSum)
	}

	return nil
}

func readIndex(workDir string) ([]*IndexEntry, error) {
	location := fmt.Sprintf("%s/base/index.txt", workDir)
	file, err := os.Open(location)
	if err != nil {
		return nil, err
	}
	result := make([]*IndexEntry, 0)
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		entry, err := parseIndexEntry(line, workDir)
		if err != nil {
			return nil, err
		}
		result = append(result, entry)
	}
	if err = scanner.Err(); err != nil {
		return nil, err
	}
	return result, nil
}

func parseIndexEntry(rawLine, workDir string) (*IndexEntry, error) {
	parts := strings.Split(rawLine, " ")
	file := parts[len(parts)-1]
	checksum := parts[0]
	if !strings.HasPrefix(file, "CentOS-Atomic-Host-7.") || !strings.HasSuffix(file, "-GenericCloud.qcow2.gz") {
		return nil, fmt.Errorf("invalid index entry")
	}

	parts = strings.Split(file, ".")
	versionWithSuffix := parts[1]
	version := strings.Split(versionWithSuffix, "-")[0]

	if len(version) != 4 {
		return nil, fmt.Errorf("invalid index entry")
	}

	versionInt, err := strconv.Atoi(version)
	if err != nil {
		return nil, err
	}
	filePath := fmt.Sprintf("%s/base/images/%s", workDir, file)

	_, err = os.Stat(filePath)
	exists := !os.IsNotExist(err)

	return &IndexEntry{
		versionInt, checksum, file, exists,
	}, nil

}

func getVersion(name string) (int, error) {
	parts := strings.Split(name, ".")
	versionWithSuffix := parts[1]
	parts = strings.Split(versionWithSuffix, "-")
	if len(parts) != 2 {
		return -1, fmt.Errorf("invalid version")
	}
	version := parts[0]
	if len(version) != 4 {
		return -1, fmt.Errorf("invalid version")
	}
	return strconv.Atoi(version)
}
