package main

import (
	"fmt"
	"net/url"
	"os"
	"os/exec"
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

	// Check if curl is available
	curlAvailable := isCurlAvailable()

	if curlAvailable {
		// Use system curl with port-forward
		runWithSystemCurl(res, localPort, serviceURL, method, headers, data, dataAscii, dataBinary,
			form, verbose, insecure, user, timeout, followRedirects, maxRedirects,
			userAgent, include, onlyHeaders, output)
	} else {
		// Fall back to current implementation
		runWithCustomHTTP(res, localPort, serviceURL, method, headers, data, dataAscii, dataBinary,
			form, verbose, insecure, user, timeout, followRedirects, maxRedirects,
			userAgent, include, onlyHeaders, output)
	}
}

// isCurlAvailable checks if curl is installed on the system
func isCurlAvailable() bool {
	_, err := exec.LookPath("curl")
	return err == nil
}

// runWithSystemCurl executes the port forward and uses system curl to make the HTTP request
func runWithSystemCurl(res *forwardTarget, localPort int, serviceURL string, method *string,
	headers *[]string, data *string, dataAscii *string, dataBinary *string, form *[]string,
	verbose *bool, insecure *bool, user *string, timeout *int, followRedirects *bool,
	maxRedirects *int, userAgent *string, include *bool, onlyHeaders *bool, output *string) {

	// Convert resource to ForwardTarget for port forwarding
	forwardTarget := &ForwardTarget{
		Name:      res.name,
		Namespace: res.namespace,
		Kind:      res.kind,
		Port:      res.port,
	}

	// Create channels for port-forward control
	stopCh := make(chan struct{}, 1)
	readyCh := make(chan struct{}, 1)

	// Start port-forward in a goroutine
	go func() {
		err := runPortForward(forwardTarget, localPort, stopCh, readyCh)
		if err != nil {
			fmt.Printf("Error in port-forward: %v\n", err)
			os.Exit(1)
		}
	}()

	// Wait for port-forward to be ready
	<-readyCh

	// If verbose flag is passed, print which pod we are going to port forward and which local port
	if *verbose {
		fmt.Printf("Setting up port-forward from local port %d to %s\n", localPort, res.String())
	}

	// Construct the local URL for the HTTP request
	localURL := reconstructURL(serviceURL, localPort)

	// Set request method to HEAD if --head flag is used
	if *onlyHeaders {
		*method = "HEAD"
	}

	// Build the curl command
	curlCmd := buildCurlCommand(localURL, *method, *headers, *data, *dataAscii, *dataBinary,
		*form, *verbose, *insecure, *user, *timeout, *followRedirects, *maxRedirects,
		*userAgent, *include, *onlyHeaders, *output)

	// If verbose flag is passed, print the actual raw curl command we invoked
	if *verbose {
		fmt.Printf("Executing curl command: %s\n", curlCmd)
	}

	// Execute the curl command
	cmd := exec.Command("sh", "-c", curlCmd)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin

	err := cmd.Run()
	if err != nil {
		fmt.Printf("Error executing curl command: %v\n", err)
		close(stopCh)
		os.Exit(1)
	}

	// Close the stop channel to terminate port-forward
	close(stopCh)
}

