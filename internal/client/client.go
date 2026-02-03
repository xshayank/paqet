package client

import (
	"context"
	"fmt"
	"paqet/internal/conf"
	"paqet/internal/flog"
	"paqet/internal/pkg/iterator"
	"paqet/internal/tnet"
	"sync"
)

type Client struct {
	cfg     *conf.Conf
	iter    *iterator.Iterator[*timedConn]
	udpPool *udpPool
	mu      sync.Mutex
}

func New(cfg *conf.Conf) (*Client, error) {
	c := &Client{
		cfg:     cfg,
		iter:    &iterator.Iterator[*timedConn]{},
		udpPool: &udpPool{strms: make(map[uint64]tnet.Strm)},
	}
	return c, nil
}

func (c *Client) Start(ctx context.Context) error {
	// Check if GFW resilience features are enabled
	useEnhanced := c.isGFWResilienceEnabled()

	for i := range c.cfg.Transport.Conn {
		var tc *timedConn
		var err error

		if useEnhanced {
			// Use enhanced connection with GFW resilience features
			ec, ecErr := newEnhancedConn(ctx, c.cfg)
			if ecErr != nil {
				flog.Errorf("failed to establish enhanced connection %d: %v", i+1, ecErr)
				return ecErr
			}
			tc = ec.timedConn
			flog.Debugf("client enhanced connection %d established successfully with GFW resilience", i+1)
		} else {
			// Use standard connection
			tc, err = newTimedConn(ctx, c.cfg)
			if err != nil {
				flog.Errorf("failed to establish connection %d: %v", i+1, err)
				return err
			}
			flog.Debugf("client connection %d established successfully", i+1)
		}

		c.iter.Items = append(c.iter.Items, tc)
	}
	go c.ticker(ctx)

	go func() {
		<-ctx.Done()
		for _, tc := range c.iter.Items {
			tc.close()
		}
		flog.Infof("client shutdown complete")
	}()

	ipv4Addr := "<nil>"
	ipv6Addr := "<nil>"
	if c.cfg.Network.IPv4.Addr != nil {
		ipv4Addr = c.cfg.Network.IPv4.Addr.IP.String()
	}
	if c.cfg.Network.IPv6.Addr != nil {
		ipv6Addr = c.cfg.Network.IPv6.Addr.IP.String()
	}

	serverInfo := "multiple servers"
	if c.cfg.Server.Addr != nil {
		serverInfo = c.cfg.Server.Addr.String()
		if len(c.cfg.Server.ServerAddrs) > 0 {
			serverInfo = fmt.Sprintf("%s + %d backups", serverInfo, len(c.cfg.Server.ServerAddrs))
		}
	} else if len(c.cfg.Server.ServerAddrs) > 0 {
		serverInfo = fmt.Sprintf("%d servers", len(c.cfg.Server.ServerAddrs))
	}

	flog.Infof("Client started: IPv4:%s IPv6:%s -> %s (%d connections)", ipv4Addr, ipv6Addr, serverInfo, len(c.iter.Items))
	return nil
}

// isGFWResilienceEnabled checks if any GFW resilience features are enabled
func (c *Client) isGFWResilienceEnabled() bool {
	cfg := c.cfg

	// Check server features
	if cfg.Server.Failover != nil && cfg.Server.Failover.Enable {
		return true
	}
	if cfg.Server.DNSObfuscation != nil && (cfg.Server.DNSObfuscation.EnableDoH || cfg.Server.DNSObfuscation.EnableDGA) {
		return true
	}

	// Check transport features
	if cfg.Transport.Obfuscation != nil && cfg.Transport.Obfuscation.Enable {
		return true
	}

	// Check KCP features
	if cfg.Transport.KCP != nil {
		if cfg.Transport.KCP.TrafficShaping != nil && (cfg.Transport.KCP.TrafficShaping.EnablePadding || cfg.Transport.KCP.TrafficShaping.EnableTimingJitter || cfg.Transport.KCP.TrafficShaping.EnableFragmentation) {
			return true
		}
		if cfg.Transport.KCP.CipherRotation != nil && cfg.Transport.KCP.CipherRotation.Enable {
			return true
		}
		if cfg.Transport.KCP.DynamicTuning != nil && cfg.Transport.KCP.DynamicTuning.Enable {
			return true
		}
	}

	return false
}
