package main

import (
	"fmt"
	"os"
)

func main() { fmt.Fprintln(os.Stderr, "fallo intencional"); os.Exit(1) }
