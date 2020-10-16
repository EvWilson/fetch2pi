package main

import (
	"flag"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/PuerkitoBio/goquery"
)

const maxRetries = 5

var (
	dbg *log.Logger
	er  *log.Logger
)

func init() {
	logFlags := log.Ldate | log.Ltime | log.Lshortfile
	dbg = log.New(os.Stdout, "DEBUG: ", logFlags)
	er = log.New(os.Stderr, "ERROR: ", logFlags)
}

func main() {
	loc, outDir, server := initConfig()

	dbg.Printf("Fetching directory at: %s, using output directory: %s, proxying to: %s", loc, outDir, server)

	startDL(loc, outDir, server)

	dbg.Println("Relay complete!")
}

func startDL(URL, outDir, dest string) {
	// Add final slash if needed
	if outDir[len(outDir)-1:] != "/" {
		outDir += "/"
	}

	var wg sync.WaitGroup
	wg.Add(1)
	go visitPage(URL, outDir, dest, &wg)
	wg.Wait()
}

// Recursively visit each link on a given page, queueing up additional pages to
//	visit if they seem to be directories, otherwise start downloading and
//	relaying the link
func visitPage(dlURL, dirPath, dest string, wg *sync.WaitGroup) {
	defer wg.Done()

	resp, err := http.Get(dlURL)
	if err != nil {
		er.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		er.Fatalf("status code error: %d %s", resp.StatusCode, resp.Status)
	}

	doc, err := goquery.NewDocumentFromReader(resp.Body)
	if err != nil {
		er.Fatal(err)
	}

	// goquery is wonderfully succinct
	doc.Find("a").Each(func(i int, s *goquery.Selection) {
		// Skip any link that isn't part of the archive
		href, _ := s.Attr("href")
		if href[:1] == "/" || href[:1] == "?" {
			return
		}

		if isDirectory(href) {
			wg.Add(1)
			go visitPage(dlURL+href, dirPath+href, dest, wg)
		} else {
			wg.Add(1)
			go proxyFile(dlURL+href, dirPath+href, dest, wg)
		}
	})
}

// Relatively simple download and post, just with a basic retry in case the
//	download fails, and the ability to monitor download status with a periodic
//	print
func proxyFile(URL, path, dest string, wg *sync.WaitGroup) {
	defer wg.Done()

	var fileResp *http.Response
	i := 0
	for {
		resp, err := http.Get(URL)
		if err != nil {
			i++
			er.Println(err, ", RETRY COUNT: ", i, ", FOR FILE: ", URL)
		} else {
			fileResp = resp
			break
		}

		if i == maxRetries {
			er.Fatal("Reached maximum retry count for: ", URL)
		}
	}
	defer fileResp.Body.Close()

	fileSize, err := strconv.ParseUint(fileResp.Header.Get("Content-Length"), 10, 64)
	if err != nil {
		fileSize = 0
	}

	rc := readCounter{
		reader:   fileResp.Body,
		tag:      path,
		complete: 0,
		size:     fileSize,
	}
	timer := scheduleAtInterval(func() { rc.Print() }, 15*time.Second)
	resp, err := http.Post(dest+path, "application/zip", &rc)
	if err != nil {
		er.Fatal(err)
	}
	defer resp.Body.Close()
	timer.Stop()
}

func isDirectory(filename string) bool {
	return filename[len(filename)-1:] == "/"
}

func isValidURL(toTest string) bool {
	_, err := url.ParseRequestURI(toTest)
	if err != nil {
		return false
	}

	u, err := url.Parse(toTest)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return false
	}

	return true
}

// Below structure allows us to see prints on a fifteen second interval showing
//	the download completion percentage for large file downloads
func scheduleAtInterval(f func(), interval time.Duration) *time.Ticker {
	ticker := time.NewTicker(interval)
	go func() {
		for range ticker.C {
			f()
		}
	}()
	return ticker
}

type readCounter struct {
	reader   io.Reader
	tag      string
	complete uint64
	size     uint64
}

func (rc *readCounter) Read(p []byte) (n int, err error) {
	n, err = rc.reader.Read(p)
	rc.complete += uint64(n)
	return
}

func (rc *readCounter) Print() {
	dbg.Printf("%s %.2f %% complete", rc.tag, float64(rc.complete)/float64(rc.size)*100)
}

func initConfig() (string, string, string) {
	locPtr := flag.String("loc", "", "Location to DL SU from")
	outDirPtr := flag.String("out", "", "The name of the output artifact")
	serverPtr := flag.String("to", "", "The location of the server to send the update to")
	flag.Parse()
	loc := *locPtr
	outDir := *outDirPtr
	server := *serverPtr
	if loc == "" {
		er.Fatal("Provide at least a URL to retrieve from with -loc")
	} else if server == "" {
		er.Fatal("Provide a relay location with -to")
	} else if !isValidURL(loc) {
		er.Fatal("Not valid URL: ", loc)
	} else if !isValidURL(server) {
		er.Fatal("Not valid URL: ", server)
	}
	if outDir == "" {
		er.Fatal("Please provide a name for the output directory with -out")
	}
	// Append slashes if necessary for our expected URL structure
	if server[len(server)-1:] != "/" {
		server += "/"
	}
	if loc[len(loc)-1:] != "/" {
		loc += "/"
	}

	return loc, outDir, server
}
