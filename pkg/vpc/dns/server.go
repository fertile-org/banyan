package dns

import (
	"context"
	"fmt"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/miekg/dns"
)

// Server is a DNS server that resolves internal hostnames using DNS Manager
// and forwards external queries to upstream DNS servers.
type Server struct {
	manager      *Manager
	udpServer    *dns.Server
	tcpServer    *dns.Server
	bindAddr     string
	internalZone string // e.g., "internal." - note the trailing dot for DNS convention
	upstreamDNS  string // e.g., "8.8.8.8:53"
	mu           sync.RWMutex
	running      bool
}

// ServerConfig holds configuration for the DNS server
type ServerConfig struct {
	BindAddr     string // Address to bind (e.g., "10.0.0.1:53" or ":5353" for testing)
	InternalZone string // Zone for internal resolution (e.g., "internal")
	UpstreamDNS  string // Upstream DNS for external queries (e.g., "8.8.8.8:53")
}

// DefaultServerConfig returns sensible defaults for production use.
// BindAddr defaults to localhost — callers must override for overlay networking.
func DefaultServerConfig() ServerConfig {
	return ServerConfig{
		BindAddr:     "127.0.0.1:53",
		InternalZone: "internal",
		UpstreamDNS:  "8.8.8.8:53",
	}
}

// NewServer creates a new DNS server
func NewServer(manager *Manager, config ServerConfig) *Server {
	// Ensure zone has trailing dot (DNS convention)
	zone := config.InternalZone
	if !strings.HasSuffix(zone, ".") {
		zone += "."
	}

	// Ensure upstream has port
	upstream := config.UpstreamDNS
	if !strings.Contains(upstream, ":") {
		upstream += ":53"
	}

	return &Server{
		manager:      manager,
		bindAddr:     config.BindAddr,
		internalZone: zone,
		upstreamDNS:  upstream,
	}
}

// Start starts the DNS server (non-blocking)
// Returns an error if the server fails to start
func (s *Server) Start() error {
	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		return fmt.Errorf("server already running")
	}
	s.mu.Unlock()

	// Create DNS handler
	handler := dns.HandlerFunc(s.handleDNSRequest)

	// Start UDP server
	s.udpServer = &dns.Server{
		Addr:    s.bindAddr,
		Net:     "udp",
		Handler: handler,
	}

	// Start TCP server (for large responses)
	s.tcpServer = &dns.Server{
		Addr:    s.bindAddr,
		Net:     "tcp",
		Handler: handler,
	}

	// Channel for startup errors
	errChan := make(chan error, 2)

	// Start UDP server in goroutine
	go func() {
		if err := s.udpServer.ListenAndServe(); err != nil {
			errChan <- fmt.Errorf("UDP server error: %w", err)
		}
	}()

	// Start TCP server in goroutine
	go func() {
		if err := s.tcpServer.ListenAndServe(); err != nil {
			errChan <- fmt.Errorf("TCP server error: %w", err)
		}
	}()

	// Give servers a moment to start and check for immediate errors
	select {
	case err := <-errChan:
		_ = s.Stop() // Best effort cleanup
		return err
	case <-time.After(100 * time.Millisecond):
		s.mu.Lock()
		s.running = true
		s.mu.Unlock()
		return nil
	}
}

// Stop gracefully stops the DNS server
func (s *Server) Stop() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.running {
		return nil
	}

	s.running = false

	var errs []error
	if s.udpServer != nil {
		if err := s.udpServer.Shutdown(); err != nil {
			errs = append(errs, fmt.Errorf("UDP shutdown: %w", err))
		}
	}
	if s.tcpServer != nil {
		if err := s.tcpServer.Shutdown(); err != nil {
			errs = append(errs, fmt.Errorf("TCP shutdown: %w", err))
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("shutdown errors: %v", errs)
	}
	return nil
}

// handleDNSRequest handles incoming DNS queries
func (s *Server) handleDNSRequest(w dns.ResponseWriter, r *dns.Msg) {
	msg := new(dns.Msg)
	msg.SetReply(r)
	msg.Authoritative = true
	msg.RecursionAvailable = true

	// Handle each question
	for _, question := range r.Question {
		// Check if this is an internal domain
		if s.isInternalDomain(question.Name) {
			s.handleInternalQuery(msg, question)
		} else {
			// Forward to upstream
			s.forwardToUpstream(w, r)
			return
		}
	}

	_ = w.WriteMsg(msg)
}

