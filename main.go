package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/extimsu/urlchecker/help"
	"github.com/extimsu/urlchecker/version"
)

type Search struct {
	Url      string
	Port     string
	Protocol string
	Timeout  time.Duration
	SearchResult
}

type SearchResult struct {
	Address string `json:"address"`
	Port    string `json:"port"`
	State   string `json:"state"`
}

// New initializes the Search struct
func New(url, port, protocol, t string) (*Search, error) {

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

func importFromFile(filename string) ([]string, error) {
	f, err := os.Open(filename)
	if err != nil {
		return nil, errors.New("Cannot open file: " + filename)
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	lines := make([]string, 0)
	for scanner.Scan() {
		line := scanner.Text()
		lines = append(lines, line)
	}

	if scanner.Err() != nil {
		return nil, scanner.Err()
	}

	return lines, nil
}

func main() {
	url := flag.String("url", "", "a url to checking, ex: example.com")
	port := flag.String("port", "80", "a port for checking, ex: 443")
	protocol := flag.String("protocol", "tcp", "a type of protocol (tcp or udp), ex: udp")
	timeout := flag.String("timeout", "5s", "a timeout for checking in seconds, ex: 3s")
	listFromFile := flag.String("file", "", "Import urls from file, ex: urls.txt")
	jsonOutput := flag.Bool("json", false, "JSON output")
	versionFlag := flag.Bool("version", false, "Version")
	flag.Parse()

	search, err := New(*url, *port, *protocol, *timeout)

	if err != nil {
		log.Fatal("We can proceed, because of error: ", err)
	}

	var (
		urls []string
		wg   sync.WaitGroup
		mu   sync.Mutex
	)

	switch {
	case *versionFlag:
		version.App()
		return
	case *listFromFile != "":
		urls, err = importFromFile(*listFromFile)
		if err != nil {
			log.Fatal(err)
		}

	case search.Url != "":
		urls = strings.Split(search.Url, ",")

	default:
		help.Show()
		return
	}

	for _, url := range urls {
		wg.Add(1)
		go func(url string) {
			mu.Lock()
			defer mu.Unlock()

			resultText := search.Check(url)

			if *jsonOutput {
				result := &SearchResult{
					Address: search.SearchResult.Address,
					Port:    search.SearchResult.Port,
					State:   search.SearchResult.State,
				}

				resultJson, err := json.Marshal(*result)
				if err != nil {
					fmt.Println("Error:", err)
				}
				fmt.Println(string(resultJson))
			} else {
				fmt.Println(resultText)
			}

			wg.Done()
		}(url)
	}
	wg.Wait()
}

// Check - checks url address using port number
func (search *Search) Check(url string) string {

	var port_from_url []string = strings.Split(url, ":")

	if len(port_from_url) != 1 {
		search.SearchResult.Address = port_from_url[0]
		search.SearchResult.Port = port_from_url[1]
	} else {
		search.SearchResult.Address = url
		search.SearchResult.Port = search.Port
	}

	addr := search.SearchResult.Address + ":" + search.SearchResult.Port
	timeout := search.Timeout
	_, err := net.DialTimeout(search.Protocol, addr, timeout)
	if err != nil {
		search.SearchResult.State = "Failed"
		return fmt.Sprintf("😿 [-] [%v]  %v", search.Protocol, addr)
	} else {
		search.SearchResult.State = "Success"
		return fmt.Sprintf("😺 [+] [%v]  %v", search.Protocol, addr)
	}
}
