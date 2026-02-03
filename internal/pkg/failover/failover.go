package failover

import (
	"crypto/rand"
	"fmt"
	"math/big"
	"net"
	"paqet/internal/conf"
	"paqet/internal/flog"
	"sync"
	"time"
)

// Manager handles server failover and load balancing
type Manager struct {
	config      *conf.Failover
	servers     []*net.UDPAddr
	currentIdx  int
	healthMap   map[string]bool
	mu          sync.RWMutex
	stopChan    chan struct{}
}

// New creates a new failover manager
func New(config *conf.Failover, servers []*net.UDPAddr) *Manager {
	if config == nil || !config.Enable || len(servers) == 0 {
		return nil
	}

	fm := &Manager{
		config:     config,
		servers:    servers,
		currentIdx: 0,
		healthMap:  make(map[string]bool),
		stopChan:   make(chan struct{}),
	}

	// Initialize all servers as healthy
	for _, server := range servers {
		fm.healthMap[server.String()] = true
	}

	return fm
}

// Start begins health checking
func (fm *Manager) Start() {
	if fm == nil {
		return
	}

	go fm.healthCheckLoop()
}

// Stop stops the health checking
func (fm *Manager) Stop() {
	if fm == nil {
		return
	}
	close(fm.stopChan)
}

// GetServer returns the next server based on the configured strategy
func (fm *Manager) GetServer() (*net.UDPAddr, error) {
	if fm == nil || len(fm.servers) == 0 {
		return nil, fmt.Errorf("no servers available")
	}

	fm.mu.Lock()
	defer fm.mu.Unlock()

	// Find healthy servers
	healthyServers := make([]*net.UDPAddr, 0, len(fm.servers))
	for _, server := range fm.servers {
		if fm.healthMap[server.String()] {
			healthyServers = append(healthyServers, server)
		}
	}

	if len(healthyServers) == 0 {
		// No healthy servers, return any server as fallback
		flog.Warnf("No healthy servers available, using fallback")
		return fm.servers[0], nil
	}

	switch fm.config.Strategy {
	case "round_robin":
		server := healthyServers[fm.currentIdx%len(healthyServers)]
		fm.currentIdx++
		return server, nil

	case "random":
		idx := fm.randomInt(0, len(healthyServers)-1)
		return healthyServers[idx], nil

	case "latency":
		// For simplicity, use round_robin for now
		// A full implementation would measure latency to each server
		server := healthyServers[fm.currentIdx%len(healthyServers)]
		fm.currentIdx++
		return server, nil

	default:
		return healthyServers[0], nil
	}
}

// MarkUnhealthy marks a server as unhealthy
func (fm *Manager) MarkUnhealthy(server *net.UDPAddr) {
	if fm == nil {
		return
	}

	fm.mu.Lock()
	defer fm.mu.Unlock()

	fm.healthMap[server.String()] = false
	flog.Warnf("Server %s marked as unhealthy", server.String())
}

// healthCheckLoop periodically checks server health
func (fm *Manager) healthCheckLoop() {
	ticker := time.NewTicker(time.Duration(fm.config.HealthCheckSec) * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			fm.checkHealth()
		case <-fm.stopChan:
			return
		}
	}
}

// checkHealth checks the health of all servers
func (fm *Manager) checkHealth() {
	fm.mu.Lock()
	defer fm.mu.Unlock()

	for _, server := range fm.servers {
		// Basic health check: try to resolve the address
		// In a real implementation, this would ping the server
		_, err := net.ResolveUDPAddr("udp", server.String())
		fm.healthMap[server.String()] = (err == nil)
	}
}

// randomInt returns a random integer between min and max (inclusive)
func (fm *Manager) randomInt(min, max int) int {
	if min >= max {
		return min
	}
	diff := max - min + 1
	n, _ := rand.Int(rand.Reader, big.NewInt(int64(diff)))
	return min + int(n.Int64())
}
