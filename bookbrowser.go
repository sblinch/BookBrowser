package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"

	_ "github.com/sblinch/BookBrowser/formats/epub"
	_ "github.com/sblinch/BookBrowser/formats/mobi"
	_ "github.com/sblinch/BookBrowser/formats/pdf"
	"github.com/sblinch/BookBrowser/server"
	"github.com/sblinch/BookBrowser/util"
	"github.com/sblinch/BookBrowser/util/sigusr"
	"github.com/spf13/pflag"
	"github.com/sblinch/BookBrowser/storage"
	"github.com/okzk/sdnotify"
)

var curversion = "dev"

func main() {
	workdir, err := os.Getwd()
	if err != nil {
		log.Fatalf("Fatal error: %s\n", err)
	}

	defdatadir, err := ioutil.TempDir("", "bookbrowser")
	if err != nil {
		defdatadir = filepath.Join(workdir, "_temp")
	}

	bookdir := pflag.StringP("bookdir", "b", workdir, "the directory to load books from (must exist)")
	datadir := pflag.StringP("datadir", "t", defdatadir, "the directory to store the database and cover thumbnails")
	addr := pflag.StringP("addr", "a", ":8090", "the address to bind the server to ([IP]:PORT)")
	nocovers := pflag.BoolP("nocovers", "n", false, "do not index covers")
	help := pflag.BoolP("help", "h", false, "Show this help text")
	sversion := pflag.Bool("version", false, "Show the version")
	pflag.Parse()

	if *sversion {
		fmt.Printf("BookBrowser %s\n", curversion)
		os.Exit(0)
	}

	if *help || pflag.NArg() != 0 {
		fmt.Fprintf(os.Stderr, "Usage: BookBrowser [OPTIONS]\n\nVersion:\n  BookBrowser %s\n\nOptions:\n", curversion)
		pflag.PrintDefaults()
		fmt.Fprintf(os.Stderr, "\n")
		if runtime.GOOS == "windows" {
			time.Sleep(time.Second * 2)
		}
		os.Exit(1)
	}

	removeDataDir := false

	log.Printf("BookBrowser %s\n", curversion)

	if _, err := os.Stat(*bookdir); err != nil {
		if os.IsNotExist(err) {
			log.Fatalf("Error: book directory %s does not exist\n", *bookdir)
		}
	}

	if fi, err := os.Stat(*datadir); err == nil || (fi != nil && fi.IsDir()) {
		removeDataDir = false
		if *datadir == defdatadir {
			removeDataDir = true
		}
	}

	*bookdir, err = filepath.Abs(*bookdir)
	if err != nil {
		log.Fatalf("Error: could not resolve book directory %s: %v\n", *bookdir, err)
	}

	if _, err := os.Stat(*datadir); os.IsNotExist(err) {
		os.Mkdir(*datadir, os.ModePerm)
	}

	*datadir, err = filepath.Abs(*datadir)
	if err != nil {
		log.Fatalf("Error: could not resolve temp directory %s: %v\n", *datadir, err)
	}

	stor, err := storage.New(*datadir + "/bookbrowser.sqlite3")
	if err != nil {
		log.Fatalf("Error: could not prepare SQLite database in %s: %v\n", *datadir, err)
	}

	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigs
		if removeDataDir {
			log.Println("Removing temporary data directory")
			os.RemoveAll(*datadir)
		}
		sdnotify.Status("Caught signal")

		os.Exit(0)
	}()

	if !strings.Contains(*addr, ":") {
		log.Fatalln("Error: invalid listening address")
	}

	sp := strings.SplitN(*addr, ":", 2)
	if sp[0] == "" {
		ip := util.GetIP()
		if ip != nil {
			log.Printf("This server can be accessed at http://%s:%s\n", ip.String(), sp[1])
		}
	}

	log.Printf("Server")
	s := server.NewServer(*addr, stor, *bookdir, *datadir, curversion, true, *nocovers)
	go func() {
		s.RefreshBookIndex()
		total, err := stor.Books.Count(storage.NewQuery())
		if err != nil {
			log.Fatalf("Fatal error: %v", err)
		}
		if total == 0 {
			log.Fatalln("Fatal error: no books found")
		}
		checkUpdate()
	}()

	sigusr.Handle(func() {
		log.Println("Booklist refresh triggered by SIGUSR1")
		s.RefreshBookIndex()
	})

	appExiting := make(chan struct{})
	watchdogExited := systemdWatchdog(appExiting)
	defer func() {
		sdnotify.Stopping()
		close(appExiting)
		<-watchdogExited
	}()

	err = s.Serve()
	if err != nil {
		log.Fatalf("Error starting server: %s\n", err)
	}
}

func systemdWatchdog(done chan struct{}) chan struct{} {
	watchdogExiting := make(chan struct{})

	sdnotify.Ready()
	sdnotify.Status("Serving requests")

	watchdogtick := time.Tick(30 * time.Second)
	go func() {
		defer func() {
			close(watchdogExiting)
		}()
		for {
			select {
			case <-watchdogtick:
				sdnotify.Watchdog()
			case <-done:
				break
			}
		}
	}()

	return watchdogExiting
}

func checkUpdate() {
	resp, err := http.Get("https://api.github.com/repos/geek1011/BookBrowser/releases/latest")
	if err != nil {
		return
	}
	defer resp.Body.Close()

	buf, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return
	}

	if resp.StatusCode != 200 {
		return
	}

	var obj struct {
		URL string `json:"html_url"`
		Tag string `json:"tag_name"`
	}
	if json.Unmarshal(buf, &obj) != nil {
		return
	}

	if curversion != "dev" {
		if !strings.HasPrefix(curversion, obj.Tag) {
			log.Printf("Running version %s. Latest version is %s: %s\n", curversion, obj.Tag, obj.URL)
		}
	}
}
