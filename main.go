package main

import (
	"errors"
	"flag"
	"fmt"
	"log"
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
	Timeout  time.Duration
}

func NewSearch(url, port, protocol, t string) (*Search, error) {

	timeout, err := time.ParseDuration(t)
	if err != nil {
		return nil, errors.New("invalid timeout, please check how to use this functional")
	}

	return &Search{
		Url:      url,
		Port:     port,
		Protocol: protocol,
		Timeout:  timeout,
	}, nil
}

func main() {
	url := flag.String("url", "", "a url to checking, ex: example.com")
	port := flag.String("port", "80", "a port for checking, ex: 443")
	protocol := flag.String("protocol", "tcp", "a type of protocol (tcp or udp), ex: udp")
	timeout := flag.String("timeout", "5s", "a timeout for checking in seconds, ex: 3s")
	versionFlag := flag.Bool("version", false, "Version")
	flag.Parse()

	search, err := NewSearch(*url, *port, *protocol, *timeout)
	if err != nil {
		log.Fatal("We can proceed, because of error: ", err)
	}

	switch {
	case *versionFlag:
		version.App()
		return
	case search.Url == "":
		ShowHelp()
		return
	}

	var urls []string
	wg := &sync.WaitGroup{}
	mu := &sync.Mutex{}

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

// Check - checks url address using port number
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

	timeout := search.Timeout
	_, err := net.DialTimeout(search.Protocol, address, timeout)
	if err != nil {
		return fmt.Sprintf("😿 [-] [%v]  %v", search.Protocol, address)
	} else {
		return fmt.Sprintf("😺 [+] [%v]  %v", search.Protocol, address)
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
