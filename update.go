package main

import (
	"bufio"
	"bytes"
	"golang.org/x/net/html"
	"fmt"
	"github.com/aarzilli/sandblast"
	"os"
	"strconv"
	"strings"
)

const debugProcessing = false

// the algorithm that looks for diffs is shit I must throw away large urls or the algorithm would never end
const MAX_STORE_SIZE = 500 * 1024

func update() {
	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		line := scanner.Text()
		if len(line) <= 0 {
			continue
		}
		if line[0] == '*' {
			// Important URL, store all diffs forever
			fmt.Printf("Getting important url: %s\n", line[1:])
			importantUrl(line[1:])
		} else {
			// Unimportant URL, store only first version
			fmt.Printf("Getting unimportant url: %s\n", line)
			unimportantUrl(line)
		}
	}
	must(scanner.Err())
}

// Important URL, store all diffs forever
func importantUrl(url string) {
	if debugProcessing {
		fmt.Printf("Fetching\n")
	}
	content, status, _, err := sandblast.FetchURL(url)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error fetching URL %s: %v\n", url, err)
		return
	}
	if status != 200 {
		fmt.Fprintf(os.Stderr, "Error fetching URL %s, status code %d\n", url, status)
		return
	}

	content, title, text := contentProcessing(url, content)

	if len(content) > MAX_STORE_SIZE {
		fmt.Printf("URL Too Large: %s\n", url)
		return
	}

	if debugProcessing {
		fmt.Printf("Lookup\n")
	}
	urlDescr := Lookup(url, true, -1, true)

	if debugProcessing {
		fmt.Printf("Getting stored content\n")
	}
	storedContent, diffs, ok := urlDescr.GetContent(-1)

	if !ok || (diffs > MAX_DIFFS) {
		if debugProcessing {
			fmt.Printf("Compression\n")
		}
		cc, isgz := maybeCompress(content)
		urlDescr.StoreContent(cc, false, isgz, true)
	} else {
		if debugProcessing {
			fmt.Printf("Compression and diff\n")
		}
		cc, isdiff, isgz := maybeDiffCompress(content, storedContent)
		urlDescr.StoreContent(cc, isdiff, isgz, true)
	}

	if debugProcessing {
		fmt.Printf("HTML Extraction\n")
	}
	if debugProcessing {
		fmt.Printf("Storing new content\n")
	}
	urlDescr.StoreContent2(title, text)
	if debugProcessing {
		fmt.Printf("Done\n")
	}
}

// Unimportant URL, store only first version
func unimportantUrl(conf string) {
	v := strings.SplitN(conf, ",", 2)
	if len(v) != 2 {
		fmt.Fprintf(os.Stderr, "Bad URL configuration line <%s>\n", conf)
		return
	}

	url := v[1]
	time, err := strconv.Atoi(v[0])
	if err != nil {
		time = 0
	}

	urlDescr := Lookup(url, false, time, true)
	if !urlDescr.IsNew {
		fmt.Fprintf(os.Stderr, "\tskipped\n")
		// already stored, skipping
		return
	}

	content, status, _, err := sandblast.FetchURL(url)
	if err != nil {
		urlDescr.Remove()
		fmt.Fprintf(os.Stderr, "Error fetching URL %s: %v\n", url, err)
		return
	}
	if status != 200 {
		urlDescr.Remove()
		fmt.Fprintf(os.Stderr, "Error fetching URL %s status code %d]\n", url, status)
		return
	}

	content, title, text := contentProcessing(url, content)

	cc, isgz := maybeCompress(content)
	urlDescr.StoreContent(cc, false, isgz, true)
	urlDescr.StoreContent2(title, text)
}

func contentProcessing(url string, content []byte) (rcontent []byte, title, text string) {
	rcontent = content
	htmlNode, err := html.Parse(bytes.NewReader(content))
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error parsing %s as HTML: %v\n", url, err)
		return
	}

	if fullStoreFlag {
		fullStore(url, htmlNode)
		var buf bytes.Buffer
		err = html.Render(&buf, htmlNode)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error fully retrieving %s HTML: %v\n", url, err)
		} else {
			rcontent = buf.Bytes()
		}
	}

	title, text, err = htmlExtract(htmlNode)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error extracting text from %s: %v\n", url, err)
	}

	return
}
