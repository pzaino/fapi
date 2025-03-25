package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const uploadDir = "./uploads"

var bufferPool = sync.Pool{
	New: func() any {
		return bufio.NewWriterSize(nil, 4096)
	},
}

func main() {
	// Create uploads dir if missing
	if err := os.MkdirAll(uploadDir, 0755); err != nil {
		panic(err)
	}

	http.HandleFunc("/collection", handleSubmit)

	fmt.Println("Listening on :8989")
	if err := http.ListenAndServe(":8989", nil); err != nil {
		panic(err)
	}
}

func handleSubmit(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		respondWithError(w, http.StatusMethodNotAllowed, "Only POST allowed", nil)
		return
	}

	// Read the body
	body, err := io.ReadAll(r.Body)
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Failed to read request body", err)
		return
	}
	defer r.Body.Close()

	// Get client IP
	ip := getClientIP(r)
	if ip == "" {
		ip = "127.0.0.1"
	}

	// Create file name
	now := time.Now().UTC()
	timestamp := now.Format("2006-01-02-15_04_05.000000000") // includes nanoseconds

	// Check if JSON is valid
	var js map[string]interface{}
	isJSON := json.Unmarshal(body, &js) == nil
	ext := ".json"
	if !isJSON {
		ext = ".txt"
	}

	filename := fmt.Sprintf("%s-%s%s", ip, timestamp, ext)
	filepath := filepath.Join(uploadDir, filename)

	go func(data []byte, path string) {
		f, err := os.Create(path)
		if err != nil {
			fmt.Printf("ERROR: Failed to create file %s: %v\n", path, err)
			return
		}
		defer f.Close()

		buf := bufferPool.Get().(*bufio.Writer)
		buf.Reset(f)
		defer bufferPool.Put(buf)

		if _, err := buf.Write(data); err != nil {
			fmt.Printf("ERROR: Failed to write to file %s: %v\n", path, err)
			return
		}
		if err := buf.Flush(); err != nil {
			fmt.Printf("ERROR: Failed to flush buffer for file %s: %v\n", path, err)
		}
	}(body, filepath)

	if isJSON {
		w.WriteHeader(http.StatusAccepted)
		_, err = w.Write([]byte("JSON stored\n"))
	} else {
		w.WriteHeader(http.StatusAccepted)
		_, err = w.Write([]byte("Invalid JSON — stored as .txt\n"))
	}
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Failed to write response", err)
		return
	}
}

func getClientIP(r *http.Request) string {
	// If behind a proxy/load balancer:
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

func respondWithError(w http.ResponseWriter, statusCode int, message string, err error) {
	logMsg := message
	if err != nil {
		logMsg += " - " + err.Error()
	}
	fmt.Println("ERROR:", logMsg)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)

	resp := map[string]string{"error": message}
	json.NewEncoder(w).Encode(resp)
}
