package aliyundeploy

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	// DefaultRegion is used when --region is omitted for Aliyun deploy/auth.
	DefaultRegion = "cn-hongkong"
	// SupportedRegion is an alias for DefaultRegion (legacy name in tests).
	SupportedRegion = DefaultRegion
	DefaultProfile  = "default"
)

// MainlandChinaRegion reports whether region is a mainland China Aliyun region.
// Hong Kong (cn-hongkong) returns false.
func MainlandChinaRegion(region string) bool {
	region = strings.TrimSpace(region)
	if region == "" || region == DefaultRegion {
		return false
	}
	return strings.HasPrefix(region, "cn-")
}

type Config struct {
	Profile         string
	Region          string
	AccessKeyID     string
	AccessKeySecret string
	SecurityToken   string
}

func LoadConfig(cfg Config) (Config, error) {
	loaded := Config{
		Region:          strings.TrimSpace(cfg.Region),
		AccessKeyID:     strings.TrimSpace(cfg.AccessKeyID),
		AccessKeySecret: strings.TrimSpace(cfg.AccessKeySecret),
		SecurityToken:   strings.TrimSpace(cfg.SecurityToken),
	}

	if loaded.AccessKeyID == "" {
		loaded.AccessKeyID = strings.TrimSpace(os.Getenv("ALIBABA_CLOUD_ACCESS_KEY_ID"))
	}
	if loaded.AccessKeySecret == "" {
		loaded.AccessKeySecret = strings.TrimSpace(os.Getenv("ALIBABA_CLOUD_ACCESS_KEY_SECRET"))
	}
	if loaded.SecurityToken == "" {
		loaded.SecurityToken = strings.TrimSpace(os.Getenv("ALIBABA_CLOUD_SECURITY_TOKEN"))
	}

	if loaded.AccessKeyID == "" || loaded.AccessKeySecret == "" {
		if fileCfg, err := readCredentialsFile(defaultString(cfg.Profile, DefaultProfile)); err == nil {
			if loaded.AccessKeyID == "" {
				loaded.AccessKeyID = fileCfg.AccessKeyID
			}
			if loaded.AccessKeySecret == "" {
				loaded.AccessKeySecret = fileCfg.AccessKeySecret
			}
			if loaded.SecurityToken == "" {
				loaded.SecurityToken = fileCfg.SecurityToken
			}
			if loaded.Region == "" {
				loaded.Region = fileCfg.Region
			}
		}
	}

	if loaded.Region == "" {
		loaded.Region = DefaultRegion
	}
	if loaded.AccessKeyID == "" || loaded.AccessKeySecret == "" {
		return Config{}, fmt.Errorf("aliyun credentials are not configured; run: cloud-forge auth aliyun")
	}
	return loaded, nil
}

func CredentialsPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".cloud-forge", "aliyun", "credentials"), nil
}

func readCredentialsFile(profile string) (Config, error) {
	path, err := CredentialsPath()
	if err != nil {
		return Config{}, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return Config{}, err
	}

	section := defaultString(profile, DefaultProfile)
	values := map[string]string{}
	current := ""
	scanner := bufio.NewScanner(strings.NewReader(string(data)))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") {
			continue
		}
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			current = strings.TrimSpace(line[1 : len(line)-1])
			continue
		}
		if current != section {
			continue
		}
		key, val, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		values[strings.TrimSpace(strings.ToLower(key))] = strings.TrimSpace(val)
	}
	if err := scanner.Err(); err != nil {
		return Config{}, err
	}

	cfg := Config{
		AccessKeyID:     values["access_key_id"],
		AccessKeySecret: values["access_key_secret"],
		SecurityToken:   values["security_token"],
		Region:          values["region"],
	}
	if cfg.AccessKeyID == "" && cfg.AccessKeySecret == "" {
		return Config{}, fmt.Errorf("credentials section [%s] not found in %s", section, path)
	}
	return cfg, nil
}

func defaultString(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return strings.TrimSpace(value)
}
