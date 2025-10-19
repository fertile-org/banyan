package security

import (
	"context"
	"fmt"
	"net"
	"strings"

	"github.com/fertile-org/banyan/pkg/vpc"
)

// IptablesRule represents a single iptables rule
type IptablesRule struct {
	Table string   // filter, nat, mangle
	Chain string   // INPUT, FORWARD, OUTPUT, or custom
	Args  []string // Arguments for iptables command
}

// ServiceResolver resolves service names to IP addresses
type ServiceResolver interface {
	ResolveServiceIPs(ctx context.Context, serviceName string) ([]net.IP, error)
}

// Translator translates high-level SecurityRule to low-level iptables rules
type Translator struct {
	serviceResolver ServiceResolver
}

// NewTranslator creates a new Translator
func NewTranslator(resolver ServiceResolver) *Translator {
	return &Translator{
		serviceResolver: resolver,
	}
}

// TranslateRule translates a SecurityRule into iptables rules
func (t *Translator) TranslateRule(ctx context.Context, rule *vpc.SecurityRule) ([]IptablesRule, error) {
	if rule == nil {
		return nil, fmt.Errorf("rule cannot be nil")
	}

	var sources, destinations []string
	var err error

	// For egress rules, source is the service itself, destination is the To field
	// For ingress rules, source is the From field, destination is the service
	if rule.Direction == "egress" {
		// Egress: traffic FROM this service TO somewhere
		sources, err = t.parseSource(ctx, fmt.Sprintf("service:%s", rule.ServiceName))
		if err != nil {
			return nil, fmt.Errorf("failed to parse source service %q: %w", rule.ServiceName, err)
		}

		destinations, err = t.parseDestination(ctx, rule.To, "")
		if err != nil {
			return nil, fmt.Errorf("failed to parse destination %q: %w", rule.To, err)
		}
	} else {
		// Ingress (or default): traffic FROM somewhere TO this service
		sources, err = t.parseSource(ctx, rule.From)
		if err != nil {
			return nil, fmt.Errorf("failed to parse source %q: %w", rule.From, err)
		}

		destinations, err = t.parseDestination(ctx, rule.To, rule.ServiceName)
		if err != nil {
			return nil, fmt.Errorf("failed to parse destination %q: %w", rule.To, err)
		}
	}

	// Generate iptables rules for each source-destination pair
	var rules []IptablesRule
	for _, src := range sources {
		for _, dst := range destinations {
			iptablesRule := t.buildIptablesRule(src, dst, rule)
			rules = append(rules, iptablesRule)
		}
	}

	return rules, nil
}

// parseSource parses the source specification
// Supports: "internet", "service:name", "cidr:x.x.x.x/y"
func (t *Translator) parseSource(ctx context.Context, from string) ([]string, error) {
	if from == "" {
		return nil, fmt.Errorf("source cannot be empty")
	}

	// Handle "internet" - allows traffic from anywhere
	if from == "internet" {
		return []string{"0.0.0.0/0"}, nil
	}

	// Handle "service:name" format
	if strings.HasPrefix(from, "service:") {
		serviceName := strings.TrimPrefix(from, "service:")
		if serviceName == "" {
			return nil, fmt.Errorf("service name cannot be empty")
		}

		if t.serviceResolver == nil {
			return nil, fmt.Errorf("service resolver not configured")
		}

		ips, err := t.serviceResolver.ResolveServiceIPs(ctx, serviceName)
		if err != nil {
			return nil, fmt.Errorf("failed to resolve service %q: %w", serviceName, err)
		}

		if len(ips) == 0 {
			return nil, fmt.Errorf("service %q has no IP addresses", serviceName)
		}

		// Convert IPs to strings
		sources := make([]string, len(ips))
		for i, ip := range ips {
			sources[i] = ip.String()
		}
		return sources, nil
	}

	// Handle "cidr:x.x.x.x/y" format
	if strings.HasPrefix(from, "cidr:") {
		cidr := strings.TrimPrefix(from, "cidr:")
		if cidr == "" {
			return nil, fmt.Errorf("CIDR cannot be empty")
		}

		// Validate CIDR
		_, _, err := net.ParseCIDR(cidr)
		if err != nil {
			return nil, fmt.Errorf("invalid CIDR %q: %w", cidr, err)
		}

		return []string{cidr}, nil
	}

	return nil, fmt.Errorf("unsupported source format %q (use 'internet', 'service:name', or 'cidr:x.x.x.x/y')", from)
}

