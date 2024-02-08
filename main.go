package main

import (
	"flag"
	"fmt"
	"net"
	"time"

	"github.com/extimsu/urlchecker/version"
)

func main() {
	url := flag.String("url", "", "a url to check")
	port := flag.String("port", "80", "a port for check")
	versionFlag := flag.Bool("version", false, "Version")
	flag.Parse()

	if *versionFlag {
		fmt.Println("Build Date:", version.BuildDate)
		fmt.Println("Git Commit:", version.GitCommit)
		fmt.Println("Version:", version.Version)
		fmt.Println("Go Version:", version.GoVersion)
		fmt.Println("OS / Arch:", version.OsArch)
		fmt.Println("Author:", version.Author)
		return
	}

	if *url == "" {
		fmt.Println("Usage: urlchecker --url <name_of_url>")
		fmt.Println("OR: urlchecker --url <name_of_url> --port <port>")
		return
	}

	check := CheckUrl(*url, *port)
	fmt.Println(check)
}

func CheckUrl(url, port string) string {
	address := url + ":" + port
	timeout := time.Duration(time.Second * 4)
	_, err := net.DialTimeout("tcp", address, timeout)
	if err != nil {
		return fmt.Sprintf("[-] %v DEAD", url)
	} else {
		return fmt.Sprintf("[+] %v ALIVE", url)
	}
}
