package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/pflag"
)

func main() {
	// Define command-line flags using pflag
	method := pflag.StringP("request", "X", "GET", "HTTP method to use (GET, POST, PUT, DELETE, etc.)")
	headers := pflag.StringArrayP("header", "H", []string{}, "HTTP headers to send with the request")
	data := pflag.StringP("data", "d", "", "Data to send in the request body")
	dataAscii := pflag.StringP("data-ascii", "", "", "HTTP POST data (same as -d, but only ASCII characters)")
	dataBinary := pflag.StringP("data-binary", "", "", "HTTP POST data exactly as specified in the string")
	form := pflag.StringArrayP("form", "F", []string{}, "Submit form data")
	verbose := pflag.BoolP("verbose", "v", false, "Enable verbose output")
	insecure := pflag.BoolP("insecure", "k", false, "Allow insecure SSL connections")
	user := pflag.StringP("user", "u", "", "Server user and password")
	timeout := pflag.IntP("max-time", "m", 0, "Maximum time in seconds allowed for the operation")
	followRedirects := pflag.BoolP("location", "L", false, "Follow redirects")
	maxRedirects := pflag.Int("max-redirs", -1, "Maximum number of redirects allowed (-1 for default)")
	userAgent := pflag.StringP("user-agent", "A", "", "User-Agent header to send")
	output := pflag.StringP("output", "o", "", "Write output to file instead of stdout")
	include := pflag.BoolP("include", "i", false, "Include response headers in output")
	onlyHeaders := pflag.BoolP("head", "I", false, "Fetch headers only (same as -X HEAD)")

	// Parse flags
	pflag.Parse()

	// Get the URL (first non-flag argument)
	args := pflag.Args()
	if len(args) < 1 {
		fmt.Println("Usage: kurl [options] <URL>")
		fmt.Println("Options:")
		pflag.PrintDefaults()
		fmt.Println("Example: kurl -X POST -H 'Content-Type: application/json' -d '{\"key\":\"value\"}' http://mysvc.mynamespace.svc:8080/api/resource")
		os.Exit(1)
	}

	serviceURL := args[0]

	// Parse the URL and extract service information
	res, err := parseKubernetesServiceURL(serviceURL)
	if err != nil {
		fmt.Printf("Error parsing service URL: %v\n", err)
		os.Exit(1)
	}

	// Find a free local port
	localPort, err := findFreePort()
	if err != nil {
		fmt.Printf("Error finding free port: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Setting up port-forward from local port %d to %s\n", localPort, res.String())

	// Create channels for port-forward control
	stopCh := make(chan struct{}, 1)
	readyCh := make(chan struct{}, 1)

	// Start port-forward in a goroutine
	go func() {
		err := runPortForward(res, localPort, stopCh, readyCh)
		if err != nil {
			fmt.Printf("Error in port-forward: %v\n", err)
			os.Exit(1)
		}
	}()

	// Wait for port-forward to be ready
	<-readyCh
	fmt.Printf("Port-forward established. Forwarding to localhost:%d\n", localPort)

	// Construct the local URL for the HTTP request
	localURL := strings.Replace(serviceURL, fmt.Sprintf("%s.%s.svc", res.name, res.namespace), "localhost", 1)
	localURL = strings.Replace(localURL, fmt.Sprintf("%s.%s.svc.cluster.local", res.name, res.namespace), "localhost", 1)
	localURL = strings.Replace(serviceURL, fmt.Sprintf("%s.%s.pod", res.name, res.namespace), "localhost", 1)
	localURL = strings.Replace(localURL, fmt.Sprintf("%s.%s.pod.cluster.local", res.name, res.namespace), "localhost", 1)
	localURL = strings.Replace(localURL, fmt.Sprintf(":%d", res.port), fmt.Sprintf(":%d", localPort), 1)
	fmt.Printf("curl against local url: %s", localURL)

	// Set request method to HEAD if --head flag is used
	if *onlyHeaders {
		*method = "HEAD"
	}

	// Make the HTTP request using the curl module
	err = makeHTTPRequest(localURL, *method, *headers, *data, *dataAscii, *dataBinary,
		*form, *verbose, *insecure, *user, *timeout, *followRedirects, *maxRedirects,
		*userAgent, *include, *onlyHeaders, *output)
	if err != nil {
		fmt.Printf("Error making HTTP request: %v\n", err)
		close(stopCh)
		os.Exit(1)
	}

	// Close the stop channel to terminate port-forward
	close(stopCh)
}
