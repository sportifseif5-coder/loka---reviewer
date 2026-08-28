// Package config loads and validates Loka configuration with inheritance
// across scopes (defaults < user-global < repository-local < explicit).
package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// Mode is the per-repository runtime permission level.
type Mode string

const (
	ModeOffline Mode = "offline"
	ModeOnline  Mode = "online"
	ModeAgent   Mode = "agent"
)

// Valid reports whether m is a known mode.
func (m Mode) Valid() bool {
	switch m {
	case ModeOffline, ModeOnline, ModeAgent:
		return true
	}
	return false
}

// Provider describes one LLM backend in the provider layer.
type Provider struct {
	Name      string `yaml:"name"`
	Model     string `yaml:"model"`
	Local     *bool  `yaml:"local"`
	BaseURL   string `yaml:"base_url"`
	APIKeyEnv string `yaml:"api_key_env"`
}

// IsLocal reports whether the provider is treated as local. A provider is
// local if explicitly marked local or if it has no base URL.
func (p Provider) IsLocal() bool {
	if p.Local != nil {
		return *p.Local
	}
	return p.BaseURL == ""
}

// Budgets bounds review and agent resource usage.
type Budgets struct {
	ReviewTokens    int `yaml:"review_tokens"`
	AgentTimeoutSec int `yaml:"agent_timeout_sec"`
}

// RulesConfig configures rule sources and third-party rule sync.
type RulesConfig struct {
	Include []string `yaml:"include"`
	Sync    []string `yaml:"sync"`
}

// Config is the fully merged configuration for a repository.
type Config struct {
	Mode      Mode        `yaml:"mode"`
	Providers []Provider  `yaml:"providers"`
	Budgets   Budgets     `yaml:"budgets"`
	Rules     RulesConfig `yaml:"rules"`
}

// Default returns the built-in configuration. Offline-first by default.
func Default() *Config {
	local := true
	return &Config{
		Mode: ModeOffline,
		Providers: []Provider{
			{Name: "ollama", Model: "qwen2.5-coder:7b", Local: &local},
		},
		Budgets: Budgets{
			ReviewTokens:    12000,
			AgentTimeoutSec: 90,
		},
		Rules: RulesConfig{
			Include: []string{"review_rules.md"},
		},
	}
}

// Merge overlays only the fields of overlay that are marked present in the
// present set. This keeps later scopes from clobbering earlier scopes with
// default values that were never explicitly set.
func (base *Config) Merge(overlay *Config, present map[string]bool) {
	if present["mode"] {
		base.Mode = overlay.Mode
	}
	if present["providers"] {
		base.Providers = overlay.Providers
	}
	if present["budgets.review_tokens"] {
		base.Budgets.ReviewTokens = overlay.Budgets.ReviewTokens
	}
	if present["budgets.agent_timeout_sec"] {
		base.Budgets.AgentTimeoutSec = overlay.Budgets.AgentTimeoutSec
	}
	if present["rules.include"] {
		base.Rules.Include = overlay.Rules.Include
	}
	if present["rules.sync"] {
		base.Rules.Sync = overlay.Rules.Sync
	}
}

// Validate checks structural validity of the configuration.
func (c *Config) Validate() error {
	if !c.Mode.Valid() {
		return fmt.Errorf("invalid mode %q: must be one of offline, online, agent", c.Mode)
	}
	if c.Budgets.ReviewTokens <= 0 {
		return fmt.Errorf("budgets.review_tokens must be > 0, got %d", c.Budgets.ReviewTokens)
	}
	if c.Budgets.AgentTimeoutSec <= 0 {
		return fmt.Errorf("budgets.agent_timeout_sec must be > 0, got %d", c.Budgets.AgentTimeoutSec)
	}
	for i, p := range c.Providers {
		if strings.TrimSpace(p.Name) == "" {
			return fmt.Errorf("providers[%d].name must not be empty", i)
		}
		if strings.TrimSpace(p.Model) == "" {
			return fmt.Errorf("providers[%d].model must not be empty", i)
		}
		if p.APIKeyEnv != "" && strings.HasPrefix(strings.TrimSpace(p.APIKeyEnv), "MCAI_") {
			return fmt.Errorf("providers[%d].api_key_env references a reserved environment name", i)
		}
	}
	return nil
}

// Parse decodes a YAML document into a Config on top of defaults. It returns
// warnings for unknown keys rather than failing, per the architecture.
func Parse(data []byte) (*Config, []string, error) {
	cfg, _, warnings, err := parseWithPresence(data)
	return cfg, warnings, err
}

