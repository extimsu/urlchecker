package main

import (
	"fmt"
	"net"
	"testing"
	"time"
)

func TestCheckUrl(t *testing.T) {
	address := "extim.su"
	timeout := time.Duration(time.Second * 4)
	_, err := net.DialTimeout("tcp", address+":80", timeout)
	if err != nil {
		fmt.Printf("[-] %v ", address)
		return
	} else {
		fmt.Printf("[+] %v ", address)
		return
	}
	panic("FAILED")
}
