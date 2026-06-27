package config

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
)

// DBType 支持的数据库类型，与前端下拉选项一致。
type DBType string

const (
	DBSQLite     DBType = "SQLite"
	DBMySQL      DBType = "MySQL"
	DBPostgreSQL DBType = "PostgreSQL"
)

// SystemConfig 系统初始化时持久化的配置，落盘到 data/config.json。
type SystemConfig struct {
	Name          string `json:"name"`
	Intro         string `json:"intro"`         // 对应前端的 cloudProvider（系统简介）
	LogoPath      string `json:"logoPath"`      // logo 相对存储路径，可为空
	DBType        DBType `json:"dbType"`
	DBHost        string `json:"dbHost,omitempty"`
	DBPort        string `json:"dbPort,omitempty"`
	DBUser        string `json:"dbUser,omitempty"`
	DBPassword    string `json:"dbPassword,omitempty"`
	DBName        string `json:"dbName,omitempty"`
	JWTSecret     string `json:"jwtSecret,omitempty"`     // JWT 签名密钥，初始化时随机生成
	Initialized   bool   `json:"initialized"`   // 系统配置是否已完成
	AdminCreated  bool   `json:"adminCreated"`  // 管理员是否已创建
}

// Manager 负责系统配置的加载与保存，并发安全。
type Manager struct {
	mu       sync.RWMutex
	path     string
	dataDir  string
	current  *SystemConfig
}

// NewManager 创建配置管理器，dataDir 为数据根目录（存放 config.json、sqlite、logo 等）。
func NewManager(dataDir string) (*Manager, error) {
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return nil, err
	}
	m := &Manager{
		path:    filepath.Join(dataDir, "config.json"),
		dataDir: dataDir,
	}
	if err := m.load(); err != nil {
		return nil, err
	}
	return m, nil
}

// DataDir 返回数据根目录。
func (m *Manager) DataDir() string {
	return m.dataDir
}

func (m *Manager) load() error {
	data, err := os.ReadFile(m.path)
	if errors.Is(err, os.ErrNotExist) {
		m.current = &SystemConfig{}
		return nil
	}
	if err != nil {
		return err
	}
	cfg := &SystemConfig{}
	if err := json.Unmarshal(data, cfg); err != nil {
		return err
	}
	m.current = cfg
	return nil
}

// Get 返回当前配置的拷贝。
func (m *Manager) Get() SystemConfig {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return *m.current
}

// Save 覆盖保存配置并落盘。
func (m *Manager) Save(cfg SystemConfig) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	data, err := json.MarshalIndent(&cfg, "", "  ")
	if err != nil {
		return err
	}
	// 先写临时文件再重命名，避免写入中断导致配置损坏。
	tmp := m.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	if err := os.Rename(tmp, m.path); err != nil {
		return err
	}
	m.current = &cfg
	return nil
}

// IsInitialized 系统配置是否已完成。
func (m *Manager) IsInitialized() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.current.Initialized
}

// IsAdminCreated 管理员是否已创建。
func (m *Manager) IsAdminCreated() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.current.AdminCreated
}

// JWTSecret 返回 JWT 签名密钥。
func (m *Manager) JWTSecret() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.current.JWTSecret
}