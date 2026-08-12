package main

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

type config struct {
	apiURL  string
	token   string
	timeout time.Duration
}

type configOverrides struct {
	apiURL  string
	token   string
	timeout time.Duration
}

func loadConfig(envPath string, environment map[string]string, overrides configOverrides) (config, error) {
	values := map[string]string{}
	if envPath != "" {
		fileValues, err := readEnvFile(envPath)
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return config{}, fmt.Errorf("read env file: %w", err)
		}
		for key, value := range fileValues {
			values[key] = value
		}
	}
	for key, value := range environment {
		if value != "" {
			values[key] = value
		}
	}

	apiURL := overrides.apiURL
	if apiURL == "" {
		apiURL = values["XION_API_URL"]
	}
	if apiURL == "" {
		apiURL = values["XION_LISTEN_ADDR"]
	}
	if apiURL == "" {
		apiURL = "127.0.0.1:8081"
	}
	apiURL = normalizeAPIURL(apiURL)

	token := overrides.token
	if token == "" {
		token = values["XION_SERVICE_TOKEN"]
	}
	if strings.TrimSpace(token) == "" {
		return config{}, errors.New("XION_SERVICE_TOKEN is required (use --token or the env file)")
	}

	timeout := overrides.timeout
	if timeout <= 0 {
		timeout = 30 * time.Second
		if raw := values["XION_TIMEOUT"]; raw != "" {
			parsed, err := time.ParseDuration(raw)
			if err != nil || parsed <= 0 {
				return config{}, fmt.Errorf("invalid XION_TIMEOUT %q", raw)
			}
			timeout = parsed
		}
	}
	return config{apiURL: strings.TrimRight(apiURL, "/"), token: token, timeout: timeout}, nil
}

func readEnvFile(path string) (map[string]string, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	values := map[string]string{}
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok || strings.TrimSpace(key) == "" {
			continue
		}
		values[strings.TrimSpace(key)] = unquoteEnvValue(strings.TrimSpace(value))
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return values, nil
}

func unquoteEnvValue(value string) string {
	if len(value) >= 2 && ((value[0] == '\'' && value[len(value)-1] == '\'') || (value[0] == '"' && value[len(value)-1] == '"')) {
		if value[0] == '"' {
			if unquoted, err := strconv.Unquote(value); err == nil {
				return unquoted
			}
		}
		return value[1 : len(value)-1]
	}
	return value
}

func normalizeAPIURL(value string) string {
	value = strings.TrimSpace(value)
	if strings.HasPrefix(value, "http://") || strings.HasPrefix(value, "https://") {
		return value
	}
	return "http://" + value
}
