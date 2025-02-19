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
	"regexp"
	"strings"
	"sync"
)

// Config holds the runtime configuration
type Config struct {
	IPv6Address       string
	IPv6Ports         []string
	IPv4Ports         []string
	FilePath          string
	DataDir           string
	WebhookToken      string
	WebhookListenPort string
	WebhookListenAddr string
	TunnelListenAddr  string
	mu                sync.RWMutex
}

// TunnelStatus represents the status of a tunnel. Used for the healthcheck
type TunnelStatus struct {
	IPv4Port  string `json:"ipv4_port"`
	IPv6Port  string `json:"ipv6_port"`
	IPv6Alive bool   `json:"ipv6_alive"`
}

func parseConfigEnv(envVar string, defaultValue string) string {
	env := os.Getenv(envVar)
	if env == "" {
		env = defaultValue // Default if not set
	}
	return env
}

// Forwards traffic between the source and destination connections
func forward(src, dst net.Conn) {
	defer src.Close()
	defer dst.Close()

	// Use io.Copy to forward data in both directions
	go io.Copy(src, dst)
	io.Copy(dst, src)
}

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

func (config *Config) loadIPv6Address() error {
	// Create a data/ dir if it's not existing to store the txt file
	err := os.MkdirAll(config.DataDir, os.ModePerm)
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

// Handles the webhook to update the IPv6 address
func updateIPv6Address(config *Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Check the token
		token := r.Header.Get("Authorization")
		if token != fmt.Sprintf("Bearer %s", config.WebhookToken) {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		bodyBytes, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "Failed to read request body", http.StatusInternalServerError)
			return
		}

		bodyString := string(bodyBytes)

		// What a wonderful regex stolen from https://stackoverflow.com/a/17871737
		ipv6RegEx := regexp.MustCompile(`(([0-9a-fA-F]{1,4}:){7,7}[0-9a-fA-F]{1,4}|([0-9a-fA-F]{1,4}:){1,7}:|([0-9a-fA-F]{1,4}:){1,6}:[0-9a-fA-F]{1,4}|([0-9a-fA-F]{1,4}:){1,5}(:[0-9a-fA-F]{1,4}){1,2}|([0-9a-fA-F]{1,4}:){1,4}(:[0-9a-fA-F]{1,4}){1,3}|([0-9a-fA-F]{1,4}:){1,3}(:[0-9a-fA-F]{1,4}){1,4}|([0-9a-fA-F]{1,4}:){1,2}(:[0-9a-fA-F]{1,4}){1,5}|[0-9a-fA-F]{1,4}:((:[0-9a-fA-F]{1,4}){1,6})|:((:[0-9a-fA-F]{1,4}){1,7}|:)|fe80:(:[0-9a-fA-F]{0,4}){0,4}%[0-9a-zA-Z]{1,}|::(ffff(:0{1,4}){0,1}:){0,1}((25[0-5]|(2[0-4]|1{0,1}[0-9]){0,1}[0-9])\.){3,3}(25[0-5]|(2[0-4]|1{0,1}[0-9]){0,1}[0-9])|([0-9a-fA-F]{1,4}:){1,4}:((25[0-5]|(2[0-4]|1{0,1}[0-9]){0,1}[0-9])\.){3,3}(25[0-5]|(2[0-4]|1{0,1}[0-9]){0,1}[0-9]))`)
		ipv6Addresses := ipv6RegEx.FindAllString(bodyString, -1)

		if len(ipv6Addresses) == 0 {
			http.Error(w, "Invalid request: the body did not contain an IPv6 address.", http.StatusBadRequest)
			log.Printf("Did not found a valid IPv6 address in the request body: '%s'", bodyString)
			return
		}

		// Always use the first matched address
		ipv6Address := ipv6Addresses[0]
		log.Printf("Found an IP address in the request body: %v", ipv6Address)

		// Disabled the proper JSON payload way for now because favonia/cloudflare-ddns only sends raw strings (even when they are sending a JSON content-type header)
		// // Parse the request jsonBody.
		// var jsonBody struct {
		// 	IPv6Address string `json:"ipv6_address"`
		// }

		// err = json.NewDecoder(r.Body).Decode(&jsonBody)
		// if err != nil {
		// log.Print("Request body does not match the expected JSON format")
		// }

		// Update the IPv6 address and save to disk
		config.mu.Lock()
		config.IPv6Address = ipv6Address
		config.mu.Unlock()

		err = config.saveIPv6Address()
		if err != nil {
			http.Error(w, "Failed to save IPv6 address", http.StatusInternalServerError)
			return
		}

		logLine := fmt.Sprintf("IPv6 address updated to %s", ipv6Address)
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, logLine)
		log.Print(logLine)
	}
}

