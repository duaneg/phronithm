package main

import (
	"fmt"
	"os"
	"time"

	"probe"
)

func main() {
	o, err := probe.Call(os.Args[1], nil, 10*time.Second)
	fmt.Println(o, err)
}
