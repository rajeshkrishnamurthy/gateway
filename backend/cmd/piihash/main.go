package main

import (
	"flag"
	"fmt"
	"gateway/pii"
	"os"
)

var value = flag.String("value", "", "value to hash")

func main() {
	flag.Parse()
	if *value == "" {
		fmt.Fprintln(os.Stderr, "value is required")
		os.Exit(2)
	}
	fmt.Println(pii.Hash(*value))
}
