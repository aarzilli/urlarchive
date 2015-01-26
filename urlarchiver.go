package main

import (
	"code.google.com/p/gosqlite/sqlite"
	"flag"
	"fmt"
	"os"
)

const MAX_DIFFS = 20
const MINGAIN = 0.80

var fullStoreFlag = false

type DecoratedChange struct {
	A, Ins, Del int
	InsText     []byte
}

func must(err error) {
	if err != nil {
		panic(err)
	}
}

func usage() {
	fmt.Fprintf(os.Stderr, "Usage: urlarchive [-f] [<archive db>] [serve|update]\n")
	fmt.Fprintf(os.Stderr, "\t-f\tRetrieves images and linked stylesheets too\n")
	os.Exit(1)
}

func parseCmd(args []string) (string, []string) {
	switch args[0] {
	case "serve":
		fallthrough
	case "update":
		return os.ExpandEnv("$HOME/.config/urlarchive/ua.sqlite"), args
	default:
		return args[0], args[1:]
	}
}

func main() {
	flag.BoolVar(&fullStoreFlag, "f", false, "Retrieves linked stylesheets and images")
	flag.Parse()

	args := flag.Args()

	if len(args) < 1 {
		usage()
	}

	dbFile, args := parseCmd(args)

	var err error
	dbConn, err = sqlite.Open(dbFile)
	must(err)
	defer dbConn.Close()
	must(createDatabase())

	switch args[0] {
	case "serve":
		serve()
	case "update":
		update()
	}
}