// handleInternalQuery resolves internal domain queries using DNS Manager
func (s *Server) handleInternalQuery(msg *dns.Msg, question dns.Question) {
	// Strip the zone suffix to get hostname
	hostname := s.stripZone(question.Name)

	// Lookup in DNS Manager
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	ips, err := s.manager.LookupHost(ctx, hostname)
	if err != nil {
		// Return NXDOMAIN for not found
		msg.Rcode = dns.RcodeNameError
		return
	}

	// Add records based on query type
	switch question.Qtype {
	case dns.TypeA:
		s.addARecords(msg, question, ips)
	case dns.TypeAAAA:
		s.addAAAARecords(msg, question, ips)
	case dns.TypeANY:
		s.addARecords(msg, question, ips)
		s.addAAAARecords(msg, question, ips)
	default:
		// For unsupported types on internal domains, return empty
		msg.Rcode = dns.RcodeSuccess
	}
}

// addARecords adds A records (IPv4) to the response
func (s *Server) addARecords(msg *dns.Msg, question dns.Question, ips []net.IP) {
	for _, ip := range ips {
		// Only include IPv4 addresses
		if ipv4 := ip.To4(); ipv4 != nil {
			rr := &dns.A{
				Hdr: dns.RR_Header{
					Name:   question.Name,
					Rrtype: dns.TypeA,
					Class:  dns.ClassINET,
					Ttl:    60, // 60 second TTL for dynamic environments
				},
				A: ipv4,
			}
			msg.Answer = append(msg.Answer, rr)
		}
	}
}

// addAAAARecords adds AAAA records (IPv6) to the response
func (s *Server) addAAAARecords(msg *dns.Msg, question dns.Question, ips []net.IP) {
	for _, ip := range ips {
		// Only include IPv6 addresses (not IPv4-mapped)
		if ip.To4() == nil && ip.To16() != nil {
			rr := &dns.AAAA{
				Hdr: dns.RR_Header{
					Name:   question.Name,
					Rrtype: dns.TypeAAAA,
					Class:  dns.ClassINET,
					Ttl:    60,
				},
				AAAA: ip.To16(),
			}
			msg.Answer = append(msg.Answer, rr)
		}
	}
}

// isInternalDomain checks if the query is for our internal zone
func (s *Server) isInternalDomain(name string) bool {
	// DNS names are case-insensitive
	return strings.HasSuffix(strings.ToLower(name), strings.ToLower(s.internalZone))
}

// stripZone removes only the trailing dot from a DNS name
// The hostname is kept intact since users register with full hostnames like "web.internal"
// e.g., "myapp.internal." returns "myapp.internal"
func (s *Server) stripZone(name string) string {
	// Remove trailing dot (DNS convention)
	return strings.TrimSuffix(name, ".")
}

// forwardToUpstream forwards the entire query to upstream DNS
func (s *Server) forwardToUpstream(w dns.ResponseWriter, r *dns.Msg) {
	client := &dns.Client{
		Net:     "udp",
		Timeout: 5 * time.Second,
	}

	response, _, err := client.Exchange(r, s.upstreamDNS)
	if err != nil {
		// Return SERVFAIL on upstream error
		msg := new(dns.Msg)
		msg.SetReply(r)
		msg.Rcode = dns.RcodeServerFailure
		_ = w.WriteMsg(msg)
		return
	}

	_ = w.WriteMsg(response)
}

// IsRunning returns whether the server is currently running
func (s *Server) IsRunning() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.running
}

// BindAddress returns the address the server is bound to
func (s *Server) BindAddress() string {
	return s.bindAddr
}

// InternalZone returns the internal zone being served
func (s *Server) InternalZone() string {
	return strings.TrimSuffix(s.internalZone, ".")
}

// UpstreamDNS returns the upstream DNS server address
func (s *Server) UpstreamDNS() string {
	return s.upstreamDNS
}
