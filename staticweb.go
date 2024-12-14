package staticweb

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"

	_ "embed"

	"github.com/gomarkdown/markdown"
	"github.com/gomarkdown/markdown/html"
	"github.com/gomarkdown/markdown/parser"
	"github.com/tdewolff/minify/v2/minify"
	regex "github.com/tkdeng/goregex"
	"github.com/tkdeng/goutil"
	"gopkg.in/yaml.v3"
)

type Config struct {
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
	}

	layout map[string][]byte

	isHomePage bool
}

//go:embed templates/layout.html
var templateLayout []byte

//go:embed templates/body.html
var templateBody []byte

var DebugMode bool = false

func init() {
	if len(os.Args) > 1 && strings.HasPrefix(os.Args[0], "/tmp/go-build") && strings.HasPrefix(os.Args[1], "-test") {
		DebugMode = true
	}

	compileHTML(&templateLayout)
	compileHTML(&templateBody)
}

func Compile(src, dist string /* , page ...string */) error {
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

	//todo: add ability to recompile individual pages
	/* if len(page) != 0 {
		if page[0] != "" && page[0] != "/" {
			path, err := goutil.JoinPath(src, page[0])
			if err != nil {
				return err
			}
			src = path

			path, err = goutil.JoinPath(dist, page[0])
			if err != nil {
				return err
			}
			dist = path
		}
	} */

	var compErr error

	var wg sync.WaitGroup

	compilePage(src, dist, &Config{
		layout: map[string][]byte{
			"layout": templateBody,
		},

		isHomePage: true,
	}, func() { wg.Add(1) }, func() { wg.Done() }, &compErr)

	wg.Wait()

	return compErr
}

func compilePage(src, dist string, config *Config, wgAdd func(), wgDone func(), compErr *error) {
	// load layout config
	goutil.ReadConfig(src+"/layout.yml", config)

	dirList := []string{}

	// pageVars := map[string]string{}
	pageOnlyConfig := Config{}

	// load layout and body files
	if files, err := os.ReadDir(src); err == nil {
		for _, file := range files {
			name := file.Name()

			if file.IsDir() {
				dirList = append(dirList, name)
			} else if (strings.HasSuffix(name, ".html") || strings.HasSuffix(name, ".md")) && !strings.HasPrefix(name, "@") {
				if buf, err := os.ReadFile(src + "/" + name); err == nil {

					// grab beginning yml config
					buf = regex.Comp(`(?s)^---+\n(.*?)\n---+\n`).RepFunc(buf, func(data func(int) []byte) []byte {
						err := yaml.Unmarshal(data(1), &pageOnlyConfig)
						if err != nil {
							// fmt.Println(err)
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
		}
	}

	// clone config data
	pageConfig := Config{
		Vars:   map[string]string{},
		Meta:   map[string]string{},
		layout: map[string][]byte{},
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

	os.Mkdir(dist, 0755)

	wgAdd()
	go func() {
		defer wgDone()
		compilePageDist(src, dist, &pageConfig, compErr)
	}()

	//todo: remove unused subpages from dist

	for _, dir := range dirList {
		compilePage(src+"/"+dir, dist+"/"+dir, config, wgAdd, wgDone, compErr)
	}
}

func compilePageDist(src, dist string, config *Config, compErr *error) {
	// open dist file
	file, err := os.OpenFile(dist+"/index.html", os.O_CREATE|os.O_TRUNC|os.O_RDWR, 0755)
	if err != nil {
		// fmt.Println(err)
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

	// compress to gzip
	/* if buf, err := os.ReadFile(dist + "/index.html"); err == nil {
		gz, err := os.OpenFile(dist+"/index.html.gz", os.O_CREATE|os.O_TRUNC|os.O_RDWR, 0755)
		if err != nil {
			*compErr = errors.Join(*compErr, err)
			return
		}
		defer gz.Close()

		if w, err := gzip.NewWriterLevel(gz, 6); err == nil {
			w.Write(buf)
			w.Close()
		}

		gz.Close()
		os.Remove(dist + "/index.html")
	} */
}

func compileHTML(buf *[]byte) {
	//todo: add plugin support with shortcodes

	if DebugMode {
		return
	}

	if m, err := minify.HTML(string(*buf)); err == nil {
		*buf = []byte(m)
	}
}

func compileMarkdown(buf *[]byte) {
	// create markdown parser with extensions
	extensions := parser.CommonExtensions | parser.AutoHeadingIDs | parser.NoEmptyLineBeforeBlock
	p := parser.NewWithExtensions(extensions)
	doc := p.Parse(*buf)

	// create HTML renderer with extensions
	htmlFlags := html.CommonFlags | html.HrefTargetBlank
	opts := html.RendererOptions{Flags: htmlFlags}
	renderer := html.NewRenderer(opts)

	*buf = markdown.Render(doc, renderer)
}
