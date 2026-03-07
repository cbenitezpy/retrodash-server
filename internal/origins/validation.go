package origins

import (
	"errors"
	"fmt"
	"net"
	"net/url"
	"regexp"
	"strings"
)

// CommandAllowlist defines allowed commands for terminal origins.
// Only commands in this list can be executed.
var CommandAllowlist = map[string]bool{
	"htop":    true,
	"top":     true,
	"btop":    true,
	"glances": true,
	"watch":   true,
	"df":      true,
	"free":    true,
	"uptime":  true,
}

// commandPatternQueryParams validates the cmd:// format with query params args.
// Format: cmd://command or cmd://command?args=arg1,arg2
var commandPatternQueryParams = regexp.MustCompile(`^cmd://([a-zA-Z0-9_-]+)(\?args=([a-zA-Z0-9,._=-]*))?$`)

// argsPattern validates that arguments are alphanumeric only (no shell metacharacters).
var argsPattern = regexp.MustCompile(`^[a-zA-Z0-9._=-]+$`)

// dangerousChars are shell metacharacters that could lead to command injection.
var dangerousChars = []string{";", "|", "&", "$", "`", "(", ")", "<", ">", "\n", "\r", "'", "\"", "\\"}

// ValidateCommand validates a command URL against the allowlist.
// Supports both formats:
//   - Query params: cmd://top?args=-l,0
//   - Space-separated: cmd://top -l 0
func ValidateCommand(cmdURL string) error {
	if cmdURL == "" {
		return errors.New("command is required")
	}

	if !strings.HasPrefix(cmdURL, "cmd://") {
		return errors.New("command must start with cmd://")
	}

	cmdPart := strings.TrimPrefix(cmdURL, "cmd://")
	if cmdPart == "" {
		return errors.New("command is required after cmd://")
	}

	var command string
	var args []string

	// Try query params format first: cmd://top?args=-l,0
	if matches := commandPatternQueryParams.FindStringSubmatch(cmdURL); matches != nil {
		command = matches[1]
		if len(matches) > 3 && matches[3] != "" {
			args = strings.Split(matches[3], ",")
		}
	} else {
		// Try space-separated format: cmd://top -l 0
		parts := strings.Fields(cmdPart)
		if len(parts) == 0 {
			return errors.New("command is required after cmd://")
		}
		command = parts[0]
		if len(parts) > 1 {
			args = parts[1:]
		}
	}

	// Validate command name is alphanumeric
	if !regexp.MustCompile(`^[a-zA-Z0-9_-]+$`).MatchString(command) {
		return fmt.Errorf("invalid command name '%s': must be alphanumeric", command)
	}

	// Check if command is in allowlist
	if !CommandAllowlist[command] {
		allowedList := make([]string, 0, len(CommandAllowlist))
		for cmd := range CommandAllowlist {
			allowedList = append(allowedList, cmd)
		}
		return fmt.Errorf("command '%s' is not allowed; allowed commands: %s",
			command, strings.Join(allowedList, ", "))
	}

	// Validate each argument for dangerous characters
	for i, arg := range args {
		for _, dangerous := range dangerousChars {
			if strings.Contains(arg, dangerous) {
				return fmt.Errorf("argument %d contains forbidden character '%s' (security restriction)", i+1, dangerous)
			}
		}
		// Also validate against pattern
		if !argsPattern.MatchString(arg) {
			return fmt.Errorf("argument %d '%s' contains invalid characters (only alphanumeric, dot, underscore, hyphen, equals allowed)", i+1, arg)
		}
	}

	return nil
}

// ParseCommand extracts the command name and arguments from a cmd:// URL.
// Supports both formats:
//   - Query params: cmd://top?args=-l,0
//   - Space-separated: cmd://top -l 0
func ParseCommand(cmdURL string) (command string, args []string, err error) {
	if err := ValidateCommand(cmdURL); err != nil {
		return "", nil, err
	}

	cmdPart := strings.TrimPrefix(cmdURL, "cmd://")

	// Try query params format first
	if matches := commandPatternQueryParams.FindStringSubmatch(cmdURL); matches != nil {
		command = matches[1]
		if len(matches) > 3 && matches[3] != "" {
			args = strings.Split(matches[3], ",")
		}
	} else {
		// Space-separated format
		parts := strings.Fields(cmdPart)
		command = parts[0]
		if len(parts) > 1 {
			args = parts[1:]
		}
	}

	return command, args, nil
}

// ValidateGrafanaURL validates a Grafana dashboard URL with SSRF protection.
func ValidateGrafanaURL(rawURL string) error {
	if rawURL == "" {
		return errors.New("url is required")
	}

	parsed, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("invalid URL: %w", err)
	}

	// Must be http or https
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return errors.New("url must use http:// or https:// scheme")
	}

	// Must have a host
	if parsed.Host == "" {
		return errors.New("url must have a host")
	}

	// SSRF protection: block private IPs and localhost
	if err := validateHostNotPrivate(parsed.Host); err != nil {
		return err
	}

	return nil
}

// validateHostNotPrivate checks that the host is not a private IP or localhost.
func validateHostNotPrivate(host string) error {
	// Remove port if present
	hostname := host
	if h, _, err := net.SplitHostPort(host); err == nil {
		hostname = h
	}

	// Remove brackets from IPv6 addresses
	hostname = strings.Trim(hostname, "[]")

	// Block localhost variants (but allow .local for homelab mDNS)
	lowered := strings.ToLower(hostname)
	if lowered == "localhost" ||
		lowered == "127.0.0.1" ||
		lowered == "::1" ||
		strings.HasSuffix(lowered, ".localhost") {
		return errors.New("url cannot point to localhost (SSRF protection)")
	}

	// Try to parse as IP
	ip := net.ParseIP(hostname)
	if ip != nil {
		if isPrivateIP(ip) {
			return errors.New("url cannot point to private IP addresses (SSRF protection)")
		}
	}

	return nil
}

// isPrivateIP checks if an IP is in a private range.
func isPrivateIP(ip net.IP) bool {
	// Check for loopback
	if ip.IsLoopback() {
		return true
	}

	// Check for private ranges
	privateRanges := []string{
		"10.0.0.0/8",
		"172.16.0.0/12",
		"192.168.0.0/16",
		"169.254.0.0/16", // Link-local
		"fc00::/7",       // IPv6 unique local
		"fe80::/10",      // IPv6 link-local
	}

	for _, cidr := range privateRanges {
		_, network, err := net.ParseCIDR(cidr)
		if err != nil {
			continue
		}
		if network.Contains(ip) {
			return true
		}
	}

	return false
}

// IsCommandURL checks if a URL is a command URL (cmd://).
func IsCommandURL(rawURL string) bool {
	return strings.HasPrefix(rawURL, "cmd://")
}

// GetAllowedCommands returns the list of allowed commands for terminal origins.
func GetAllowedCommands() []string {
	commands := make([]string, 0, len(CommandAllowlist))
	for cmd := range CommandAllowlist {
		commands = append(commands, cmd)
	}
	return commands
}
