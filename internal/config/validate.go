package config

import (
	"fmt"
	"net/url"
	"strings"
)

// Validate checks the account configuration for structural problems.
func (c *Settings) Validate() error {
	if c == nil {
		return fmt.Errorf("configuration is nil")
	}

	problems := make([]string, 0)
	if c.SchemaVersion != SchemaVersion {
		problems = append(problems, "only schema version 1 is supported")
	}
	if err := validateProxyURL(c.ProxyBaseURL); err != nil {
		problems = append(problems, err.Error())
	}
	if c.ContextWindow <= 0 {
		problems = append(problems, "context_window must be greater than 0")
	}
	if c.MaxOutputTokens <= 0 {
		problems = append(problems, "max_output_tokens must be greater than 0")
	}
	if c.Session.Compaction.TriggerRatio < 0 || c.Session.Compaction.TriggerRatio > 1 {
		problems = append(problems, "session.compaction.trigger_ratio must be between 0 and 1")
	}

	if len(problems) == 0 {
		return nil
	}
	return fmt.Errorf("%s", strings.Join(problems, "; "))
}

func validateProxyURL(raw string) error {
	if strings.TrimSpace(raw) == "" {
		return fmt.Errorf("proxy_base_url is required")
	}
	u, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("proxy_base_url is invalid: %w", err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("proxy_base_url must use http or https")
	}
	if u.Host == "" {
		return fmt.Errorf("proxy_base_url must include a host")
	}
	return nil
}