// parseWithPresence decodes a YAML document and reports which fields were
// explicitly present, enabling presence-aware merging across scopes.
func parseWithPresence(data []byte) (*Config, map[string]bool, []string, error) {
	var node yaml.Node
	if err := yaml.Unmarshal(data, &node); err != nil {
		return nil, nil, nil, fmt.Errorf("parse yaml: %w", err)
	}
	doc := &node
	if doc.Kind == yaml.DocumentNode && len(doc.Content) > 0 {
		doc = doc.Content[0]
	}
	if doc.Kind != yaml.MappingNode {
		return nil, nil, nil, errors.New("config root must be a mapping")
	}

	var warnings []string
	present := map[string]bool{}
	walkConfigNode(doc, "", knownTopLevel, present, &warnings)

	cfg := Default()
	if err := doc.Decode(cfg); err != nil {
		return nil, present, warnings, fmt.Errorf("decode config: %w", err)
	}
	return cfg, present, warnings, nil
}

var (
	knownTopLevel = map[string]bool{
		"mode": true, "providers": true, "budgets": true, "rules": true,
	}
	knownProvider = map[string]bool{
		"name": true, "model": true, "local": true, "base_url": true, "api_key_env": true,
	}
	knownBudgets = map[string]bool{
		"review_tokens": true, "agent_timeout_sec": true,
	}
	knownRules = map[string]bool{
		"include": true, "sync": true,
	}
)

// walkConfigNode collects unknown keys as warnings, records which keys are
// explicitly present, and descends into nested sections whose keys are known.
func walkConfigNode(n *yaml.Node, path string, known map[string]bool, present map[string]bool, warnings *[]string) {
	switch n.Kind {
	case yaml.MappingNode:
		for i := 0; i+1 < len(n.Content); i += 2 {
			keyNode, valNode := n.Content[i], n.Content[i+1]
			key := keyNode.Value
			full := key
			if path != "" {
				full = path + "." + key
			}
			present[full] = true
			if !known[key] {
				*warnings = append(*warnings, fmt.Sprintf("unknown config key %q", full))
			}
			switch full {
			case "providers":
				walkConfigNode(valNode, full, knownProvider, present, warnings)
			case "budgets":
				walkConfigNode(valNode, full, knownBudgets, present, warnings)
			case "rules":
				walkConfigNode(valNode, full, knownRules, present, warnings)
			}
		}
	case yaml.SequenceNode:
		for _, item := range n.Content {
			walkConfigNode(item, path, known, present, warnings)
		}
	}
}

// UserConfigFile returns the user-global config path, or "" if it cannot be
// determined.
func UserConfigFile() string {
	dir, err := os.UserConfigDir()
	if err != nil {
		return ""
	}
	return filepath.Join(dir, "loka", "review.yaml")
}

// RepoConfigFile returns the repository-local config path.
func RepoConfigFile(repoPath string) string {
	return filepath.Join(repoPath, ".loka", "review.yaml")
}

// Load merges configuration from paths in order; later paths win over
// earlier ones. Missing files produce warnings, not errors. The result is
// validated before returning.
func Load(paths ...string) (*Config, []string, error) {
	cfg := Default()
	var warnings []string
	for _, p := range paths {
		if p == "" {
			continue
		}
		data, err := os.ReadFile(p)
		if errors.Is(err, os.ErrNotExist) {
			warnings = append(warnings, fmt.Sprintf("config file %s not found, skipped", p))
			continue
		}
		if err != nil {
			return nil, warnings, fmt.Errorf("read config %s: %w", p, err)
		}
		overlay, present, w, err := parseWithPresence(data)
		warnings = append(warnings, w...)
		if err != nil {
			return nil, warnings, fmt.Errorf("parse config %s: %w", p, err)
		}
		cfg.Merge(overlay, present)
	}
	if err := cfg.Validate(); err != nil {
		return nil, warnings, err
	}
	return cfg, warnings, nil
}

// LoadForRepo resolves the effective configuration for a repository using
// the standard scope order. explicitPath, if non-empty, has the highest
// precedence.
func LoadForRepo(repoPath, explicitPath string) (*Config, []string, error) {
	var paths []string
	if p := UserConfigFile(); p != "" {
		paths = append(paths, p)
	}
	if repoPath != "" {
		paths = append(paths, RepoConfigFile(repoPath))
	}
	if explicitPath != "" {
		paths = append(paths, explicitPath)
	}
	return Load(paths...)
}
