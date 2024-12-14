package main

import (
	"path/filepath"

	"github.com/tkdeng/goutil"
	"github.com/tkdeng/staticweb"
)

func main() {
	args := goutil.ReadArgs()

	help := args.Get("", "help", "h")
	if help != "" {
		printHelp()
		return
	}

	src := args.Get("", "src", "root", "")
	out := args.Get("", "out", "output", "o", "dist")

	if src == "" {
		printHelp()
		return
	}

	if out == "" {
		out = filepath.Dir(src) + "/dist"
	}

	staticweb.Compile(src, out)
}
