# FAPI (File API)

A simple file API for logs, tests and other file operations via REST API.

## Features

- Upload files

## Building

```bash
./autobuild.sh
```

This will create a `bin` directory with the compiled binaries.

## Using the healthCheck tool

### Health check for API container

./check --host=api --port=8080 --check=health

### Readiness check for DB proxy

./check --host=db-proxy --port=5432 --check=readiness --timeout=2s
