// Copyright 2023 Paolo Fabio Zaino
//
// Licensed under the GNU AFFERO GENERAL PUBLIC LICENSE (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     https://www.gnu.org/licenses/agpl-3.0.en.html#license-text
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Package main implements a file upload HTTP server that accepts JSON data via POST requests.
package main

import (
	"bufio"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const (
	uploadDir     = "./uploads"
	maxBodySize   = 10 << 20 // 10 MB
	workerCount   = 4
	writeQueueCap = 100
)

var (
	isReady   bool
	readyLock sync.RWMutex
)

type writeRequest struct {
	data []byte
	path string
}

var (
	writeQueue = make(chan writeRequest, writeQueueCap)
	bufferPool = sync.Pool{
		New: func() any {
			return bufio.NewWriterSize(nil, 4096)
		},
	}
)

func setReady(ready bool) {
	readyLock.Lock()
	defer readyLock.Unlock()
	isReady = ready
}

func checkReady() bool {
	readyLock.RLock()
	defer readyLock.RUnlock()
	return isReady
}

func main() {
	rand.Seed(time.Now().UnixNano())

	if err := os.MkdirAll(uploadDir, 0755); err != nil {
		log.Fatalf("Failed to create upload directory: %v", err)
	}

	for i := 0; i < workerCount; i++ {
		go fileWriterWorker()
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/v1/collection", handleSubmit)
	mux.HandleFunc("/v1/collection/", handleSubmit)
	mux.HandleFunc("/v1/health", handleHealth)
	mux.HandleFunc("/v1/ready", handleReady)

	handler := withRecover(withLogging(withCORS(mux)))

	server := &http.Server{
		Addr:         ":8989",
		Handler:      handler,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	log.Println("Listening on :8989")
	// after initialization
	setReady(true)
	if err := server.ListenAndServe(); err != nil {
		log.Fatalf("Server error: %v", err)
	}
}

func withCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		next.ServeHTTP(w, r)
	})
}

func withRecover(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				log.Printf("PANIC: %v", rec)
				http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			}
		}()
		next.ServeHTTP(w, r)
	})
}

func withLogging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		log.Printf("%s %s %s", r.Method, r.URL.Path, time.Since(start))
	})
}

func handleHealth(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("OK\n"))
}

func handleReady(w http.ResponseWriter, r *http.Request) {
	if checkReady() {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("READY\n"))
	} else {
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte("NOT READY\n"))
	}
}

func handleSubmit(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodPost:
		handlePost(w, r)
	case http.MethodGet:
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"API is alive"}`))
	default:
		respondWithError(w, http.StatusMethodNotAllowed, "Only GET and POST allowed", nil)
	}
}

func handlePost(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		respondWithError(w, http.StatusMethodNotAllowed, "Only POST allowed", nil)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxBodySize)
	defer r.Body.Close()

	var reader io.Reader = r.Body

	// Check for gzip
	if r.Header.Get("Content-Encoding") == "gzip" {
		gzr, err := gzip.NewReader(r.Body)
		if err != nil {
			respondWithError(w, http.StatusBadRequest, "Invalid gzip data", err)
			return
		}
		defer gzr.Close()
		reader = gzr
	}

	body, err := io.ReadAll(reader)
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Failed to read request body", err)
		return
	}

	ip := sanitizeIP(getClientIP(r))
	if ip == "" {
		ip = "unknown"
	}

	now := time.Now().UTC()
	timestamp := now.Format("2006-01-02-15_04_05.000000000")
	suffix := fmt.Sprintf("-%d", rand.Intn(10000))

	isJSON := json.Valid(body)
	ext := ".json"
	if !isJSON {
		ext = ".txt"
	}

	filename := fmt.Sprintf("%s-%s%s%s", ip, timestamp, suffix, ext)
	fullPath := filepath.Join(uploadDir, filename)

	req := writeRequest{
		data: body,
		path: fullPath,
	}

	select {
	case writeQueue <- req:
		// OK
	case <-r.Context().Done():
		respondWithError(w, http.StatusRequestTimeout, "Request cancelled", r.Context().Err())
		return
	}

	w.WriteHeader(http.StatusAccepted)
	if isJSON {
		_, _ = w.Write([]byte("JSON stored\n"))
	} else {
		_, _ = w.Write([]byte("Invalid JSON — stored as .txt\n"))
	}
}

func fileWriterWorker() {
	for req := range writeQueue {
		writeToFile(req.data, req.path)
	}
}

func writeToFile(data []byte, path string) {
	f, err := os.Create(path)
	if err != nil {
		log.Printf("ERROR: Failed to create file %s: %v\n", path, err)
		return
	}
	defer f.Close()

	buf := bufferPool.Get().(*bufio.Writer)
	buf.Reset(f)
	defer bufferPool.Put(buf)

	if _, err := buf.Write(data); err != nil {
		log.Printf("ERROR: Failed to write to file %s: %v\n", path, err)
		return
	}
	if err := buf.Flush(); err != nil {
		log.Printf("ERROR: Failed to flush buffer for file %s: %v\n", path, err)
	}
}

func getClientIP(r *http.Request) string {
	if forwarded := r.Header.Get("X-Forwarded-For"); forwarded != "" {
		parts := strings.Split(forwarded, ",")
		return strings.TrimSpace(parts[0])
	}
	ip, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return ""
	}
	return ip
}

func sanitizeIP(ip string) string {
	// Remove characters that are unsafe for filenames
	ip = strings.ReplaceAll(ip, ":", "_")
	ip = strings.ReplaceAll(ip, "/", "_")
	ip = strings.ReplaceAll(ip, "\\", "_")
	return ip
}

func respondWithError(w http.ResponseWriter, statusCode int, message string, err error) {
	logMsg := message
	if err != nil {
		logMsg += " - " + err.Error()
	}
	log.Println("ERROR:", logMsg)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	resp := map[string]string{"error": message}
	_ = json.NewEncoder(w).Encode(resp)
}
