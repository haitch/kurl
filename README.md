# kurl - Kubernetes URL Tool

kurl is a command-line tool that allows you to make HTTP requests to Kubernetes services and pods using their internal DNS names, by automatically setting up port forwarding.

## Usage

```bash
kurl [curl options] <kubernetes URL>
```

kurl will pass all options to curl, it would first do portforward (inference active kubeconfig, use export KUBECONFIG if you need to), then update url to localhost url, invoke curl with localhost url.

### Examples

```bash 
kurl -X POST -H 'Content-Type: application/json' -d '{"key":"value"}' http://my-service.my-namespace.svc:8080/api/endpoint
```

would translate to
```bash
kubectl -n my-namespace port-forward svc/my-service xxx:8080
curl -X POST -H 'Content-Type: application/json' -d '{"key":"value"}' http://localhost:xxx
```

## Requirements

- Go (for building)
- Access to a Kubernetes cluster with kubeconfig configured (typically at ~/.kube/config)
- curl (optional, for enhanced functionality; falls back to built-in client if not available)