package main

import (
	"flag"
	"fmt"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/extimsu/urlchecker/version"
)

type Search struct {
	Url      string
	Port     string
	Protocol string
}

func NewSearch(url, port, protocol string) *Search {
	return &Search{
		Url:      url,
		Port:     port,
		Protocol: protocol,
	}
}

func main() {
	url := flag.String("url", "", "a url to checking")
	port := flag.String("port", "80", "a port for checking")
	protocol := flag.String("protocol", "tcp", "a type of protocol")
	versionFlag := flag.Bool("version", false, "Version")
	flag.Parse()

	search := NewSearch(*url, *port, *protocol)

	switch {
	case search.Url == "":
		ShowHelp()
		return
	case *versionFlag:
		version.App()
		return
	}

	var urls []string
	wg := &sync.WaitGroup{}
	mu := sync.Mutex{}
	urls = strings.Split(search.Url, ",")

	for _, url := range urls {
		wg.Add(1)
		go func(url string) {
			mu.Lock()
			defer mu.Unlock()

			fmt.Println(search.Check(url))

			wg.Done()
		}(url)
	}
	wg.Wait()
	fmt.Println("---")
}

func (search *Search) Check(url string) string {
	var (
		port_from_url []string
		address       string
	)
	port_from_url = strings.Split(url, ":")

	if len(port_from_url) != 1 {
		address = port_from_url[0] + ":" + port_from_url[1]
	} else {
		address = url + ":" + search.Port
	}

	timeout := time.Duration(time.Second * 4)
	_, err := net.DialTimeout(search.Protocol, address, timeout)
	if err != nil {
		return fmt.Sprintf("[-] [%v] %v [DOWN]", search.Protocol, address)
	} else {
		return fmt.Sprintf("[+] [UP] [%v] %v", search.Protocol, address)
	}
}

func ShowHelp() {
	fmt.Println(`
	_____________
	< URL-checker >
	 -------------
	`)
	fmt.Println("Usage: urlchecker --url <url>")
	fmt.Println("OR: urlchecker --url <url> --port <port>")
	fmt.Println("")

}
