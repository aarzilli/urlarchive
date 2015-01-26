package main

import (
	"camlistore.org/pkg/syncutil"
	"code.google.com/p/go.net/html"
	"code.google.com/p/go.net/html/atom"
	"fmt"
	"net/url"
	"os"
	"strings"
	"sync"
)

const dldParallelism = 20

type urlToFetch struct {
	resUrl string
	attrs  []*string
}

type urlsToFetch map[string]*urlToFetch

func fullStore(url string, node *html.Node) {
	toFetch := make(urlsToFetch)
	fullStoreSiblingRecur(url, node, toFetch)
	gate := syncutil.NewGate(dldParallelism)
	var dbAccessLock sync.Mutex
	for k := range toFetch {
		gate.Start()
		f := toFetch[k]
		go func() {
			defer gate.Done()
			f.Fetch(dbAccessLock)
		}()
	}
	for i := 0; i < dldParallelism; i++ {
		gate.Start()
	}
	return
}

func fullStoreSiblingRecur(url string, node *html.Node, toFetch urlsToFetch) {
	for n := node; n != nil; n = n.NextSibling {
		fullStoreChildRecur(url, n, toFetch)
	}
}

func fullStoreChildRecur(url string, node *html.Node, toFetch urlsToFetch) {
	fullStoreProcess(url, node, toFetch)
	if node.FirstChild != nil {
		fullStoreSiblingRecur(url, node.FirstChild, toFetch)
	}
}

func fullStoreProcess(url string, node *html.Node, toFetch urlsToFetch) {
	if node.Type != html.ElementNode {
		return
	}

	switch node.DataAtom {
	case atom.Img:
		for i := range node.Attr {
			if strings.ToLower(node.Attr[i].Key) == "src" {
				resUrl := resolveUrl(url, node.Attr[i].Val)
				toFetch.Add(resUrl, &(node.Attr[i].Val))
			}
		}

	case atom.Link:
		isStylesheet := false
		hrefIdx := -1
		for i := range node.Attr {
			switch strings.ToLower(node.Attr[i].Key) {
			case "rel":
				if strings.ToLower(node.Attr[i].Val) == "stylesheet" {
					isStylesheet = true
				}
			case "href":
				hrefIdx = i
			}
		}

		if isStylesheet && hrefIdx >= 0 {
			resUrl := resolveUrl(url, node.Attr[hrefIdx].Val)
			toFetch.Add(resUrl, &node.Attr[hrefIdx].Val)
		}

	default:
		//nothing
	}
}

func resolveUrl(originUrl, relUrl string) string {
	u, err := url.Parse(originUrl)
	if err != nil {
		return relUrl
	}

	ru, err := u.Parse(relUrl)
	if err != nil {
		return relUrl
	}

	return ru.String()
}

func (toFetch urlsToFetch) Add(resUrl string, val *string) {
	m, ok := toFetch[resUrl]
	if !ok {
		m = &urlToFetch{resUrl, []*string{}}
		toFetch[resUrl] = m
	}
	m.attrs = append(m.attrs, val)
}

func (u *urlToFetch) Fetch(dbAccessLock sync.Mutex) {
	caddr, err := RetrieveContentAddressable(u.resUrl, dbAccessLock)
	if err != nil {
		fmt.Fprintf(os.Stderr, "\tError retrieving resource %s: %v\n", u.resUrl, err)
		return
	}

	for i := range u.attrs {
		*(u.attrs[i]) = "/additional/" + caddr
	}
}
