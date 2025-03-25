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
		http.Error(w, "Only POST allowed", http.StatusMethodNotAllowed)
		return
	}

	// Read the body
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Failed to read body", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	// Validate JSON
	var js map[string]interface{}
	if err := json.Unmarshal(body, &js); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	// Get client IP
	ip := getClientIP(r)
	if ip == "" {
		http.Error(w, "Could not determine client IP", http.StatusInternalServerError)
		return
	}

	// Create file name
	now := time.Now().UTC()
	timestamp := now.Format("2006-01-02-15_04_05")
	filename := fmt.Sprintf("%s-%s.json", ip, timestamp)
	filepath := filepath.Join(uploadDir, filename)

	// Save the file concurrently
	go func(data []byte, path string) {
		f, err := os.Create(path)
		if err != nil {
			fmt.Printf("Failed to create file: %v\n", err)
			return
		}
		defer f.Close()

		buf := bufferPool.Get().(*bufio.Writer)
		buf.Reset(f)
		defer bufferPool.Put(buf)

		if _, err := buf.Write(data); err != nil {
			fmt.Printf("Failed to write file: %v\n", err)
			return
		}
		buf.Flush()
	}(body, filepath)

	w.WriteHeader(http.StatusAccepted)
	_, err = w.Write([]byte("JSON stored\n"))
	if err != nil {
		fmt.Printf("Failed to write response: %v\n", err)
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
