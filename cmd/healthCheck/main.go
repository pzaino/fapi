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

// Package main provides a simple health and readiness check utility for Docker containers.
package main

import (
	"flag"
	"fmt"
	"net/http"
	"os"
	"time"
)

var (
	host      string
	port      int
	useSSL    bool
	checkType string
	timeout   time.Duration
)

// genURL builds the URL for a health or readiness check
func genURL(checkType string) string {
	scheme := "http"
	if useSSL {
		scheme = "https"
	}

	path := "/v1/health"
	if checkType == "readiness" {
		path = "/v1/readiness"
	}

	return fmt.Sprintf("%s://%s:%d%s", scheme, host, port, path)
}

// check performs a GET request against the given URL and returns true if 200 OK
func check(url string) bool {
	client := &http.Client{Timeout: timeout}
	resp, err := client.Get(url)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error connecting to %s: %v\n", url, err)
		return false
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		fmt.Fprintf(os.Stderr, "Check failed for %s: status %d\n", url, resp.StatusCode)
		return false
	}

	return true
}

func main() {
	// Flags
	flag.StringVar(&host, "host", "localhost", "Host of the service")
	flag.IntVar(&port, "port", 8080, "Port of the service")
	flag.BoolVar(&useSSL, "ssl", false, "Use HTTPS instead of HTTP")
	flag.StringVar(&checkType, "check", "health", "Type of check: health or readiness")
	flag.DurationVar(&timeout, "timeout", 5*time.Second, "HTTP timeout")
	flag.Parse()

	// Build URL and perform check
	url := genURL(checkType)
	if !check(url) {
		os.Exit(1)
	}

	fmt.Printf("%s check succeeded for %s\n", checkType, url)
	os.Exit(0)
}
