package main

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path"
	"regexp"
	"sort"
	"strings"
	"syscall"
)

func main() {
	var signals = make(chan os.Signal)
	go func() {
		go signal.Notify(signals, syscall.SIGHUP)
		for range signals {
			log.Printf("HUP")
		}
	}()

	http.Handle("/asset/", assetPrefix(http.FileServer(http.Dir("./assets"))))
	http.Handle("/data/", http.StripPrefix("/data", http.HandlerFunc(dataHandler)))

	mappers := 0
	for _, arg := range os.Args[1:] {
		if idx := strings.IndexByte(arg, '='); idx >= 0 {
			mappers++
			prefix := arg[:idx]
			prefix = path.Join("/", prefix)
			if !strings.HasSuffix(prefix, "/") {
				prefix += "/"
			}
			file := path.Join(".", arg[idx+1:])
			fmt.Printf("mapped %q to %q\n", prefix, file)
			http.Handle(prefix, bootloader(file))
		} else {
			err := os.Chdir(arg)
			if err != nil {
				panic(err)
			}
		}
	}
	if mappers == 0 {
		prefix := "/"
		file := "index.html"
		fmt.Printf("mapped %q to %q\n", prefix, file)
		http.Handle(prefix, bootloader(file))
	}

	fmt.Printf("listening on: http://localhost:3080/\n")
	err := http.ListenAndServe(":3080", nil)
	if err != nil {
		panic(err)
	}
}

func bootloader(file string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		data, err := ioutil.ReadFile(file)
		if err != nil {
			panic(err)
		}

		tmpl := parseTmpl(data)

		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-cache, private")

		head := `<script>var Featherhead = {"commit":"` + bootHash + `","repo":"fhdevd","ref":"refs/heads/master","master-domain":"localhost:3080","cdn-domain":"localhost:3080","app-domain":"localhost:3080"};</script>`

		_, _, err = tmpl.writeTo(w, []byte(head), []byte("/asset/fhdevd/"+bootHash+"/"))
		if err != nil {
			panic(err)
		}
	}
}

type template struct {
	Parts [][]byte
	Links []string
}

var (
	reHEAD  = regexp.MustCompile("<head[^>]*>")
	reBASE  = regexp.MustCompile("<(?:[^/][^ >]*)(?:\\s(?:[^>\"']+|(?:[\"']([./]*assets/)([^\"']+))|[\"'])+)>")
	litHEAD = []byte{0}
	litBASE = []byte{1}
)

func parseTmpl(data []byte) *template {
	var (
		m0 = reHEAD.FindAllIndex(data, -1)
		m1 = reBASE.FindAllSubmatchIndex(data, -1)
		m  = m1
	)

	if len(m0) > 0 {
		m = append(m, m0...)
	}

	sort.Sort(sortedMatches(m))

	var (
		parts  [][]byte
		links  []string
		offset int
	)

	for _, mi := range m {
		switch len(mi) {
		case 2:
			idx := mi[1] - offset
			if idx > 0 {
				part := data[:idx]
				data = data[idx:]
				offset += idx
				parts = append(parts, part)
			}
			parts = append(parts, litHEAD)
		case 6:
			beg := mi[2] - offset
			end := mi[3] - offset
			begLink := mi[4] - offset
			endLink := mi[5] - offset

			if begLink >= 0 && endLink >= 0 {
				link := string(data[begLink:endLink])
				if strings.HasSuffix(link, ".css") || strings.HasSuffix(link, ".js") {
					links = append(links, link)
				}
			}

			if beg > 0 {
				part := data[:beg]
				parts = append(parts, part)
			}

			if beg >= 0 {
				parts = append(parts, litBASE)
				data = data[end:]
				offset += end
			}

		default:
			panic(fmt.Sprintf("invalid match: %v", mi))
		}

	}
	if len(data) > 0 {
		parts = append(parts, data)
	}

	return &template{Parts: parts, Links: links}
}

type sortedMatches [][]int

func (s sortedMatches) Len() int           { return len(s) }
func (s sortedMatches) Less(i, j int) bool { return s[i][0] < s[j][0] }
func (s sortedMatches) Swap(i, j int)      { s[i], s[j] = s[j], s[i] }

func (t *template) writeTo(w io.Writer, head, base []byte) (int64, []string, error) {
	var n int64

	for _, part := range t.Parts {
		var (
			err error
			ni  int
		)

		switch part[0] {
		case 0:
			ni, err = w.Write(head)
			n += int64(ni)
		case 1:
			ni, err = w.Write(base)
			n += int64(ni)
		default:
			ni, err = w.Write(part)
			n += int64(ni)
		}

		if err != nil {
			return n, nil, err
		}
	}

	var links = make([]string, len(t.Links))
	var baseStr = string(base)
	for i, path := range t.Links {
		links[i] = "<" + baseStr + path + ">; rel=\"prefetch\"; crossorigin"
	}

	return n, links, nil
}

func assetPrefix(h http.Handler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		fname := path.Join("/", r.URL.Path)
		if strings.Count(fname, "/") < 3 {
			http.NotFound(w, r)
			return
		}
		fname = strings.Join(strings.Split(fname, "/")[4:], "/")
		fname = path.Join("/", fname)
		r.URL.Path = fname
		h.ServeHTTP(w, r)
	}
}

func dataHandler(w http.ResponseWriter, r *http.Request) {
	fname := path.Join("/", r.URL.Path)
	fname = strings.TrimSuffix(fname, ".json")
	if strings.Count(fname, "/") < 2 {
		http.NotFound(w, r)
		return
	}
	fname = strings.Join(strings.Split(fname, "/")[3:], "/")
	fname = path.Join("./data", fname)

	data, err := ioutil.ReadFile(fname + ".json")
	if os.IsNotExist(err) {
		data = nil
		err = nil
	}
	if err != nil {
		panic(err)
	}

	entries, err := ioutil.ReadDir(fname)
	if os.IsNotExist(err) {
		entries = nil
		err = nil
	}
	if err != nil {
		panic(err)
	}

	if entries == nil && data == nil {
		http.NotFound(w, r)
		return
	}

	type child struct {
		Name string `json:"name"`
	}

	var resp struct {
		Data     json.RawMessage `json:"data,omitempty"`
		Children []child         `json:"children"`
	}

	resp.Data = data
	seen := map[string]bool{}
	for _, entry := range entries {
		ename := entry.Name()
		ename = strings.TrimSuffix(ename, ".json")
		if !seen[ename] {
			seen[ename] = true
			resp.Children = append(resp.Children, child{Name: ename})
		}
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache, private")

	err = json.NewEncoder(w).Encode(&resp)
	if err != nil {
		panic(err)
	}
}

var bootHash = randHash()

func randHash() string {
	var d [20]byte
	_, err := io.ReadFull(rand.Reader, d[:])
	if err != nil {
		panic(err)
	}
	return hex.EncodeToString(d[:])
}
