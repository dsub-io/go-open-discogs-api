//go:build ignore

package main

import (
	"fmt"
	"net"
)

const loopbackEphemeralAddress = "127.0.0.1:0"

func main() {
	listener, err := net.Listen("tcp", loopbackEphemeralAddress)
	if err != nil {
		panic(err)
	}
	defer listener.Close()
	fmt.Println(listener.Addr().(*net.TCPAddr).Port)
}
