# kurl - Kubernetes URL Tool

kurl is a command-line tool that allows you to make HTTP requests to Kubernetes services and pods using their internal DNS names, by automatically setting up port forwarding.

## Usage

```bash
kurl [options] <URL>
```

### Examples

#### Query a Kubernetes Service
```bash
# This will find pods selected by 'my-service' in 'default' namespace and port-forward to one of them
kurl http://my-service.default.svc:8080/api/endpoint

# With HTTPS and authentication
kurl -X POST -H 'Content-Type: application/json' -d '{"key":"value"}' https://my-service.default.svc:8443/api/endpoint
```

#### Query a Kubernetes Pod
```bash
# This will port-forward directly to the specified pod
kurl http://my-pod.default.pod:8080/api/endpoint
```

#### Common Options
- `-X, --request`: HTTP method to use (GET, POST, PUT, DELETE, etc.)
- `-H, --header`: HTTP headers to send with the request
- `-d, --data`: Data to send in the request body
- `-v, --verbose`: Enable verbose output
- `-k, --insecure`: Allow insecure SSL connections
- `-u, --user`: Server user and password for authentication
- `-L, --location`: Follow redirects

## How it Works

When you specify a service URL (like `service.namespace.svc:port`):
1. kurl parses the URL to extract service name, namespace, and port
2. It connects to your Kubernetes cluster (using your current kubeconfig context)
3. It fetches the service and extracts its selector labels
4. It finds pods in the namespace that match the selector
5. It establishes a port-forward connection to one of the matching pods
6. It forwards your HTTP request to the local port and returns the response

When you specify a pod URL (like `pod.namespace.pod:port`):
- It connects directly to the specified pod using port-forward

## Service Support Implementation

The key enhancement is that kurl now supports service URLs by:
- Identifying service resources (`.svc` in the hostname)
- Looking up the service in Kubernetes to get its selector
- Finding matching pods that the service would route to
- Establishing port-forward to one of those pods
- Allowing HTTP requests to work as if connecting to the service directly