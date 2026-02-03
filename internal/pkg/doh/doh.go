package doh

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"net"
	"net/http"
	"paqet/internal/conf"
	"paqet/internal/flog"
	"time"
)

// Resolver handles DNS-over-HTTPS queries
type Resolver struct {
	config *conf.DNSObfuscation
	client *http.Client
}

// New creates a new DoH resolver
func New(config *conf.DNSObfuscation) *Resolver {
	if config == nil || !config.EnableDoH {
		return nil
	}

	return &Resolver{
		config: config,
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// Resolve resolves a hostname using DNS-over-HTTPS
func (r *Resolver) Resolve(hostname string) ([]net.IP, error) {
	if r == nil {
		// Fall back to standard DNS
		return net.LookupIP(hostname)
	}

	// Try DoH servers in order
	var lastErr error
	for _, dohServer := range r.config.DoHServers {
		ips, err := r.queryDoH(dohServer, hostname)
		if err == nil && len(ips) > 0 {
			return ips, nil
		}
		lastErr = err
	}

	// Fall back to standard DNS if DoH fails
	flog.Warnf("DoH resolution failed, falling back to standard DNS: %v", lastErr)
	return net.LookupIP(hostname)
}

// queryDoH queries a specific DoH server
func (r *Resolver) queryDoH(dohServer, hostname string) ([]net.IP, error) {
	// Use JSON API format (easier than wire format)
	url := fmt.Sprintf("%s?name=%s&type=A", dohServer, hostname)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Accept", "application/dns-json")
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")

	resp, err := r.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("DoH query failed with status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var result struct {
		Answer []struct {
			Data string `json:"data"`
		} `json:"Answer"`
	}

	if err := json.Unmarshal(body, &result); err != nil {
		return nil, err
	}

	ips := make([]net.IP, 0, len(result.Answer))
	for _, answer := range result.Answer {
		ip := net.ParseIP(answer.Data)
		if ip != nil {
			ips = append(ips, ip)
		}
	}

	return ips, nil
}

// GenerateDGADomains generates domains using a domain generation algorithm
func (r *Resolver) GenerateDGADomains() []string {
	if r == nil || !r.config.EnableDGA {
		return nil
	}

	domains := make([]string, r.config.DGADomains)
	seed := []byte(r.config.DGASeed)

	for i := 0; i < r.config.DGADomains; i++ {
		// Generate deterministic but pseudo-random domain
		hash := sha256.Sum256(append(seed, byte(i)))
		domain := r.hashToDomain(hash[:])
		domains[i] = domain + ".com"
	}

	return domains
}

// hashToDomain converts a hash to a pronounceable domain name
func (r *Resolver) hashToDomain(hash []byte) string {
	// Use first 12 bytes for a 12-character domain
	const charset = "abcdefghijklmnopqrstuvwxyz0123456789"
	domain := make([]byte, 12)

	for i := 0; i < 12 && i < len(hash); i++ {
		domain[i] = charset[int(hash[i])%len(charset)]
	}

	return string(domain)
}

// buildDNSQuery builds a DNS query packet
func buildDNSQuery(hostname string) ([]byte, error) {
	buf := new(bytes.Buffer)

	// Transaction ID
	binary.Write(buf, binary.BigEndian, uint16(randomUint16()))

	// Flags: standard query
	binary.Write(buf, binary.BigEndian, uint16(0x0100))

	// Questions: 1
	binary.Write(buf, binary.BigEndian, uint16(1))

	// Answer RRs: 0
	binary.Write(buf, binary.BigEndian, uint16(0))

	// Authority RRs: 0
	binary.Write(buf, binary.BigEndian, uint16(0))

	// Additional RRs: 0
	binary.Write(buf, binary.BigEndian, uint16(0))

	// Question section
	for _, part := range bytes.Split([]byte(hostname), []byte(".")) {
		buf.WriteByte(byte(len(part)))
		buf.Write(part)
	}
	buf.WriteByte(0) // End of name

	// Type: A (1)
	binary.Write(buf, binary.BigEndian, uint16(1))

	// Class: IN (1)
	binary.Write(buf, binary.BigEndian, uint16(1))

	return buf.Bytes(), nil
}

// randomUint16 generates a random uint16
func randomUint16() uint16 {
	n, _ := rand.Int(rand.Reader, big.NewInt(65536))
	return uint16(n.Int64())
}

// encodeBase64URL encodes data in base64url format
func encodeBase64URL(data []byte) string {
	return base64.RawURLEncoding.EncodeToString(data)
}
