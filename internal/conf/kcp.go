package conf

import (
	"fmt"
	"slices"

	"github.com/xtaci/kcp-go/v5"
)

type KCP struct {
	Mode         string `yaml:"mode"`
	NoDelay      int    `yaml:"nodelay"`
	Interval     int    `yaml:"interval"`
	Resend       int    `yaml:"resend"`
	NoCongestion int    `yaml:"nocongestion"`
	WDelay       bool   `yaml:"wdelay"`
	AckNoDelay   bool   `yaml:"acknodelay"`

	MTU    int `yaml:"mtu"`
	Rcvwnd int `yaml:"rcvwnd"`
	Sndwnd int `yaml:"sndwnd"`
	Dshard int `yaml:"dshard"`
	Pshard int `yaml:"pshard"`

	Block_ string `yaml:"block"`
	Key    string `yaml:"key"`

	Smuxbuf   int `yaml:"smuxbuf"`
	Streambuf int `yaml:"streambuf"`

	// GFW Resilience Features
	TrafficShaping *TrafficShaping `yaml:"traffic_shaping"`
	CipherRotation *CipherRotation `yaml:"cipher_rotation"`
	DynamicTuning  *DynamicTuning  `yaml:"dynamic_tuning"`

	Block kcp.BlockCrypt `yaml:"-"`
}

// TrafficShaping contains options for traffic obfuscation
type TrafficShaping struct {
	EnablePadding       bool `yaml:"enable_padding"`
	MinPaddingBytes     int  `yaml:"min_padding_bytes"`
	MaxPaddingBytes     int  `yaml:"max_padding_bytes"`
	EnableTimingJitter  bool `yaml:"enable_timing_jitter"`
	MinJitterMs         int  `yaml:"min_jitter_ms"`
	MaxJitterMs         int  `yaml:"max_jitter_ms"`
	EnableFragmentation bool `yaml:"enable_fragmentation"`
	FragmentSize        int  `yaml:"fragment_size"`
}

// CipherRotation enables automatic cipher switching
type CipherRotation struct {
	Enable           bool     `yaml:"enable"`
	RotationInterval int      `yaml:"rotation_interval_seconds"`
	Ciphers          []string `yaml:"ciphers"`
}

// DynamicTuning enables adaptive KCP parameter adjustment
type DynamicTuning struct {
	Enable              bool `yaml:"enable"`
	AdaptiveInterval    bool `yaml:"adaptive_interval"`
	AdaptiveCongestion  bool `yaml:"adaptive_congestion"`
	MonitoringWindowSec int  `yaml:"monitoring_window_seconds"`
}

func (k *KCP) setDefaults(role string) {
	if k.Mode == "" {
		k.Mode = "fast"
	}
	if k.MTU == 0 {
		k.MTU = 1350
	}

	if k.Rcvwnd == 0 {
		if role == "server" {
			k.Rcvwnd = 1024
		} else {
			k.Rcvwnd = 512
		}
	}
	if k.Sndwnd == 0 {
		if role == "server" {
			k.Sndwnd = 1024
		} else {
			k.Sndwnd = 512
		}
	}

	// if k.Dshard == 0 {
	// 	k.Dshard = 10
	// }
	// if k.Pshard == 0 {
	// 	k.Pshard = 3
	// }

	if k.Block_ == "" {
		k.Block_ = "aes"
	}

	if k.Smuxbuf == 0 {
		k.Smuxbuf = 4 * 1024 * 1024
	}
	if k.Streambuf == 0 {
		k.Streambuf = 2 * 1024 * 1024
	}

	// Set defaults for traffic shaping
	if k.TrafficShaping != nil {
		if k.TrafficShaping.MaxPaddingBytes == 0 {
			k.TrafficShaping.MaxPaddingBytes = 128
		}
		if k.TrafficShaping.MaxJitterMs == 0 {
			k.TrafficShaping.MaxJitterMs = 50
		}
		if k.TrafficShaping.FragmentSize == 0 {
			k.TrafficShaping.FragmentSize = 512
		}
	}

	// Set defaults for cipher rotation
	if k.CipherRotation != nil {
		if k.CipherRotation.RotationInterval == 0 {
			k.CipherRotation.RotationInterval = 3600 // 1 hour
		}
		if len(k.CipherRotation.Ciphers) == 0 {
			k.CipherRotation.Ciphers = []string{"aes-128-gcm", "salsa20", "twofish"}
		}
	}

	// Set defaults for dynamic tuning
	if k.DynamicTuning != nil {
		if k.DynamicTuning.MonitoringWindowSec == 0 {
			k.DynamicTuning.MonitoringWindowSec = 60
		}
	}
}

