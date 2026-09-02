package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const (
	ProviderFileName     = "providers.jsonc"
	ReservedProviderName = "health"
)

type Protocol string

const (
	ProtocolOpenAIChat      Protocol = "openai_chat"
	ProtocolOpenAIResponses Protocol = "openai_responses"
	ProtocolClaudeMessages  Protocol = "claude_messages"
	ProtocolGemini          Protocol = "gemini"
)

func (p Protocol) Valid() bool {
	switch p {
	case ProtocolOpenAIChat, ProtocolOpenAIResponses, ProtocolClaudeMessages, ProtocolGemini:
		return true
	}
	return false
}

func (p Protocol) String() string { return string(p) }

// Provider is one named upstream.
type Provider struct {
	Name     string
	BaseURL  string            `json:"base_url"`
	Protocol Protocol          `json:"protocol"`
	APIKey   string            `json:"api_key"`
	Model    string            `json:"model"`
	Headers  map[string]string `json:"headers"`
}

type File struct {
	Providers map[string]*Provider
	Path      string
}

func DefaultConfigDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".caosi"), nil
}

func ProviderFilePath(configDir string) string {
	return filepath.Join(configDir, ProviderFileName)
}

// FirstRunResult is returned when the Provider File did not exist.
type FirstRunResult struct {
	WrotePath string
}

func EnsureProviderFile(configDir string) (*FirstRunResult, error) {
	path := ProviderFilePath(configDir)
	if _, err := os.Stat(path); err == nil {
		return nil, nil
	} else if !os.IsNotExist(err) {
		return nil, err
	}
	if err := os.MkdirAll(configDir, 0o700); err != nil {
		return nil, err
	}
	if err := os.WriteFile(path, []byte(SampleProviderFile), 0o600); err != nil {
		return nil, err
	}
	return &FirstRunResult{WrotePath: path}, nil
}

func Load(configDir string) (*File, error) {
	path := ProviderFilePath(configDir)
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	stripped, err := StripJSONC(raw)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(stripped, &obj); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	out := &File{
		Providers: make(map[string]*Provider, len(obj)),
		Path:      path,
	}
	var errs []string
	for name, rawProv := range obj {
		if name == ReservedProviderName {
			errs = append(errs, fmt.Sprintf("%q 是保留名，/health 是健康检查，不能当 Provider Name", name))
			continue
		}
		if name == "" || strings.Contains(name, "/") {
			errs = append(errs, fmt.Sprintf("非法 Provider Name %q", name))
			continue
		}
		var p Provider
		if err := json.Unmarshal(rawProv, &p); err != nil {
			errs = append(errs, fmt.Sprintf("%s: %v", name, err))
			continue
		}
		p.Name = name
		p.BaseURL = strings.TrimSpace(p.BaseURL)
		p.APIKey = strings.TrimSpace(p.APIKey)
		p.Model = strings.TrimSpace(p.Model)
		if p.BaseURL == "" {
			errs = append(errs, fmt.Sprintf("%s: 缺少 base_url", name))
		}
		if !p.Protocol.Valid() {
			errs = append(errs, fmt.Sprintf("%s: protocol 必须是 openai_chat | openai_responses | claude_messages | gemini", name))
		}
		out.Providers[name] = &p
	}
	if len(errs) > 0 {
		return nil, fmt.Errorf("%s:\n  - %s", path, strings.Join(errs, "\n  - "))
	}
	if len(out.Providers) == 0 {
		return nil, fmt.Errorf("%s: 没有配置任何 Provider，至少需要一条", path)
	}
	return out, nil
}

func Names(f *File) []string {
	if f == nil {
		return nil
	}
	names := make([]string, 0, len(f.Providers))
	for n := range f.Providers {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}
