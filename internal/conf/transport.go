package conf

import (
	"fmt"
	"slices"
)

type Transport struct {
	Protocol  string   `yaml:"protocol"`
	Protocols []string `yaml:"protocols"`
	Conn      int      `yaml:"conn"`
	KCP       *KCP     `yaml:"kcp"`

	// Multi-protocol support
	Obfuscation *Obfuscation `yaml:"obfuscation"`
}

// Obfuscation for pluggable transports
type Obfuscation struct {
	Enable        bool   `yaml:"enable"`
	Type          string `yaml:"type"` // obfs4, meek, websocket, http2
	ObfsKey       string `yaml:"obfs_key"`
	MeekURL       string `yaml:"meek_url"`
	WebSocketPath string `yaml:"websocket_path"`
}

func (t *Transport) setDefaults(role string) {
	if t.Conn == 0 {
		t.Conn = 1
	}
	if t.Protocol == "" && len(t.Protocols) == 0 {
		t.Protocol = "kcp"
	}
	switch t.Protocol {
	case "kcp":
		if t.KCP != nil {
			t.KCP.setDefaults(role)
		}
	}

	// Set defaults for obfuscation
	if t.Obfuscation != nil {
		if t.Obfuscation.Type == "" {
			t.Obfuscation.Type = "obfs4"
		}
		if t.Obfuscation.WebSocketPath == "" {
			t.Obfuscation.WebSocketPath = "/ws"
		}
	}
}

func (t *Transport) validate() []error {
	var errors []error

	validProtocols := []string{"kcp", "websocket", "http2", "tcp"}
	if t.Protocol != "" && !slices.Contains(validProtocols, t.Protocol) {
		errors = append(errors, fmt.Errorf("transport protocol must be one of: %v", validProtocols))
	}

	// Validate protocols list if specified
	for i, proto := range t.Protocols {
		if !slices.Contains(validProtocols, proto) {
			errors = append(errors, fmt.Errorf("protocols[%d] must be one of: %v", i, validProtocols))
		}
	}

	if t.Conn < 1 || t.Conn > 256 {
		errors = append(errors, fmt.Errorf("conn must be between 1-256 connections"))
	}

	switch t.Protocol {
	case "kcp":
		if t.KCP != nil {
			errors = append(errors, t.KCP.validate()...)
		}
	}

	// Validate obfuscation
	if t.Obfuscation != nil && t.Obfuscation.Enable {
		validTypes := []string{"obfs4", "meek", "websocket", "http2"}
		if !slices.Contains(validTypes, t.Obfuscation.Type) {
			errors = append(errors, fmt.Errorf("obfuscation.type must be one of: %v", validTypes))
		}
		if t.Obfuscation.Type == "meek" && t.Obfuscation.MeekURL == "" {
			errors = append(errors, fmt.Errorf("obfuscation.meek_url is required when type is meek"))
		}
	}

	return errors
}