func (k *KCP) validate() []error {
	var errors []error

	validModes := []string{"normal", "fast", "fast2", "fast3", "manual"}
	if !slices.Contains(validModes, k.Mode) {
		errors = append(errors, fmt.Errorf("KCP mode must be one of: %v", validModes))
	}

	if k.MTU < 50 || k.MTU > 1500 {
		errors = append(errors, fmt.Errorf("KCP MTU must be between 50-1500 bytes"))
	}

	if k.Rcvwnd < 1 || k.Rcvwnd > 32768 {
		errors = append(errors, fmt.Errorf("KCP rcvwnd must be between 1-32768"))
	}
	if k.Sndwnd < 1 || k.Sndwnd > 32768 {
		errors = append(errors, fmt.Errorf("KCP sndwnd must be between 1-32768"))
	}

	validBlocks := []string{"aes", "aes-128", "aes-128-gcm", "aes-192", "salsa20", "blowfish", "twofish", "cast5", "3des", "tea", "xtea", "xor", "sm4", "none", "null"}
	if !slices.Contains(validBlocks, k.Block_) {
		errors = append(errors, fmt.Errorf("KCP encryption block must be one of: %v", validBlocks))
	}
	if !slices.Contains([]string{"none", "null"}, k.Block_) && len(k.Key) == 0 {
		errors = append(errors, fmt.Errorf("KCP encryption key is required"))
	}
	b, err := newBlock(k.Block_, k.Key)
	if err != nil {
		errors = append(errors, err)
	}
	k.Block = b

	if k.Smuxbuf < 1024 {
		errors = append(errors, fmt.Errorf("KCP smuxbuf must be >= 1024 bytes"))
	}
	if k.Streambuf < 1024 {
		errors = append(errors, fmt.Errorf("KCP streambuf must be >= 1024 bytes"))
	}

	// Validate traffic shaping
	if k.TrafficShaping != nil {
		if k.TrafficShaping.MinPaddingBytes < 0 || k.TrafficShaping.MinPaddingBytes > 1024 {
			errors = append(errors, fmt.Errorf("traffic_shaping.min_padding_bytes must be between 0-1024"))
		}
		if k.TrafficShaping.MaxPaddingBytes < 0 || k.TrafficShaping.MaxPaddingBytes > 1024 {
			errors = append(errors, fmt.Errorf("traffic_shaping.max_padding_bytes must be between 0-1024"))
		}
		if k.TrafficShaping.MinPaddingBytes > k.TrafficShaping.MaxPaddingBytes {
			errors = append(errors, fmt.Errorf("traffic_shaping.min_padding_bytes must be <= max_padding_bytes"))
		}
		if k.TrafficShaping.MinJitterMs < 0 || k.TrafficShaping.MinJitterMs > 1000 {
			errors = append(errors, fmt.Errorf("traffic_shaping.min_jitter_ms must be between 0-1000"))
		}
		if k.TrafficShaping.MaxJitterMs < 0 || k.TrafficShaping.MaxJitterMs > 1000 {
			errors = append(errors, fmt.Errorf("traffic_shaping.max_jitter_ms must be between 0-1000"))
		}
		if k.TrafficShaping.MinJitterMs > k.TrafficShaping.MaxJitterMs {
			errors = append(errors, fmt.Errorf("traffic_shaping.min_jitter_ms must be <= max_jitter_ms"))
		}
		if k.TrafficShaping.FragmentSize < 64 || k.TrafficShaping.FragmentSize > k.MTU {
			errors = append(errors, fmt.Errorf("traffic_shaping.fragment_size must be between 64-%d", k.MTU))
		}
	}

	// Validate cipher rotation
	if k.CipherRotation != nil && k.CipherRotation.Enable {
		if k.CipherRotation.RotationInterval < 60 {
			errors = append(errors, fmt.Errorf("cipher_rotation.rotation_interval_seconds must be >= 60"))
		}
		for _, cipher := range k.CipherRotation.Ciphers {
			if !slices.Contains(validBlocks, cipher) {
				errors = append(errors, fmt.Errorf("cipher_rotation cipher '%s' must be one of: %v", cipher, validBlocks))
			}
		}
		if len(k.CipherRotation.Ciphers) < 2 {
			errors = append(errors, fmt.Errorf("cipher_rotation must have at least 2 ciphers"))
		}
	}

	// Validate dynamic tuning
	if k.DynamicTuning != nil && k.DynamicTuning.Enable {
		if k.DynamicTuning.MonitoringWindowSec < 10 || k.DynamicTuning.MonitoringWindowSec > 600 {
			errors = append(errors, fmt.Errorf("dynamic_tuning.monitoring_window_seconds must be between 10-600"))
		}
	}

	return errors
}
