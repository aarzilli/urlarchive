package main

import (
	"bytes"
	"golang.org/x/net/html"
	"github.com/aarzilli/sandblast"
	"io"
	"strings"
)

func findChild(root *html.Node, name string) *html.Node {
	if root == nil {
		return nil
	}
	name = strings.ToLower(name)
	child := root.FirstChild
	for child != nil {
		if (child.Type == html.ElementNode) && (strings.ToLower(child.Data) == name) {
			return child
		}
		child = child.NextSibling
	}
	return nil
}

func findRoot(node *html.Node) *html.Node {
	if node == nil {
		return nil
	}
	if node.Type == html.DocumentNode {
		return findRoot(node.FirstChild)
	}
	for node != nil {
		if (node.Type == html.ElementNode) && (strings.ToLower(node.Data) == "html") {
			return node
		}
		node = node.NextSibling
	}
	return nil
}

func findContent(node *html.Node) string {
	if node == nil {
		return ""
	}
	out := bytes.NewBuffer([]byte{})
	for node != nil {
		if node.Type == html.TextNode {
			out.Write([]byte(node.Data))
		}
		node = node.NextSibling
	}
	return string(out.Bytes())
}

func getTitle(root *html.Node) string {
	head := findChild(root, "head")
	title := findChild(head, "title")
	if title == nil {
		return ""
	}
	return findContent(title.FirstChild)

}

func getText(node *html.Node, out io.Writer) {
	if node == nil {
		return
	}
	switch node.Type {
	case html.ElementNode:
		child := node.FirstChild
		for child != nil {
			getText(child, out)
			child = child.NextSibling
		}
	case html.TextNode:
		out.Write([]byte(node.Data))
	}
}

func htmlExtract(node *html.Node) (title string, text string, err error) {
	title, text, err = sandblast.Extract(node)
	return
}
