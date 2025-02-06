package main

import (
	"errors"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"

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
	port := args.Get("", "port", "p", "listen", "http", "live", "l", "")

	if src == "" {
		printHelp()
		return
	}

	if out == "" {
		out = filepath.Dir(src) + "/dist"
	}

	if port == "true" {
		port = "3000"
	} else if port == "false" {
		port = ""
	} else if port != "" {
		if i, err := strconv.Atoi(port); err == nil && i >= 3000 && i <= 65535 {
			port = strconv.Itoa(i)
		} else {
			panic(errors.New("Cannot Listen On Port " + port))
		}
	}

	if port == "" {
		staticweb.Compile(src, out)
		return
	}

	//todo: listen on port
	staticweb.Live(src, out)

	fs := http.FileServer(http.Dir(out))

	// http.Handle("/", fs)
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		// Check if the file exists. If not, serve the 404 page.
		if path, err := goutil.JoinPath(out, goutil.Clean(r.URL.Path)); err == nil {
			if _, err := os.Stat(path); err != nil {
				w.WriteHeader(http.StatusNotFound)

				if path, err := goutil.JoinPath(out, "404", "index.html"); err == nil {
					if _, err := os.Stat(path); err == nil {
						http.ServeFile(w, r, path)
						return
					}
				}else if path, err := goutil.JoinPath(out, "404.html"); err == nil {
					if _, err := os.Stat(path); err == nil {
						http.ServeFile(w, r, path)
						return
					}
				}

				w.Write([]byte("Error 404: Page Not Found!"))
				return
			}
		}

		// If the file exists, serve it normally.
		http.StripPrefix("/", fs).ServeHTTP(w, r)
	})

	log.Println("Listening On Port " + port)
	log.Fatal(http.ListenAndServe(":"+port, nil))
}
