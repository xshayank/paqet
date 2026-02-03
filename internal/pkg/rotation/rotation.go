package rotation

import (
	"crypto/rand"
	"fmt"
	"math/big"
	"paqet/internal/conf"
	"sync"
	"time"

	"github.com/xtaci/kcp-go/v5"
)

// RotationManager handles cipher rotation
type RotationManager struct {
	config         *conf.CipherRotation
	key            string
	currentCipher  int
	cipherBlocks   []kcp.BlockCrypt
	mu             sync.RWMutex
	stopChan       chan struct{}
}

// New creates a new cipher rotation manager
func New(config *conf.CipherRotation, key string) (*RotationManager, error) {
	if config == nil || !config.Enable {
		return nil, nil
	}

	rm := &RotationManager{
		config:   config,
		key:      key,
		stopChan: make(chan struct{}),
	}

	// Pre-initialize all cipher blocks
	rm.cipherBlocks = make([]kcp.BlockCrypt, len(config.Ciphers))
	for i, cipherName := range config.Ciphers {
		block, err := conf.NewBlockCrypt(cipherName, key)
		if err != nil {
			return nil, fmt.Errorf("failed to initialize cipher %s: %v", cipherName, err)
		}
		rm.cipherBlocks[i] = block
	}

	// Start with a random cipher
	rm.currentCipher = rm.randomInt(0, len(config.Ciphers)-1)

	return rm, nil
}

// Start begins the rotation process
func (rm *RotationManager) Start() {
	if rm == nil {
		return
	}

	go rm.rotationLoop()
}

// Stop stops the rotation process
func (rm *RotationManager) Stop() {
	if rm == nil {
		return
	}
	close(rm.stopChan)
}

// GetCurrentBlock returns the current cipher block
func (rm *RotationManager) GetCurrentBlock() kcp.BlockCrypt {
	if rm == nil {
		return nil
	}

	rm.mu.RLock()
	defer rm.mu.RUnlock()

	return rm.cipherBlocks[rm.currentCipher]
}

// GetCurrentCipherName returns the name of the current cipher
func (rm *RotationManager) GetCurrentCipherName() string {
	if rm == nil {
		return ""
	}

	rm.mu.RLock()
	defer rm.mu.RUnlock()

	return rm.config.Ciphers[rm.currentCipher]
}

// rotationLoop rotates ciphers at configured intervals
func (rm *RotationManager) rotationLoop() {
	ticker := time.NewTicker(time.Duration(rm.config.RotationInterval) * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			rm.rotate()
		case <-rm.stopChan:
			return
		}
	}
}

// rotate switches to the next cipher
func (rm *RotationManager) rotate() {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	// Move to next cipher (randomly or sequentially)
	rm.currentCipher = rm.randomInt(0, len(rm.config.Ciphers)-1)
}

// randomInt returns a random integer between min and max (inclusive)
func (rm *RotationManager) randomInt(min, max int) int {
	if min >= max {
		return min
	}
	diff := max - min + 1
	n, _ := rand.Int(rand.Reader, big.NewInt(int64(diff)))
	return min + int(n.Int64())
}
