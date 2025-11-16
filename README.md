# kurl

`kurl` is a simple command-line tool that combines Kubernetes port-forwarding and HTTP requests to make API calls to Kubernetes services without having to manually set up port-forwarding.

## Installation

First, make sure you have Go installed, then build the binary:

```bash
cd kurl
go build -o kurl
```

Optionally, you can move the binary to a location in your PATH:

```bash
sudo mv kurl /usr/local/bin/
```

## Usage

The basic usage pattern is:

```bash
kurl [options] <Kubernetes service URL>
```

The Kubernetes service URL should be in the format:
- `http://service.namespace.svc:port`
- `http://service.namespace.svc.cluster.local:port`

## Supported Options

`kurl` supports common curl-like options:

- `-X, --request`: HTTP method to use (GET, POST, PUT, DELETE, etc.) [default: GET]
- `-H, --header`: HTTP headers to send with the request (can be used multiple times)
- `-d, --data`: Data to send in the request body
- `--data-ascii`: HTTP POST data (same as -d, but only ASCII characters)
- `--data-binary`: HTTP POST data exactly as specified in the string
- `-F, --form`: Submit form data
- `-v, --verbose`: Enable verbose output
- `-k, --insecure`: Allow insecure SSL connections
- `-u, --user`: Server user and password for authentication
- `-m, --max-time`: Maximum time in seconds allowed for the operation
- `-L, --location`: Follow redirects
- `--max-redirs`: Maximum number of redirects allowed
- `-A, --user-agent`: User-Agent header to send
- `-o, --output`: Write output to file instead of stdout
- `-i, --include`: Include response headers in output
- `-I, --head`: Fetch headers only (same as -X HEAD)

## Examples

```bash
# Simple GET request
kurl http://mysvc.mynamespace.svc:8080

# POST request with headers and data
kurl -X POST -H "Content-Type: application/json" -d '{"key":"value"}' http://mysvc.mynamespace.svc:8080/api/v1/resource

# GET with query parameters
kurl http://mysvc.mynamespace.svc:8080/api/v1/resource?param=value

# Verbose request
kurl -v http://mysvc.mynamespace.svc:8080

# Request with headers
kurl -H "Authorization: Bearer token" -H "Custom-Header: value" http://mysvc.mynamespace.svc:8080

# PUT request
kurl -X PUT -d '{"id": 1, "name": "example"}' http://mysvc.mynamespace.svc:8080/api/v1/resource/1

# Get only headers
kurl -I http://mysvc.mynamespace.svc:8080

# Include response headers with body
kurl -i http://mysvc.mynamespace.svc:8080

# Submit form data
kurl -F "name=John" -F "email=john@example.com" http://mysvc.mynamespace.svc:8080/form

# Set a timeout
kurl -m 30 http://mysvc.mynamespace.svc:8080

# Output to file
kurl -o response.json http://mysvc.mynamespace.svc:8080/data
```

## How It Works

The `kurl` command does the following:

1. Parses the Kubernetes service URL to extract the service name, namespace, and port
2. Uses your local kubeconfig to authenticate with the Kubernetes cluster
3. Finds an available local port
4. Sets up internal port-forward from the local port to the Kubernetes service
5. Makes HTTP requests to the local forwarded port with specified options
6. Cleans up the port-forward when done

## Requirements

- Go (for building)
- Access to a Kubernetes cluster with kubeconfig configured (typically at ~/.kube/config)
- The binary can be run without external dependencies like kubectl or curl