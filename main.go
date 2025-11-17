package main

import (
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"strconv"
	"strings"
)

func main() {
	// Parse command line arguments, identifying the URL (which should be the last non-flag argument)
	// and separating flags that affect kurl's behavior from those that go to curl
	args := os.Args[1:] // Skip the program name

	if len(args) < 1 {
		fmt.Println("Usage: kurl [curl options] <Kubernetes service URL>")
		fmt.Println("Options will be passed through to curl when available")
		fmt.Println("Example: kurl -X POST -H 'Content-Type: application/json' -d '{\"key\":\"value\"}' http://mysvc.mynamespace.svc:8080/api/resource")
		os.Exit(1)
	}

	// Identify the URL (last argument that looks like a URL)
	serviceURL := ""
	urlIndex := -1

	// Look for the URL from the end of the arguments
	for i := len(args) - 1; i >= 0; i-- {
		arg := args[i]
		if isURL(arg) {
			serviceURL = arg
			urlIndex = i
			break
		}
	}

	if serviceURL == "" {
		fmt.Println("Error: No Kubernetes service URL found in arguments")
		fmt.Println("URLs should follow the format: http://service.namespace.svc:port")
		os.Exit(1)
	}

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

	// Determine if verbose mode is enabled by checking if -v or --verbose is in the args
	verbose := containsFlag(args, "-v", "--verbose")

	if curlAvailable {
		// Use system curl with port-forward
		runWithSystemCurlNew(res, localPort, serviceURL, args[:urlIndex], verbose)
	} else {
		// Fall back to current implementation
		runWithCustomHTTPNew(res, localPort, serviceURL, args[:urlIndex], verbose)
	}
}

// isURL checks if a string looks like a URL
func isURL(s string) bool {
	// Simple check for URLs starting with http:// or https://
	return strings.HasPrefix(s, "http://") || strings.HasPrefix(s, "https://")
}

// containsFlag checks if any of the provided arguments contains any of the specified flags
func containsFlag(args []string, shortFlag string, longFlag string) bool {
	for _, arg := range args {
		if arg == shortFlag || arg == longFlag {
			return true
		}
		// Check for flags with values like -H "header" or --header "header"
		if strings.HasPrefix(arg, shortFlag+"=") || strings.HasPrefix(arg, longFlag+"=") {
			return true
		}
	}
	return false
}

// isCurlAvailable checks if curl is installed on the system
func isCurlAvailable() bool {
	_, err := exec.LookPath("curl")
	return err == nil
}

