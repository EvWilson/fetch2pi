package main

import (
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

const port = ":8321"
const createPerm = os.ModePerm

// 1MB copy buffer
const copyBufferSize = 1024 * 1024

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
	mux := http.NewServeMux()
	mux.Handle("/", routeSplitter())

	wrappedMux := serveLogger(mux)

	s := http.Server{
		Addr:    port,
		Handler: wrappedMux,
	}

	dbg.Println("Serving at ", port)
	er.Fatal(s.ListenAndServe())
}

// POSTs to memory-optimized file sink
// GETs through standard Golang fileserver (gosh that's nice)
// Drop all else
func routeSplitter() http.Handler {
	raspi := raspiZipHandler{}
	fileserver := http.FileServer(http.Dir("."))

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" {
			raspi.ServeHTTP(w, r)
		} else if r.Method == "GET" {
			fileserver.ServeHTTP(w, r)
		} else {
			w.WriteHeader(405)
		}
	})
}

// Below handler is for saving incoming file data without buffering too much
//	in memory, as I used a Raspberry Pi 3B as my sink
type raspiZipHandler struct{}

func (r raspiZipHandler) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	name := strings.TrimLeft(req.URL.Path, "/\\")

	err := os.MkdirAll(filepath.Dir(name), createPerm)
	if err != nil {
		logServError(w, "Error creating wrapping directories", err)
		return
	}

	out, err := os.Create(name)
	if err != nil {
		logServError(w, "Error creating outfile", err)
		return
	}

	// buffer for copy - standard copy uses awful 32KB buffer
	buf := make([]byte, copyBufferSize)
	_, err = io.CopyBuffer(out, req.Body, buf)
	if err != nil {
		logServError(w, "Error while copying file data", err)
		return
	}
	out.Close()
}

// Below struct wraps server mux to provide logging on all requests
type loggingResponseWriter struct {
	http.ResponseWriter
	statusCode int
}

func newLoggingResponseWriter(w http.ResponseWriter) *loggingResponseWriter {
	return &loggingResponseWriter{w, http.StatusOK}
}

func (lrw *loggingResponseWriter) WriteHeader(code int) {
	lrw.statusCode = code
	lrw.ResponseWriter.WriteHeader(code)
}

func serveLogger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		lrw := newLoggingResponseWriter(w)
		next.ServeHTTP(lrw, r)
		dbg.Printf("%s %d %s", r.Method, lrw.statusCode, r.URL)
	})
}

// Simplify error responses
func logServError(w http.ResponseWriter, msg string, err error) {
	er.Println(msg, ": ", err)
	w.WriteHeader(500)
	w.Write([]byte(msg))
}
