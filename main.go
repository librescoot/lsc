package main

import (
	"librescoot/lsc/cmd/lsc"
)

var version = "dev"

func main() {
	lsc.Execute(version)
}
