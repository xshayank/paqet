package client

import (
	"context"
	"fmt"
	"net"
	"paqet/internal/conf"
	"paqet/internal/flog"
	"paqet/internal/pkg/doh"
	"paqet/internal/pkg/failover"
	"paqet/internal/pkg/rotation"
	"paqet/internal/pkg/shaping"
	"paqet/internal/socket"
	"paqet/internal/tnet"
	"paqet/internal/tnet/kcp"
	"time"
)

// enhancedConn wraps timedConn with GFW resilience features
type enhancedConn struct {
	*timedConn
	failoverMgr  *failover.Manager
	dohResolver  *doh.Resolver
	rotationMgr  *rotation.RotationManager
	trafficShaper *shaping.Shaper
}

// newEnhancedConn creates a new connection with GFW resilience features
func newEnhancedConn(ctx context.Context, cfg *conf.Conf) (*enhancedConn, error) {
	ec := &enhancedConn{
		timedConn: &timedConn{cfg: cfg, ctx: ctx},
	}

	// Initialize DoH resolver
	if cfg.Server.DNSObfuscation != nil {
		ec.dohResolver = doh.New(cfg.Server.DNSObfuscation)
	}

	// Initialize failover manager
	if cfg.Server.Failover != nil && cfg.Server.Failover.Enable {
		servers := make([]*net.UDPAddr, 0)
		if cfg.Server.Addr != nil {
			servers = append(servers, cfg.Server.Addr)
		}
		servers = append(servers, cfg.Server.ServerAddrs...)
		ec.failoverMgr = failover.New(cfg.Server.Failover, servers)
		if ec.failoverMgr != nil {
			ec.failoverMgr.Start()
		}
	}

	// Initialize cipher rotation
	if cfg.Transport.KCP.CipherRotation != nil {
		var err error
		ec.rotationMgr, err = rotation.New(cfg.Transport.KCP.CipherRotation, cfg.Transport.KCP.Key)
		if err != nil {
			flog.Warnf("Failed to initialize cipher rotation: %v", err)
		} else if ec.rotationMgr != nil {
			ec.rotationMgr.Start()
		}
	}

	// Initialize traffic shaper
	if cfg.Transport.KCP.TrafficShaping != nil {
		ec.trafficShaper = shaping.New(cfg.Transport.KCP.TrafficShaping)
	}

	// Create the connection
	var err error
	ec.timedConn.conn, err = ec.createConnWithFailover()
	if err != nil {
		return nil, err
	}

	return ec, nil
}

// createConnWithFailover creates a connection with failover support
func (ec *enhancedConn) createConnWithFailover() (tnet.Conn, error) {
	// Get server address (with failover if enabled)
	serverAddr := ec.cfg.Server.Addr
	if ec.failoverMgr != nil {
		var err error
		serverAddr, err = ec.failoverMgr.GetServer()
		if err != nil {
			return nil, err
		}
	}

	// Resolve hostname with DoH if enabled
	if ec.dohResolver != nil && serverAddr != nil {
		hostname := serverAddr.IP.String()
		if serverAddr.IP.IsUnspecified() || serverAddr.IP.IsLoopback() {
			// This is likely a hostname that needs resolution
			ips, err := ec.dohResolver.Resolve(hostname)
			if err == nil && len(ips) > 0 {
				serverAddr.IP = ips[0]
				flog.Debugf("Resolved %s to %s using DoH", hostname, ips[0])
			}
		}
	}

	// Create raw packet connection
	netCfg := ec.cfg.Network
	pConn, err := socket.New(ec.ctx, &netCfg)
	if err != nil {
		return nil, fmt.Errorf("could not create raw packet conn: %w", err)
	}

	// Get block cipher (with rotation if enabled)
	block := ec.cfg.Transport.KCP.Block
	if ec.rotationMgr != nil {
		block = ec.rotationMgr.GetCurrentBlock()
		flog.Debugf("Using cipher: %s", ec.rotationMgr.GetCurrentCipherName())
	}

	// Override the block cipher for this connection
	kcpCfg := *ec.cfg.Transport.KCP
	kcpCfg.Block = block

	// Create KCP connection
	conn, err := kcp.Dial(serverAddr, &kcpCfg, pConn)
	if err != nil {
		// Mark server as unhealthy if failover is enabled
		if ec.failoverMgr != nil {
			ec.failoverMgr.MarkUnhealthy(serverAddr)
		}
		return nil, err
	}

	// Send TCP flags
	err = ec.timedConn.sendTCPF(conn)
	if err != nil {
		return nil, err
	}

	return conn, nil
}

// close closes the enhanced connection and cleanup resources
func (ec *enhancedConn) close() {
	if ec.rotationMgr != nil {
		ec.rotationMgr.Stop()
	}
	if ec.failoverMgr != nil {
		ec.failoverMgr.Stop()
	}
	ec.timedConn.close()
}

// waitConn waits for connection with retry and failover
func (ec *enhancedConn) waitConn() tnet.Conn {
	maxRetries := 3
	if ec.cfg.Server.Failover != nil {
		maxRetries = ec.cfg.Server.Failover.MaxRetries
	}

	for retry := 0; retry < maxRetries; retry++ {
		if c, err := ec.createConnWithFailover(); err == nil {
			return c
		} else {
			flog.Warnf("Connection attempt %d failed: %v", retry+1, err)
			retryDelay := time.Second
			if ec.cfg.Server.Failover != nil {
				retryDelay = time.Duration(ec.cfg.Server.Failover.RetryDelaySec) * time.Second
			}
			time.Sleep(retryDelay)
		}
	}

	// Fallback to original behavior
	return ec.timedConn.waitConn()
}
