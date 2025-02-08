package staticweb

import (
	"bytes"
	"compress/gzip"
	"errors"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	_ "embed"

	"github.com/gomarkdown/markdown"
	mdhtml "github.com/gomarkdown/markdown/html"
	"github.com/gomarkdown/markdown/parser"
	"github.com/tdewolff/minify/v2"
	"github.com/tdewolff/minify/v2/html"
	regex "github.com/tkdeng/goregex"
	"github.com/tkdeng/goutil"
	"gopkg.in/yaml.v3"
)

type Config struct {
	Opts map[string]bool
	Vars map[string]string
	Meta map[string]string

	Styles []struct {
		Url   string
		Print bool
		Lazy  bool
	}

	Scripts []struct {
		Url    string
		Module bool
		Defer  bool
		Async  bool
		Wasm   string
	}

	layout map[string][]byte

	isHomePage bool
}

//go:embed templates/layout.html
var templateLayout []byte

//go:embed templates/body.html
var templateBody []byte

var DebugMode bool = false

var TemplateMode bool = false

func init() {
	if len(os.Args) > 1 && strings.HasPrefix(os.Args[0], "/tmp/go-build") && strings.HasPrefix(os.Args[1], "-test") {
		DebugMode = true
	}

	compileHTML(&templateLayout)
	compileHTML(&templateBody)
}

func Live(src, dist string, handleErr ...func(err error)) *goutil.FSWatcher {
	if len(handleErr) == 0 {
		handleErr = append(handleErr, func(err error) {
			log.Println(err)
		})
	}

	if path, err := filepath.Abs(src); err == nil {
		src = path
	}

	if path, err := filepath.Abs(dist); err == nil {
		dist = path
	}

	err := Compile(src, dist)
	if err != nil {
		handleErr[0](err)
	}

	lastChange := time.Now().UnixMilli()

	fw := goutil.FileWatcher()

	fw.OnFileChange = func(path, op string) {
		now := time.Now().UnixMilli()
		if now-lastChange < 10 {
			return
		}
		lastChange = now

		path = filepath.Dir(strings.TrimPrefix(path, src))

		if path == "/" || path == "" {
			if err := Compile(src, dist); err != nil {
				handleErr[0](err)
			}
		} else {
			if err := Compile(src, dist, path); err != nil {
				handleErr[0](err)
			}
		}
	}

	fw.OnDirAdd = func(path, op string) (addWatcher bool) {
		now := time.Now().UnixMilli()
		if now-lastChange < 10 {
			return
		}
		lastChange = now

		path = strings.TrimPrefix(path, src)

		if err := Compile(src, dist, path); err != nil {
			handleErr[0](err)
		}
		return true
	}

	fw.OnRemove = func(path, op string) (removeWatcher bool) {
		now := time.Now().UnixMilli()
		if now-lastChange < 10 {
			return
		}
		lastChange = now

		if strings.HasSuffix(path, ".html") || strings.HasSuffix(path, ".md") || strings.HasSuffix(path, ".yml") || strings.HasSuffix(path, ".yaml") || strings.HasSuffix(path, ".json") || strings.HasSuffix(path, ".cson") {
			path = filepath.Dir(strings.TrimPrefix(path, src))

			if path == "/" || path == "" {
				if err := Compile(src, dist); err != nil {
					handleErr[0](err)
				}
			} else {
				if err := Compile(src, dist, path); err != nil {
					handleErr[0](err)
				}
			}
		} else if distPath := strings.Replace(path, src, dist, 1); distPath != path {
			if err := os.RemoveAll(distPath); err != nil {
				handleErr[0](err)
			}
		}
		return true
	}

	fw.WatchDir(src)

	return fw
}

func Compile(src, dist string, page ...string) error {
	if path, err := filepath.Abs(src); err == nil {
		src = path
	}

	if path, err := filepath.Abs(dist); err == nil {
		dist = path
	}

	if stat, err := os.Stat(src); err != nil || !stat.IsDir() {
		return errors.New("src must be a directory: " + src)
	}

	os.MkdirAll(dist, 0755)

	var compPage []string
	if len(page) != 0 {
		compPage = strings.Split(strings.Trim(page[0], "/"), "/")
	}

	var compErr error

	var wg sync.WaitGroup

	compilePage(src, dist, &Config{
		layout: map[string][]byte{
			"layout": templateBody,
		},

		isHomePage: true,
	}, func() { wg.Add(1) }, func() { wg.Done() }, compPage, &compErr, true)

	wg.Wait()

	return compErr
}

