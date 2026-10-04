package main

import (
	"fmt"
	"os"

	"github.com/bradfordly/bradfordly-games/internal/version"
)

func main() {
	fmt.Println(version.String)
	os.Exit(0)
}
