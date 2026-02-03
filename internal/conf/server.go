package conf

import (
	"fmt"
	"net"
	"slices"
)

type Server struct {
	Addr_ string       `yaml:"addr"`
	Addr  *net.UDPAddr `yaml:"-"`

	// Multiple server support for redundancy
	Servers       []string       `yaml:"servers"`
	ServerAddrs   []*net.UDPAddr `yaml:"-"`
	Failover      *Failover      `yaml:"failover"`
	DNSObfuscation *DNSObfuscation `yaml:"dns_obfuscation"`
}

// Failover configuration for server redundancy
type Failover struct {
	Enable          bool   `yaml:"enable"`
	Strategy        string `yaml:"strategy"` // round_robin, random, latency
	HealthCheckSec  int    `yaml:"health_check_seconds"`
	MaxRetries      int    `yaml:"max_retries"`
	RetryDelaySec   int    `yaml:"retry_delay_seconds"`
}

// DNSObfuscation for DNS-over-HTTPS and domain generation
type DNSObfuscation struct {
	EnableDoH       bool     `yaml:"enable_doh"`
	DoHServers      []string `yaml:"doh_servers"`
	EnableDGA       bool     `yaml:"enable_dga"`
	DGASeed         string   `yaml:"dga_seed"`
	DGADomains      int      `yaml:"dga_domains"`
}

func (s *Server) setDefaults() {
	// Set defaults for failover
	if s.Failover != nil {
		if s.Failover.Strategy == "" {
			s.Failover.Strategy = "round_robin"
		}
		if s.Failover.HealthCheckSec == 0 {
			s.Failover.HealthCheckSec = 30
		}
		if s.Failover.MaxRetries == 0 {
			s.Failover.MaxRetries = 3
		}
		if s.Failover.RetryDelaySec == 0 {
			s.Failover.RetryDelaySec = 5
		}
	}

	// Set defaults for DNS obfuscation
	if s.DNSObfuscation != nil {
		if len(s.DNSObfuscation.DoHServers) == 0 {
			s.DNSObfuscation.DoHServers = []string{
				"https://cloudflare-dns.com/dns-query",
				"https://dns.google/dns-query",
			}
		}
		if s.DNSObfuscation.DGADomains == 0 {
			s.DNSObfuscation.DGADomains = 10
		}
	}
}

func (s *Server) validate() []error {
	var errors []error

	// Validate primary server address
	if s.Addr_ != "" {
		addr, err := validateAddr(s.Addr_, true)
		if err != nil {
			errors = append(errors, err)
		}
		s.Addr = addr
	}

	// Validate multiple servers if specified
	if len(s.Servers) > 0 {
		s.ServerAddrs = make([]*net.UDPAddr, 0, len(s.Servers))
		for i, serverAddr := range s.Servers {
			addr, err := validateAddr(serverAddr, true)
			if err != nil {
				errors = append(errors, fmt.Errorf("servers[%d]: %v", i, err))
			} else {
				s.ServerAddrs = append(s.ServerAddrs, addr)
			}
		}
	}

	// At least one server address must be specified
	if s.Addr_ == "" && len(s.Servers) == 0 {
		errors = append(errors, fmt.Errorf("at least one server address must be specified"))
	}

	// Validate failover configuration
	if s.Failover != nil && s.Failover.Enable {
		if len(s.Servers) < 2 && s.Addr_ == "" {
			errors = append(errors, fmt.Errorf("failover requires at least 2 servers"))
		}
		validStrategies := []string{"round_robin", "random", "latency"}
		if !slices.Contains(validStrategies, s.Failover.Strategy) {
			errors = append(errors, fmt.Errorf("failover.strategy must be one of: %v", validStrategies))
		}
		if s.Failover.HealthCheckSec < 10 || s.Failover.HealthCheckSec > 300 {
			errors = append(errors, fmt.Errorf("failover.health_check_seconds must be between 10-300"))
		}
		if s.Failover.MaxRetries < 1 || s.Failover.MaxRetries > 10 {
			errors = append(errors, fmt.Errorf("failover.max_retries must be between 1-10"))
		}
		if s.Failover.RetryDelaySec < 1 || s.Failover.RetryDelaySec > 60 {
			errors = append(errors, fmt.Errorf("failover.retry_delay_seconds must be between 1-60"))
		}
	}

	// Validate DNS obfuscation
	if s.DNSObfuscation != nil {
		if s.DNSObfuscation.EnableDoH && len(s.DNSObfuscation.DoHServers) == 0 {
			errors = append(errors, fmt.Errorf("dns_obfuscation.doh_servers cannot be empty when DoH is enabled"))
		}
		if s.DNSObfuscation.EnableDGA {
			if s.DNSObfuscation.DGASeed == "" {
				errors = append(errors, fmt.Errorf("dns_obfuscation.dga_seed is required when DGA is enabled"))
			}
			if s.DNSObfuscation.DGADomains < 1 || s.DNSObfuscation.DGADomains > 100 {
				errors = append(errors, fmt.Errorf("dns_obfuscation.dga_domains must be between 1-100"))
			}
		}
	}

	return errors
}
