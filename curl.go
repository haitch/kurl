package main

import (
	"crypto/tls"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

// makeHTTPRequest handles the actual HTTP request with all the specified options
func makeHTTPRequest(url string, method string, headers []string, data string, dataAscii string, dataBinary string, 
	form []string, verbose bool, insecure bool, user string, timeout int, followRedirects bool, maxRedirects int,
	userAgent string, includeHeaders bool, onlyHeaders bool, output string) error {
	
	// Determine request body
	var requestBody io.Reader
	if data != "" {
		requestBody = strings.NewReader(data)
	} else if dataAscii != "" {
		requestBody = strings.NewReader(dataAscii)  // Same as -d for ASCII data
	} else if dataBinary != "" {
		requestBody = strings.NewReader(dataBinary)  // Same as -d for binary data (as string)
	}
	
	// Handle form data
	if len(form) > 0 {
		// Simple implementation: join form data with &
		formData := strings.Join(form, "&")
		requestBody = strings.NewReader(formData)
		if method == "GET" || method == "HEAD" {
			method = "POST" // Form submission defaults to POST
		}
		// Add content-type for form data
		headers = append(headers, "Content-Type: application/x-www-form-urlencoded")
	}
	
	// Create the HTTP request
	req, err := http.NewRequest(method, url, requestBody)
	if err != nil {
		return fmt.Errorf("error creating request: %v", err)
	}

	// Add headers
	for _, header := range headers {
		parts := strings.SplitN(header, ":", 2)
		if len(parts) == 2 {
			req.Header.Add(strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1]))
		}
	}
	
	// Add Authorization header if user is specified
	if user != "" {
		parts := strings.SplitN(user, ":", 2)
		var username, password string
		if len(parts) == 2 {
			username, password = parts[0], parts[1]
		} else {
			username = parts[0]
			// If no password provided, use empty string
			password = ""
		}
		req.SetBasicAuth(username, password)
	}
	
	// Add User-Agent header if specified
	if userAgent != "" {
		req.Header.Set("User-Agent", userAgent)
	}

	// Enable verbose output if requested
	if verbose {
		fmt.Printf("Making request: %s %s\n", method, url)
		fmt.Printf("Headers: %v\n", req.Header)
		if requestBody != nil {
			// Note: Reading the request body for verbose output might alter it, 
			// so we just indicate that a body was provided
			fmt.Printf("Request body provided\n")
		}
	}

	// Create HTTP client
	client := &http.Client{}
	
	// Configure insecure SSL if requested
	if insecure {
		client.Transport = &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		}
	}
	
	// Configure timeout if specified
	if timeout > 0 {
		client.Timeout = time.Duration(timeout) * time.Second
	}
	
	// Configure redirect behavior
	if !followRedirects {
		client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse // Don't follow redirects
		}
	} else if maxRedirects >= 0 {
		client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
			if len(via) >= maxRedirects {
				return http.ErrUseLastResponse
			}
			return nil
		}
	}

	// Execute the HTTP request
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("error executing request: %v", err)
	}
	defer resp.Body.Close()

	// Determine output destination
	var outputWriter io.Writer = os.Stdout
	if output != "" {
		file, err := os.Create(output)
		if err != nil {
			return fmt.Errorf("error creating output file %s: %v", output, err)
		}
		defer file.Close()
		outputWriter = file
	}

	// Output response headers if requested
	if includeHeaders || onlyHeaders {
		for name, values := range resp.Header {
			for _, value := range values {
				fmt.Fprintf(outputWriter, "%s: %s\r\n", name, value)
			}
		}
		if includeHeaders {
			fmt.Fprintf(outputWriter, "\r\n")  // Add empty line between headers and body
		}
	}

	// Copy response to output writer (or skip if only headers requested)
	if !onlyHeaders {
		_, err = io.Copy(outputWriter, resp.Body)
		if err != nil {
			return fmt.Errorf("error reading response: %v", err)
		}
	}

	// Print response status if verbose
	if verbose {
		fmt.Printf("\nResponse Status: %s\n", resp.Status)
		fmt.Printf("Response Headers: %v\n", resp.Header)
	}

	return nil
}