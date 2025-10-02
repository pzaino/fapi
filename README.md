# FAPI (File API)

A simple file API for logs, tests and other file operations via REST API.

I use it to collect logs and test results from my integration tests running in other Docker containers. It may be useful to you as well.

## Features

- Generates files out of received data
- Each request creates a new file with a unique name
- Supports multiple endpoints for different file types (e.g., logs, test results)
- Health and readiness checks for container orchestration systems
- Has a "catch-all" endpoint so no need to modify your code and tests to use it

## Building

```bash
./autobuild.sh
```

This will create a `bin` directory with the compiled binaries.

## Using the healthCheck tool

### Health check for API container

./check --host=api --port=8989 --check=health

### Readiness check for API container

./check --host=api --port=8989 --check=readiness --timeout=2s