// Checks if a connection to the IPv6 address and port is possible
func checkTunnel(ipv6Addr, port string) (bool, error) {
	conn, err := net.DialTimeout("tcp6", fmt.Sprintf("[%s]:%s", ipv6Addr, port), 2*1e9) // 2 seconds timeout
	if err != nil {
		return false, err
	}
	conn.Close()
	return true, nil
}

// Provides a health check for all open tunnels
func healthCheckHandler(config *Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		config.mu.RLock()
		defer config.mu.RUnlock()

		var statuses []TunnelStatus
		allHealthy := true

		for i, ipv4Port := range config.IPv4Ports {
			ipv6Port := config.IPv6Ports[i]
			ipv6Alive, err := checkTunnel(config.IPv6Address, ipv6Port)
			status := TunnelStatus{
				IPv4Port:  ipv4Port,
				IPv6Port:  ipv6Port,
				IPv6Alive: ipv6Alive,
			}
			statuses = append(statuses, status)

			if !ipv6Alive {
				allHealthy = false
				log.Printf("Healthcheck failed for port %v! %v", ipv6Port, err)
			}
		}

		if allHealthy {
			w.WriteHeader(http.StatusOK) // HTTP 200 if all tunnels are healthy
		} else {
			w.WriteHeader(http.StatusInternalServerError) // HTTP 500 if at least one tunnel is down
		}

		// Respond with JSON containing the tunnel statuses.
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(statuses)
	}
}

func main() {
	token := os.Getenv("WEBHOOK_TOKEN")
	if token == "" {
		log.Fatal("WEBHOOK_TOKEN environment variable not set")
	}

	srcPortsEnv := parseConfigEnv("SRC_PORTS", "8080")
	srcPorts := strings.Split(srcPortsEnv, ",")

	destPortsEnv := parseConfigEnv("DEST_PORTS", "8080")
	destPorts := strings.Split(destPortsEnv, ",")

	if len(srcPorts) != len(destPorts) {
		log.Fatalf("SRC_PORTS has a different length (%v) than DEST_PORTS (%v). Please make sure that both variables have the same amount of ports", len(srcPorts), len(destPorts))
	}

	sourceListenAddr := parseConfigEnv("SRC_LISTEN_ADDR", "0.0.0.0")

	webhookPort := parseConfigEnv("WEBHOOK_LISTEN_PORT", "8081")
	webhookAddr := parseConfigEnv("WEBHOOK_LISTEN_ADDR", "0.0.0.0")

	dataPath := "data" // Name of the data directory

	// Initial configuration
	config := &Config{
		IPv6Address:       "2001:db8::1", // Default IPv6 address
		IPv4Ports:         srcPorts,
		IPv6Ports:         destPorts,
		WebhookToken:      token,
		DataDir:           filepath.Join(".", dataPath),
		FilePath:          filepath.Join(dataPath, "ipv6_address.txt"),
		WebhookListenPort: webhookPort,
		WebhookListenAddr: webhookAddr,
		TunnelListenAddr:  sourceListenAddr,
	}

	// Load IPv6 address from the file if it exists
	if err := config.loadIPv6Address(); err != nil {
		log.Printf("Failed to load IPv6 address from file: %v. Using default (%s).", err, config.IPv6Address)
	}

	// Start the HTTP server to listen for webhook updates and health check
	http.HandleFunc("/update", updateIPv6Address(config))
	http.HandleFunc("/health", healthCheckHandler(config))
	go func() {
		fullAddr := fmt.Sprintf("%s:%s", config.WebhookListenAddr, config.WebhookListenPort)
		log.Printf("Starting webhook server on %s\n", fullAddr)
		log.Fatal(http.ListenAndServe(fullAddr, nil))
	}()

	for i, port := range config.IPv4Ports {
		go func(port string) {
			listener, err := net.Listen("tcp4", fmt.Sprintf("%s:%s", config.TunnelListenAddr, port))
			if err != nil {
				log.Fatalf("Error listening on IPv4 address %s port %s: %v", config.TunnelListenAddr, port, err)
			}

			defer listener.Close()
			log.Printf("Listening on %s:%s for IPv4 connections...\n", config.TunnelListenAddr, port)

			for {
				srcConn, err := listener.Accept()
				if err != nil {
					log.Printf("Error accepting connection: %v", err)
					continue
				}

				config.mu.RLock()
				ipv6Addr := config.IPv6Address
				// Use the destination port that is at the same index as the source port
				ipv6Port := config.IPv6Ports[i]
				config.mu.RUnlock()

				destConn, err := net.Dial("tcp6", fmt.Sprintf("[%s]:%s", ipv6Addr, ipv6Port))
				if err != nil {
					log.Printf("Error dialing IPv6 address %s port %s: %v", ipv6Addr, ipv6Port, err)
					srcConn.Close()
					continue
				}

				go forward(srcConn, destConn)
			}
		}(port)
	}

	// Keep the main goroutine running
	select {}
}
