package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"time"
)

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	http.HandleFunc("/", handleIndex)
	http.HandleFunc("/health", handleHealth)

	log.Printf("API server listening on :%s\n", port)
	if err := http.ListenAndServe(":"+port, nil); err != nil {
		log.Fatal(err)
	}
}

func handleIndex(w http.ResponseWriter, r *http.Request) {
	hostname, _ := os.Hostname()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"service":  "api",
		"hostname": hostname,
		"status":   "ok",
	})
}

func handleHealth(w http.ResponseWriter, r *http.Request) {
	resp := map[string]any{
		"status":    "ok",
		"timestamp": time.Now().UTC().Format(time.RFC3339),
		"server_ip": getServerIP(),
	}

	dbHost := os.Getenv("DB_HOST")
	if dbHost != "" {
		dbPort := os.Getenv("DB_PORT")
		if dbPort == "" {
			dbPort = "5432"
		}
		addr := net.JoinHostPort(dbHost, dbPort)
		// Resolve DB host to IP for visibility
		ips, _ := net.LookupHost(dbHost)
		if len(ips) > 0 {
			resp["db_ip"] = ips[0]
		}
		conn, err := net.DialTimeout("tcp", addr, 2*time.Second)
		if err != nil {
			resp["db"] = "unreachable"
			resp["db_addr"] = addr
			resp["db_error"] = err.Error()
		} else {
			conn.Close()
			resp["db"] = "reachable"
			resp["db_addr"] = addr
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func getServerIP() string {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return "unknown"
	}
	for _, addr := range addrs {
		if ipnet, ok := addr.(*net.IPNet); ok && !ipnet.IP.IsLoopback() && ipnet.IP.To4() != nil {
			return ipnet.IP.String()
		}
	}
	hostname, _ := os.Hostname()
	return fmt.Sprintf("hostname:%s", hostname)
}