// runWithSystemCurlNew executes the port forward and uses system curl with the original args
func runWithSystemCurlNew(res *forwardTarget, localPort int, serviceURL string, originalArgs []string, verbose bool) {
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
	if verbose {
		fmt.Printf("Setting up port-forward from local port %d to %s\n", localPort, res.String())
	}

	// Construct the local URL for the HTTP request
	localURL := reconstructURL(serviceURL, localPort)

	// Build the curl command using the original args with the new local URL
	curlCmd := buildCurlCommandFromArgs(originalArgs, localURL)

	// If verbose flag is passed, print the actual raw curl command we invoked
	if verbose {
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

// runWithCustomHTTPNew executes the port forward and uses custom HTTP client with selected args only
func runWithCustomHTTPNew(res *forwardTarget, localPort int, serviceURL string, originalArgs []string, verbose bool) {
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

	// Extract flags that affect HTTP request from original arguments for fallback HTTP client
	method := extractMethod(originalArgs)
	headers := extractHeaders(originalArgs)
	data, dataAscii, dataBinary := extractData(originalArgs)
	form := extractForm(originalArgs)
	user := extractUser(originalArgs)
	timeout := extractTimeout(originalArgs)
	userAgent := extractUserAgent(originalArgs)
	insecure := containsFlag(originalArgs, "-k", "--insecure")
	followRedirects := containsFlag(originalArgs, "-L", "--location")
	include := containsFlag(originalArgs, "-i", "--include")
	onlyHeaders := containsFlag(originalArgs, "-I", "--head")

	// Make the HTTP request using the custom HTTP module
	err := makeHTTPRequest(localURL, method, headers, data, dataAscii, dataBinary,
		form, verbose, insecure, user, timeout, followRedirects, -1, // maxRedirects not implemented for fallback
		userAgent, include, onlyHeaders, "") // output to stdout, not file for fallback
	if err != nil {
		fmt.Printf("Error making HTTP request: %v\n", err)
		close(stopCh)
		os.Exit(1)
	}

	// Close the stop channel to terminate port-forward
	close(stopCh)
}

// buildCurlCommandFromArgs builds a curl command from original arguments, replacing the URL
func buildCurlCommandFromArgs(originalArgs []string, newURL string) string {
	// Start with the curl command
	args := []string{"curl"}

	// Add all original arguments (they will be properly escaped)
	for _, arg := range originalArgs {
		args = append(args, shellEscape(arg))
	}

	// Add the new local URL
	args = append(args, shellEscape(newURL))

	return strings.Join(args, " ")
}

// Helper functions to extract specific flags from arguments for fallback HTTP client
func extractMethod(args []string) string {
	for i, arg := range args {
		if arg == "-X" || arg == "--request" {
			if i+1 < len(args) {
				return args[i+1]
			}
		}
		// Handle -X=method format
		if strings.HasPrefix(arg, "-X=") || strings.HasPrefix(arg, "--request=") {
			parts := strings.SplitN(arg, "=", 2)
			if len(parts) == 2 {
				return parts[1]
			}
		}
	}
	return "GET" // default
}

func extractHeaders(args []string) []string {
	var headers []string
	for i, arg := range args {
		if arg == "-H" || arg == "--header" {
			if i+1 < len(args) {
				headers = append(headers, args[i+1])
			}
		}
		// Handle -H=header format
		if strings.HasPrefix(arg, "-H=") || strings.HasPrefix(arg, "--header=") {
			parts := strings.SplitN(arg, "=", 2)
			if len(parts) == 2 {
				headers = append(headers, parts[1])
			}
		}
	}
	return headers
}

func extractData(args []string) (string, string, string) {
	var data, dataAscii, dataBinary string
	for i, arg := range args {
		if arg == "-d" || arg == "--data" {
			if i+1 < len(args) {
				data = args[i+1]
			}
		} else if arg == "--data-ascii" {
			if i+1 < len(args) {
				dataAscii = args[i+1]
			}
		} else if arg == "--data-binary" {
			if i+1 < len(args) {
				dataBinary = args[i+1]
			}
		}
		// Handle = format
		if strings.HasPrefix(arg, "-d=") || strings.HasPrefix(arg, "--data=") {
			parts := strings.SplitN(arg, "=", 2)
			if len(parts) == 2 {
				data = parts[1]
			}
		} else if strings.HasPrefix(arg, "--data-ascii=") {
			parts := strings.SplitN(arg, "=", 2)
			if len(parts) == 2 {
				dataAscii = parts[1]
			}
		} else if strings.HasPrefix(arg, "--data-binary=") {
			parts := strings.SplitN(arg, "=", 2)
			if len(parts) == 2 {
				dataBinary = parts[1]
			}
		}
	}
	return data, dataAscii, dataBinary
}

func extractForm(args []string) []string {
	var forms []string
	for i, arg := range args {
		if arg == "-F" || arg == "--form" {
			if i+1 < len(args) {
				forms = append(forms, args[i+1])
			}
		}
		// Handle = format
		if strings.HasPrefix(arg, "-F=") || strings.HasPrefix(arg, "--form=") {
			parts := strings.SplitN(arg, "=", 2)
			if len(parts) == 2 {
				forms = append(forms, parts[1])
			}
		}
	}
	return forms
}

func extractUser(args []string) string {
	for i, arg := range args {
		if arg == "-u" || arg == "--user" {
			if i+1 < len(args) {
				return args[i+1]
			}
		}
		// Handle = format
		if strings.HasPrefix(arg, "-u=") || strings.HasPrefix(arg, "--user=") {
			parts := strings.SplitN(arg, "=", 2)
			if len(parts) == 2 {
				return parts[1]
			}
		}
	}
	return ""
}

func extractTimeout(args []string) int {
	for i, arg := range args {
		if arg == "-m" || arg == "--max-time" {
			if i+1 < len(args) {
				if timeout, err := strconv.Atoi(args[i+1]); err == nil {
					return timeout
				}
			}
		}
		// Handle = format
		if strings.HasPrefix(arg, "-m=") || strings.HasPrefix(arg, "--max-time=") {
			parts := strings.SplitN(arg, "=", 2)
			if len(parts) == 2 {
				if timeout, err := strconv.Atoi(parts[1]); err == nil {
					return timeout
				}
			}
		}
	}
	return 0
}

func extractUserAgent(args []string) string {
	for i, arg := range args {
		if arg == "-A" || arg == "--user-agent" {
			if i+1 < len(args) {
				return args[i+1]
			}
		}
		// Handle = format
		if strings.HasPrefix(arg, "-A=") || strings.HasPrefix(arg, "--user-agent=") {
			parts := strings.SplitN(arg, "=", 2)
			if len(parts) == 2 {
				return parts[1]
			}
		}
	}
	return ""
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
