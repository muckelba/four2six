package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sync"
)

// Config holds the runtime configuration.
type Config struct {
	IPv6Address     string
	IPv6Port        string
	IPv4Port        string
	FilePath        string
	ConfigDir       string
	WebhookToken    string
	WebhookListener string
	mu              sync.RWMutex
}

// forward forwards traffic between the source and destination connections.
func forward(src, dst net.Conn) {
	defer src.Close()
	defer dst.Close()

	// Use io.Copy to forward data in both directions.
	go io.Copy(src, dst)
	io.Copy(dst, src)
}

// saveIPv6Address saves the current IPv6 address to a file.
func (config *Config) saveIPv6Address() error {
	config.mu.RLock()
	defer config.mu.RUnlock()

	file, err := os.Create(config.FilePath)
	if err != nil {
		return err
	}
	defer file.Close()

	_, err = file.WriteString(config.IPv6Address)
	if err != nil {
		return err
	}

	return nil
}

// loadIPv6Address loads the IPv6 address from a file.
func (config *Config) loadIPv6Address() error {

	// Create a config/ dir if it's not existing
	err := os.MkdirAll(config.ConfigDir, os.ModePerm)
	if err != nil {
		return err
	}

	file, err := os.Open(config.FilePath)
	if err != nil {
		return err
	}
	defer file.Close()

	var ipv6Addr string
	_, err = fmt.Fscanf(file, "%s", &ipv6Addr)
	if err != nil {
		return err
	}

	config.mu.Lock()
	config.IPv6Address = ipv6Addr
	config.mu.Unlock()

	return nil
}

// updateIPv6Address handles the webhook to update the IPv6 address.
func updateIPv6Address(config *Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Check the token.
		token := r.Header.Get("Authorization")
		if token != fmt.Sprintf("Bearer %s", config.WebhookToken) {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		// Parse the request body.
		var body struct {
			IPv6Address string `json:"ipv6_address"`
		}

		err := json.NewDecoder(r.Body).Decode(&body)
		if err != nil || body.IPv6Address == "" {
			http.Error(w, "Invalid request", http.StatusBadRequest)
			return
		}

		// Update the IPv6 address and save to disk.
		config.mu.Lock()
		config.IPv6Address = body.IPv6Address
		config.mu.Unlock()

		err = config.saveIPv6Address()
		if err != nil {
			http.Error(w, "Failed to save IPv6 address", http.StatusInternalServerError)
			return
		}

		logLine := fmt.Sprintf("IPv6 address updated to %s", body.IPv6Address)
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, logLine)
		log.Print(logLine)
	}
}

func main() {
	token := os.Getenv("WEBHOOK_TOKEN")
	if token == "" {
		log.Fatal("WEBHOOK_TOKEN environment variable not set")
	}

	ipv6DestinationPort := os.Getenv("DEST_PORT")
	if ipv6DestinationPort == "" {
		ipv6DestinationPort = "8080" // Default destination port if not set.
	}

	ipv4SourcePort := os.Getenv("SRC_PORT")
	if ipv4SourcePort == "" {
		ipv4SourcePort = ":8080" // Default source port if not set.
	}

	webhookListener := os.Getenv("WEBHOOK_LISTENER")
	if webhookListener == "" {
		webhookListener = ":8081" // Default webhook listener port.
	}

	configPath := "config" // Name of the config directory

	// Initial configuration.
	config := &Config{
		IPv6Address:     "2001:db8::1", // Default IPv6 address.
		IPv6Port:        ipv6DestinationPort,
		WebhookToken:    token,
		ConfigDir:       filepath.Join(".", configPath),
		FilePath:        filepath.Join(configPath, "ipv6_address.txt"),
		WebhookListener: webhookListener,
		IPv4Port:        ipv4SourcePort,
	}

	// Load IPv6 address from the file if it exists.
	if err := config.loadIPv6Address(); err != nil {
		log.Printf("Failed to load IPv6 address from file: %v. Using default.", err)
	}

	// Start the HTTP server to listen for webhook updates.
	http.HandleFunc("/update", updateIPv6Address(config))
	go func() {
		log.Printf("Starting webhook server on %s\n", config.WebhookListener)
		log.Fatal(http.ListenAndServe(config.WebhookListener, nil))
	}()

	// Listen for incoming connections on the IPv4 address and port.
	listener, err := net.Listen("tcp4", config.IPv4Port) // Listening on IPv4, specified port.
	if err != nil {
		log.Fatalf("Error listening on IPv4 address: %v", err)
	}
	defer listener.Close()
	log.Printf("Listening on %s for IPv4 connections...\n", config.IPv4Port)

	for {
		// Accept incoming connections.
		srcConn, err := listener.Accept()
		if err != nil {
			log.Printf("Error accepting connection: %v", err)
			continue
		}

		// Get the current IPv6 address in a thread-safe way.
		config.mu.RLock()
		ipv6Addr := config.IPv6Address
		port := config.IPv6Port
		config.mu.RUnlock()

		// Dial the destination IPv6 address and port.
		dstConn, err := net.Dial("tcp6", fmt.Sprintf("[%s]:%s", ipv6Addr, port))
		if err != nil {
			log.Printf("Error dialing IPv6 address: %v", err)
			srcConn.Close()
			continue
		}

		// Forward traffic between the two connections.
		go forward(srcConn, dstConn)
	}
}
