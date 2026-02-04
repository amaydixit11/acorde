// Package vault provides multi-vault management.
package vault

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// VaultInfo contains metadata about a vault
type VaultInfo struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	DataDir     string `json:"data_dir"`
	Encrypted   bool   `json:"encrypted"`
	EntryCount  int    `json:"entry_count,omitempty"`
	CreatedAt   int64  `json:"created_at"`
	LastOpened  int64  `json:"last_opened,omitempty"`
}

// Manager manages multiple vaults
type Manager struct {
	baseDir     string
	vaults      map[string]*VaultInfo
	activeVault string
	mu          sync.RWMutex
}

// NewManager creates a new vault manager
func NewManager(baseDir string) (*Manager, error) {
	m := &Manager{
		baseDir: baseDir,
		vaults:  make(map[string]*VaultInfo),
	}

	if err := os.MkdirAll(baseDir, 0700); err != nil {
		return nil, fmt.Errorf("failed to create base directory: %w", err)
	}

	// Load existing vaults
	if err := m.loadVaults(); err != nil {
		return nil, err
	}

	return m, nil
}

// loadVaults discovers existing vaults in the base directory
func (m *Manager) loadVaults() error {
	configPath := filepath.Join(m.baseDir, "vaults.json")
	
	data, err := os.ReadFile(configPath)
	if os.IsNotExist(err) {
		return nil // No vaults yet
	}
	if err != nil {
		return err
	}

	var vaults []VaultInfo
	if err := json.Unmarshal(data, &vaults); err != nil {
		return err
	}

	for _, v := range vaults {
		m.vaults[v.ID] = &v
	}

	return nil
}

// saveVaults persists vault metadata
func (m *Manager) saveVaults() error {
	vaults := make([]VaultInfo, 0, len(m.vaults))
	for _, v := range m.vaults {
		vaults = append(vaults, *v)
	}

	data, err := json.MarshalIndent(vaults, "", "  ")
	if err != nil {
		return err
	}

	configPath := filepath.Join(m.baseDir, "vaults.json")
	return os.WriteFile(configPath, data, 0600)
}

// Create creates a new vault
func (m *Manager) Create(name string) (*VaultInfo, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Check for duplicate name
	for _, v := range m.vaults {
		if v.Name == name {
			return nil, fmt.Errorf("vault with name '%s' already exists", name)
		}
	}

	// Generate ID from name
	id := sanitizeID(name)
	if _, exists := m.vaults[id]; exists {
		id = id + "-" + generateShortID()
	}

	// Create vault directory
	dataDir := filepath.Join(m.baseDir, id)
	if err := os.MkdirAll(dataDir, 0700); err != nil {
		return nil, fmt.Errorf("failed to create vault directory: %w", err)
	}

	vault := &VaultInfo{
		ID:        id,
		Name:      name,
		DataDir:   dataDir,
		Encrypted: false,
		CreatedAt: nowUnix(),
	}

	m.vaults[id] = vault

	if err := m.saveVaults(); err != nil {
		return nil, err
	}

	return vault, nil
}

// List returns all vaults
func (m *Manager) List() []VaultInfo {
	m.mu.RLock()
	defer m.mu.RUnlock()

	vaults := make([]VaultInfo, 0, len(m.vaults))
	for _, v := range m.vaults {
		vaults = append(vaults, *v)
	}
	return vaults
}

// Get retrieves a vault by ID or name
func (m *Manager) Get(idOrName string) (*VaultInfo, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	// Try ID first
	if v, ok := m.vaults[idOrName]; ok {
		return v, nil
	}

	// Try name
	for _, v := range m.vaults {
		if v.Name == idOrName {
			return v, nil
		}
	}

	return nil, fmt.Errorf("vault not found: %s", idOrName)
}

// Delete removes a vault
func (m *Manager) Delete(idOrName string, removeData bool) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	var vault *VaultInfo
	var vaultID string

	// Find vault
	if v, ok := m.vaults[idOrName]; ok {
		vault = v
		vaultID = idOrName
	} else {
		for id, v := range m.vaults {
			if v.Name == idOrName {
				vault = v
				vaultID = id
				break
			}
		}
	}

	if vault == nil {
		return fmt.Errorf("vault not found: %s", idOrName)
	}

	// Remove data if requested
	if removeData {
		if err := os.RemoveAll(vault.DataDir); err != nil {
			return fmt.Errorf("failed to remove vault data: %w", err)
		}
	}

	delete(m.vaults, vaultID)
	return m.saveVaults()
}

// SetActive sets the active vault
func (m *Manager) SetActive(idOrName string) error {
	vault, err := m.Get(idOrName)
	if err != nil {
		return err
	}

	m.mu.Lock()
	m.activeVault = vault.ID
	m.mu.Unlock()

	return nil
}

// GetActive returns the active vault
func (m *Manager) GetActive() (*VaultInfo, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.activeVault == "" {
		return nil, fmt.Errorf("no active vault")
	}

	return m.vaults[m.activeVault], nil
}

// Rename renames a vault
func (m *Manager) Rename(idOrName, newName string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Find vault
	var vault *VaultInfo
	for _, v := range m.vaults {
		if v.ID == idOrName || v.Name == idOrName {
			vault = v
			break
		}
	}

	if vault == nil {
		return fmt.Errorf("vault not found: %s", idOrName)
	}

	// Check for duplicate name
	for _, v := range m.vaults {
		if v.Name == newName && v.ID != vault.ID {
			return fmt.Errorf("vault with name '%s' already exists", newName)
		}
	}

	vault.Name = newName
	return m.saveVaults()
}

// Helper functions

func sanitizeID(s string) string {
	result := make([]byte, 0, len(s))
	for _, c := range []byte(s) {
		if (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || c == '-' {
			result = append(result, c)
		} else if c >= 'A' && c <= 'Z' {
			result = append(result, c+32) // lowercase
		} else if c == ' ' || c == '_' {
			result = append(result, '-')
		}
	}
	return string(result)
}

func generateShortID() string {
	return fmt.Sprintf("%d", time.Now().Unix()%10000)
}

func nowUnix() int64 {
	return time.Now().Unix()
}