func compilePage(src, dist string, config *Config, wgAdd func(), wgDone func(), compPage []string, compErr *error, init bool) {
	// load layout config
	goutil.ReadConfig(src+"/layout.yml", config)

	dirList := []string{}

	// pageVars := map[string]string{}
	pageOnlyConfig := Config{}

	// load layout and body files
	if files, err := os.ReadDir(src); err == nil {
		for _, file := range files {
			name := file.Name()

			/* if compPage != nil && len(compPage) != 0 {
				if name != compPage[0] && strings.TrimSuffix(name, ".html") != compPage[0] && strings.TrimSuffix(name, ".md") != compPage[0] {
					continue
				}
			} */

			if file.IsDir() {
				if compPage != nil && len(compPage) != 0 {
					if name != compPage[0] || len(dirList) != 0 {
						continue
					}
				}

				dirList = append(dirList, name)
			} else if (strings.HasSuffix(name, ".html") || strings.HasSuffix(name, ".md")) && !strings.HasPrefix(name, "@") {
				if buf, err := os.ReadFile(src + "/" + name); err == nil {
					// grab beginning yml config
					buf = regex.Comp(`(?s)^---+\n(.*?)\n---+\n`).RepFunc(buf, func(data func(int) []byte) []byte {
						err := yaml.Unmarshal(data(1), &pageOnlyConfig)
						if err != nil {
							*compErr = errors.Join(*compErr, err)
						}
						return []byte{}
					})

					if strings.HasSuffix(name, ".md") {
						compileMarkdown(&buf)
						name = name[:len(name)-3]
					} else {
						name = name[:len(name)-5]
					}

					compileHTML(&buf)
					config.layout[name] = buf
				}
			}

			/* if compPage != nil && len(compPage) != 0 {
				compPage = compPage[1:]
				break
			} */
		}
	}

	// clone config data
	pageConfig := Config{
		Opts:   map[string]bool{},
		Vars:   map[string]string{},
		Meta:   map[string]string{},
		layout: map[string][]byte{},
	}

	for key, val := range config.Opts {
		pageConfig.Opts[key] = val
	}

	for key, val := range config.Vars {
		pageConfig.Vars[key] = val
	}

	for key, val := range config.Meta {
		pageConfig.Meta[key] = val
	}

	// copy(pageConfig.Meta, config.Meta)
	pageConfig.Styles = append(pageConfig.Styles, config.Styles...)
	pageConfig.Scripts = append(pageConfig.Scripts, config.Scripts...)
	// copy(pageConfig.layout, config.layout)

	for key, val := range config.layout {
		pageConfig.layout[key] = goutil.CloneBytes(val)
	}

	// merge page only config
	if pageOnlyConfig.Vars != nil {
		for key, val := range pageOnlyConfig.Opts {
			pageConfig.Opts[key] = val
		}
	}

	if pageOnlyConfig.Vars != nil {
		for key, val := range pageOnlyConfig.Vars {
			pageConfig.Vars[key] = val
		}
	}

	if pageOnlyConfig.Meta != nil {
		for key, val := range pageOnlyConfig.Meta {
			pageConfig.Meta[key] = val
		}
	}

	if pageOnlyConfig.Styles != nil {
		pageConfig.Styles = append(pageConfig.Styles, pageOnlyConfig.Styles...)
	}

	if pageOnlyConfig.Scripts != nil {
		pageConfig.Scripts = append(pageConfig.Scripts, pageOnlyConfig.Scripts...)
	}

	pageConfig.isHomePage = config.isHomePage
	config.isHomePage = false

	if pageConfig.Meta["page"] == "" {
		pageConfig.Meta["page"] = pageConfig.Meta["title"]
	}

	if /* !pageConfig.isHomePage */ pageConfig.Meta["title"] != "" && pageConfig.Meta["sitetitle"] != "" && pageConfig.Meta["title"] != pageConfig.Meta["sitetitle"] {
		pageConfig.Meta["title"] = pageConfig.Meta["title"] + " | " + pageConfig.Meta["sitetitle"]
	} else if pageConfig.Meta["title"] == "" {
		pageConfig.Meta["title"] = pageConfig.Meta["sitetitle"]
	}

	if pageConfig.isHomePage {
		//todo: generate manifest.json (may use separate function)
	}

	if TemplateMode && !init && len(dirList) != 0 {
		os.Mkdir(dist, 0755)
	} else if !TemplateMode {
		os.Mkdir(dist, 0755)
	}

	wgAdd()
	go func() {
		defer wgDone()
		compilePageDist(src, dist, &pageConfig, compErr, init)
	}()

	//todo: remove unused subpages from dist

	for _, dir := range dirList {
		compilePage(src+"/"+dir, dist+"/"+dir, config, wgAdd, wgDone, compPage, compErr, false)
	}
}