// parseDestination parses the destination specification
// Supports: "service:name" (implicit in ServiceName field), "cidr:x.x.x.x/y"
func (t *Translator) parseDestination(ctx context.Context, to, serviceName string) ([]string, error) {
	// If ServiceName is provided, use it (this is the primary service being protected)
	if serviceName != "" {
		if t.serviceResolver == nil {
			return nil, fmt.Errorf("service resolver not configured")
		}

		ips, err := t.serviceResolver.ResolveServiceIPs(ctx, serviceName)
		if err != nil {
			return nil, fmt.Errorf("failed to resolve service %q: %w", serviceName, err)
		}

		if len(ips) == 0 {
			return nil, fmt.Errorf("service %q has no IP addresses", serviceName)
		}

		// Convert IPs to strings
		destinations := make([]string, len(ips))
		for i, ip := range ips {
			destinations[i] = ip.String()
		}
		return destinations, nil
	}

	// If To is provided, parse it
	if to != "" {
		// Handle "cidr:x.x.x.x/y" format
		if strings.HasPrefix(to, "cidr:") {
			cidr := strings.TrimPrefix(to, "cidr:")
			if cidr == "" {
				return nil, fmt.Errorf("CIDR cannot be empty")
			}

			// Validate CIDR
			_, _, err := net.ParseCIDR(cidr)
			if err != nil {
				return nil, fmt.Errorf("invalid CIDR %q: %w", cidr, err)
			}

			return []string{cidr}, nil
		}

		// Handle "service:name" format
		if strings.HasPrefix(to, "service:") {
			targetService := strings.TrimPrefix(to, "service:")
			if targetService == "" {
				return nil, fmt.Errorf("service name cannot be empty")
			}

			if t.serviceResolver == nil {
				return nil, fmt.Errorf("service resolver not configured")
			}

			ips, err := t.serviceResolver.ResolveServiceIPs(ctx, targetService)
			if err != nil {
				return nil, fmt.Errorf("failed to resolve service %q: %w", targetService, err)
			}

			if len(ips) == 0 {
				return nil, fmt.Errorf("service %q has no IP addresses", targetService)
			}

			// Convert IPs to strings
			destinations := make([]string, len(ips))
			for i, ip := range ips {
				destinations[i] = ip.String()
			}
			return destinations, nil
		}

		return nil, fmt.Errorf("unsupported destination format %q (use 'service:name' or 'cidr:x.x.x.x/y')", to)
	}

	return nil, fmt.Errorf("either ServiceName or To must be specified")
}

// buildIptablesRule constructs an iptables rule from source, destination, and rule details
func (t *Translator) buildIptablesRule(src, dst string, rule *vpc.SecurityRule) IptablesRule {
	// Source and Destination IP/CIDR
	args := []string{"-s", src, "-d", dst}

	// Protocol (tcp, udp, icmp, or all)
	if rule.Protocol != "" && rule.Protocol != "all" {
		args = append(args, "-p", strings.ToLower(rule.Protocol))

		// Port (only for tcp/udp)
		if rule.ToPort != "" && (rule.Protocol == "tcp" || rule.Protocol == "udp") {
			args = append(args, "--dport", rule.ToPort)
		}
	}

	// Action (ACCEPT or DROP)
	action := "ACCEPT"
	if rule.Action == "deny" {
		action = "DROP"
	}
	args = append(args, "-j", action)

	return IptablesRule{
		Table: "filter",
		Chain: "FORWARD", // Container-to-container traffic goes through FORWARD chain
		Args:  args,
	}
}
