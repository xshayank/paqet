package shaping

import (
	"crypto/rand"
	"math/big"
	"paqet/internal/conf"
	"time"
)

// Shaper handles traffic shaping operations
type Shaper struct {
	config *conf.TrafficShaping
}

// New creates a new traffic shaper
func New(config *conf.TrafficShaping) *Shaper {
	if config == nil {
		return nil
	}
	return &Shaper{
		config: config,
	}
}

// AddPadding adds random padding to data if enabled
func (s *Shaper) AddPadding(data []byte) []byte {
	if s == nil || !s.config.EnablePadding {
		return data
	}

	paddingSize := s.randomInt(s.config.MinPaddingBytes, s.config.MaxPaddingBytes)
	if paddingSize == 0 {
		return data
	}

	padding := make([]byte, paddingSize)
	rand.Read(padding)

	// Add length prefix so we can strip padding later
	result := make([]byte, 2+len(data)+paddingSize)
	result[0] = byte(len(data) >> 8)
	result[1] = byte(len(data) & 0xFF)
	copy(result[2:], data)
	copy(result[2+len(data):], padding)

	return result
}

// RemovePadding removes padding from data
func (s *Shaper) RemovePadding(data []byte) []byte {
	if s == nil || !s.config.EnablePadding || len(data) < 2 {
		return data
	}

	dataLen := int(data[0])<<8 | int(data[1])
	if dataLen+2 > len(data) {
		return data // Invalid padding, return as-is
	}

	return data[2 : 2+dataLen]
}

// ApplyJitter applies timing jitter if enabled
func (s *Shaper) ApplyJitter() {
	if s == nil || !s.config.EnableTimingJitter {
		return
	}

	jitterMs := s.randomInt(s.config.MinJitterMs, s.config.MaxJitterMs)
	if jitterMs > 0 {
		time.Sleep(time.Duration(jitterMs) * time.Millisecond)
	}
}

// Fragment splits data into smaller fragments if enabled
func (s *Shaper) Fragment(data []byte) [][]byte {
	if s == nil || !s.config.EnableFragmentation {
		return [][]byte{data}
	}

	fragmentSize := s.config.FragmentSize
	if fragmentSize <= 0 || len(data) <= fragmentSize {
		return [][]byte{data}
	}

	fragments := make([][]byte, 0, (len(data)+fragmentSize-1)/fragmentSize)
	for i := 0; i < len(data); i += fragmentSize {
		end := i + fragmentSize
		if end > len(data) {
			end = len(data)
		}
		fragments = append(fragments, data[i:end])
	}

	return fragments
}

// randomInt returns a random integer between min and max (inclusive)
func (s *Shaper) randomInt(min, max int) int {
	if min >= max {
		return min
	}
	diff := max - min + 1
	n, _ := rand.Int(rand.Reader, big.NewInt(int64(diff)))
	return min + int(n.Int64())
}