func compilePageDist(src, dist string, config *Config, compErr *error, init bool) {
	distFile := dist + "/index.html"

	if TemplateMode && !init {
		distFile = dist + ".html"
	}

	// open dist file
	file, err := os.OpenFile(distFile, os.O_CREATE|os.O_TRUNC|os.O_RDWR, 0755)
	if err != nil {
		*compErr = errors.Join(*compErr, err)
		return
	}
	defer file.Close()

	// write initial layout template
	file.Write(templateLayout)
	file.Sync()

	usedMetaVars := []string{
		"sitetitle",
		"apptitle",
		"title",
		"page",
	}

	// write meta vars
	regex.Comp(`\{([A-Za-z0-9]+)(:.*?|)\}`).RepFileFunc(file, func(data func(int) []byte) []byte {
		if val, ok := config.Vars[string(data(1))]; ok {
			return []byte(val)
		} else if val, ok := config.Meta[string(data(1))]; ok {
			usedMetaVars = append(usedMetaVars, string(data(1)))
			return []byte(val)
		} else if len(data(2)) != 0 {
			return data(2)[1:]
		}
		return []byte{}
	}, true)

	// write meta to head
	for name, val := range config.Meta {
		if !goutil.Contains(usedMetaVars, name) {
			html := []byte(`<meta name="` + name + `" content="` + val + `"/>`)

			if DebugMode {
				html = append(html, '\n')
			}

			html = append(html, []byte(`{@head}`)...)
			regex.Comp(`\{@head\}`).RepFileStr(file, html, false)
		}
	}

	// write styles to head
	for _, style := range config.Styles {
		html := []byte(`<link rel="stylesheet" href="` + style.Url + `"`)

		if style.Print {
			html = append(html, []byte(` media="print"`)...)
		}

		if style.Lazy {
			html = append(html, []byte(` media="print" onload="this.media='all'"`)...)
		}

		html = append(html, []byte(`/>`)...)

		if DebugMode {
			html = append(html, '\n')
		}

		html = append(html, []byte(`{@head}`)...)
		regex.Comp(`\{@head\}`).RepFileStr(file, html, false)
	}

	// write scripts to head
	for _, script := range config.Scripts {
		html := []byte(`<script src="` + script.Url + `"`)

		if script.Module {
			html = append(html, []byte(` type="module"`)...)
		} else if script.Wasm != "" {
			html = append(html, []byte(` type="wasm/`+script.Wasm+`"`)...)
		}

		if script.Defer {
			html = append(html, []byte(` defer`)...)
		}

		if script.Async {
			html = append(html, []byte(` async`)...)
		}

		html = append(html, []byte("></script>")...)

		if DebugMode {
			html = append(html, '\n')
		}

		html = append(html, []byte("{@head}")...)
		regex.Comp(`\{@head\}`).RepFileStr(file, html, false)
	}

	// write main layoout to body
	regex.Comp(`\{@body\}`).RepFileStr(file, config.layout["layout"], false)

	file.Sync()

	// write body and embedded layout files
	regex.Comp(`\{@([A-Za-z0-9]+)\}\r?\n?`).RepFileFunc(file, func(data func(int) []byte) []byte {
		name := string(data(1))

		buf, err := os.ReadFile(src + "/@" + name + ".html")
		if err != nil {
			buf, err = os.ReadFile(src + "/@" + name + ".md")
			compileMarkdown(&buf)
		}

		if err == nil {
			compileHTML(&buf)
			return buf
		}

		if val, ok := config.layout[name]; ok {
			return val
		}

		return []byte{}
	}, true)

	// write meta vars
	regex.Comp(`\{([A-Za-z0-9]+)(:.*?|)\}`).RepFileFunc(file, func(data func(int) []byte) []byte {
		if val, ok := config.Vars[string(data(1))]; ok {
			return []byte(val)
		} else if val, ok := config.Meta[string(data(1))]; ok {
			return []byte(val)
		} else if len(data(2)) != 0 {
			return data(2)[1:]
		}
		return []byte{}
	}, true)

	file.Sync()
	file.Close()

	if DebugMode || !(config.Opts["gzip"] || config.Opts["gziponly"]) {
		return
	}

	// compress to gzip
	if buf, err := os.ReadFile(dist + "/index.html"); err == nil {
		gz, err := os.OpenFile(dist+"/index.html.gz", os.O_CREATE|os.O_TRUNC|os.O_RDWR, 0755)
		if err != nil {
			*compErr = errors.Join(*compErr, err)
			return
		}
		defer gz.Close()

		if w, err := gzip.NewWriterLevel(gz, 6); err == nil {
			w.Write(buf)
			w.Close()

			gz.Close()

			if config.Opts["gziponly"] {
				os.Remove(dist + "/index.html")
			}
		}
	}
}

func compileHTML(buf *[]byte) {
	//todo: add plugin support with shortcodes

	if DebugMode {
		return
	}

	/* if m, err := minify.HTML(string(*buf)); err == nil {
		*buf = []byte(m)
	} */

	m := minify.New()
	m.AddFunc("text/html", html.Minify)

	m.Add("text/html", &html.Minifier{
		KeepQuotes:       true,
		KeepDocumentTags: true,
		KeepEndTags:      true,
	})

	var b bytes.Buffer
	if err := m.Minify("text/html", &b, bytes.NewBuffer(*buf)); err == nil {
		*buf = b.Bytes()
	}
}

func compileMarkdown(buf *[]byte) {
	// create markdown parser with extensions
	extensions := parser.CommonExtensions | parser.AutoHeadingIDs | parser.NoEmptyLineBeforeBlock
	p := parser.NewWithExtensions(extensions)
	doc := p.Parse(*buf)

	// create HTML renderer with extensions
	htmlFlags := mdhtml.CommonFlags | mdhtml.HrefTargetBlank
	opts := mdhtml.RendererOptions{Flags: htmlFlags}
	renderer := mdhtml.NewRenderer(opts)

	*buf = markdown.Render(doc, renderer)
}
