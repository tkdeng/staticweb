package main

import (
	"fmt"

	"github.com/tkdeng/goutil"
	"golang.org/x/term"
)

func printHelp() {
	tSize := 80
	if w, _, err := term.GetSize(0); err == nil {
		if w-10 > 20 {
			tSize = w - 10
		} else {
			tSize = 20
		}
	}

	fmt.Println(goutil.ToColumns([][]string{
		{"\nUsage: staticweb [src] [options...]\n"},
		{"--src, --root", "source dicectory."},
		{"--out", "output directory. (defaults to 'dist' just outside the source directory)."},
		{"--port, -l", "run live server on port. (defaults to '3000')."},
		{"--help, -h", "print this list."},
	}, tSize, "    ", "\n\n"))
}
