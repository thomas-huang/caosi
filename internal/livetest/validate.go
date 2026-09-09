package livetest

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/thomas-huang/caosi/internal/config"
)

const (
	defaultLiveDirName = ".caosi-live"
	liveConfigDirEnv   = "CAOSI_LIVE_CONFIG_DIR"
	reportMDName       = "live-report.md"
	reportHTMLName     = "live-report.html"
)

func liveConfigDir() (string, error) {
	if d := strings.TrimSpace(os.Getenv(liveConfigDirEnv)); d != "" {
		return d, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, defaultLiveDirName), nil
}

func loadLiveFile(dir string) (*config.File, error) {
	path := config.ProviderFilePath(dir)
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("live Provider File missing: %s (copy providers.jsonc into this Config Directory)", path)
		}
		return nil, err
	}
	stripped, err := config.StripJSONC(raw)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	dups, err := duplicateTopLevelKeys(stripped)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	if len(dups) > 0 {
		return nil, fmt.Errorf("%s: duplicate Provider Name %s", path, strings.Join(dups, ", "))
	}
	file, err := config.Load(dir)
	if err != nil {
		return nil, err
	}
	if err := checkSnapshot(file); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return file, nil
}

func checkSnapshot(file *config.File) error {
	if len(usableNames(file)) == 0 {
		return fmt.Errorf("没有可用的 Provider（列表为空或密钥都是占位符）")
	}
	return nil
}

func usableNames(file *config.File) []string {
	var out []string
	for _, name := range config.Names(file) {
		if !isPlaceholderKey(file.Providers[name].APIKey) {
			out = append(out, name)
		}
	}
	return out
}

func placeholderNames(file *config.File) []string {
	var out []string
	for _, name := range config.Names(file) {
		if isPlaceholderKey(file.Providers[name].APIKey) {
			out = append(out, name)
		}
	}
	return out
}

func missingUpstream(file *config.File) []config.Protocol {
	seen := map[config.Protocol]bool{}
	for _, name := range usableNames(file) {
		seen[file.Providers[name].Protocol] = true
	}
	var miss []config.Protocol
	for _, p := range clientProtocols {
		if !seen[p] {
			miss = append(miss, p)
		}
	}
	return miss
}

func isPlaceholderKey(key string) bool {
	k := strings.ToLower(strings.TrimSpace(key))
	return k == "" || strings.Contains(k, "your-key") || strings.Contains(k, "sk-xxx")
}

func duplicateTopLevelKeys(stripped []byte) ([]string, error) {
	dec := json.NewDecoder(bytes.NewReader(bytes.TrimSpace(stripped)))
	tok, err := dec.Token()
	if err != nil {
		return nil, err
	}
	delim, ok := tok.(json.Delim)
	if !ok || delim != '{' {
		return nil, fmt.Errorf("top level must be an object")
	}
	seen := map[string]int{}
	var dups []string
	for dec.More() {
		tok, err := dec.Token()
		if err != nil {
			return nil, err
		}
		key, ok := tok.(string)
		if !ok {
			return nil, fmt.Errorf("expected object key")
		}
		seen[key]++
		if seen[key] == 2 {
			dups = append(dups, key)
		}
		var skip json.RawMessage
		if err := dec.Decode(&skip); err != nil {
			return nil, err
		}
	}
	return dups, nil
}