// runWithCustomHTTP executes the port forward and uses custom HTTP client (fallback)
func runWithCustomHTTP(res *forwardTarget, localPort int, serviceURL string, method *string,
	headers *[]string, data *string, dataAscii *string, dataBinary *string, form *[]string,
	verbose *bool, insecure *bool, user *string, timeout *int, followRedirects *bool,
	maxRedirects *int, userAgent *string, include *bool, onlyHeaders *bool, output *string) {

	// Convert resource to ForwardTarget for port forwarding
	forwardTarget := &ForwardTarget{
		Name:      res.name,
		Namespace: res.namespace,
		Kind:      res.kind,
		Port:      res.port,
	}

	// Create channels for port-forward control
	stopCh := make(chan struct{}, 1)
	readyCh := make(chan struct{}, 1)

	// Start port-forward in a goroutine
	go func() {
		err := runPortForward(forwardTarget, localPort, stopCh, readyCh)
		if err != nil {
			fmt.Printf("Error in port-forward: %v\n", err)
			os.Exit(1)
		}
	}()

	// Wait for port-forward to be ready
	<-readyCh
	fmt.Printf("Port-forward established. Forwarding to localhost:%d\n", localPort)

	// Construct the local URL for the HTTP request
	localURL := reconstructURL(serviceURL, localPort)

	// Set request method to HEAD if --head flag is used
	if *onlyHeaders {
		*method = "HEAD"
	}

	// Make the HTTP request using the custom HTTP module
	err := makeHTTPRequest(localURL, *method, *headers, *data, *dataAscii, *dataBinary,
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

// buildCurlCommand constructs the curl command with all the specified options
func buildCurlCommand(url string, method string, headers []string, data string, dataAscii string, dataBinary string,
	form []string, verbose bool, insecure bool, user string, timeout int, followRedirects bool, maxRedirects int,
	userAgent string, includeHeaders bool, onlyHeaders bool, output string) string {

	args := []string{"curl"}

	// Add method
	if method != "GET" {
		args = append(args, "-X", shellEscape(method))
	}

	// Add headers
	for _, header := range headers {
		args = append(args, "-H", shellEscape(header))
	}

	// Add data
	if data != "" {
		args = append(args, "-d", shellEscape(data))
	} else if dataAscii != "" {
		args = append(args, "--data-ascii", shellEscape(dataAscii))
	} else if dataBinary != "" {
		args = append(args, "--data-binary", shellEscape(dataBinary))
	}

	// Add form data
	for _, formItem := range form {
		args = append(args, "-F", shellEscape(formItem))
	}

	// Add verbose flag
	if verbose {
		args = append(args, "-v")
	}

	// Add insecure flag
	if insecure {
		args = append(args, "-k")
	}

	// Add user authentication
	if user != "" {
		args = append(args, "-u", shellEscape(user))
	}

	// Add timeout
	if timeout > 0 {
		args = append(args, "-m", fmt.Sprintf("%d", timeout))
	}

	// Add follow redirects
	if followRedirects {
		args = append(args, "-L")
	}

	// Add max redirects
	if maxRedirects >= 0 {
		args = append(args, "--max-redirs", fmt.Sprintf("%d", maxRedirects))
	}

	// Add user agent
	if userAgent != "" {
		args = append(args, "-A", shellEscape(userAgent))
	}

	// Add output file
	if output != "" {
		args = append(args, "-o", shellEscape(output))
	}

	// Add include headers
	if includeHeaders {
		args = append(args, "-i")
	}

	// Add head (only headers)
	if onlyHeaders {
		args = append(args, "-I")
	}

	// Add the URL
	args = append(args, shellEscape(url))

	return strings.Join(args, " ")
}

// shellEscape escapes a string for use in a shell command
func shellEscape(s string) string {
	// Simple shell escaping by wrapping in single quotes and escaping any single quotes within
	if s == "" {
		return "''"
	}

	// Replace single quotes with '\'', then wrap the whole string in single quotes
	escaped := strings.ReplaceAll(s, "'", "'\"'\"'")
	return "'" + escaped + "'"
}

// reconstructURL properly reconstructs the URL to use localhost and the local port
func reconstructURL(originalURL string, localPort int) string {
	parsedURL, err := url.Parse(originalURL)
	if err != nil {
		// If we can't parse the URL, return the original
		return originalURL
	}

	// Reconstruct the URL with localhost and localPort
	newURL := &url.URL{
		Scheme:   parsedURL.Scheme,
		Host:     fmt.Sprintf("localhost:%d", localPort),
		Path:     parsedURL.Path,
		RawQuery: parsedURL.RawQuery,
		Fragment: parsedURL.Fragment,
	}

	return newURL.String()
}
