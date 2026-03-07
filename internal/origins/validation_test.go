package origins

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetAllowedCommands(t *testing.T) {
	commands := GetAllowedCommands()
	assert.NotEmpty(t, commands)
	assert.Contains(t, commands, "top")
	assert.Contains(t, commands, "htop")
	assert.Equal(t, len(CommandAllowlist), len(commands))
}

func TestValidateCommand(t *testing.T) {
	tests := []struct {
		name    string
		cmd     string
		wantErr bool
	}{
		{"Valid command", "cmd://top", false},
		{"Valid command with args", "cmd://htop?args=-d,1", false},
		{"Valid command with simple args", "cmd://watch?args=-n,1", false},
		{"Invalid scheme", "http://top", true},
		{"Empty command", "", true},
		{"Not allowed command", "cmd://rm", true},
		{"Invalid args format", "cmd://top?args=rm -rf /", true}, // spaces not allowed
		{"Valid space-separated format", "cmd://top -l 0", false},
		{"Invalid only scheme", "cmd://", true},

		// Command Injection Tests
		{"Semicolon injection", "cmd://htop;rm -rf /", true},
		{"Pipe injection", "cmd://htop|cat", true},
		{"Backtick injection", "cmd://htop`whoami`", true},
		{"Subshell injection", "cmd://watch?args=$(rm)", true},
		{"Dangerous shell command bash", "cmd://bash", true},
		{"Dangerous shell command sh", "cmd://sh", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateCommand(tt.cmd)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestParseCommand(t *testing.T) {
	cmd, args, err := ParseCommand("cmd://htop?args=-d,1")
	assert.NoError(t, err)
	assert.Equal(t, "htop", cmd)
	assert.Equal(t, []string{"-d", "1"}, args)

	_, _, err = ParseCommand("invalid")
	assert.Error(t, err)
}

func TestValidateGrafanaURL(t *testing.T) {
	tests := []struct {
		name    string
		url     string
		wantErr bool
	}{
		{"Valid HTTPS", "https://grafana.com", false},
		{"Valid HTTP", "http://grafana.com", false},
		{"Invalid Scheme", "ftp://grafana.com", true},
		{"Empty", "", true},
		{"Public IP", "http://8.8.8.8", false},
		{"Invalid URL", ":/foo", true},

		// SSRF Protection Tests
		{"Localhost", "http://localhost", true},
		{"Localhost with port", "http://localhost:8080", true},
		{"Loopback IP", "http://127.0.0.1", true},
		{"IPv6 Loopback", "http://[::1]", true},
		{"Private IP 10.x", "http://10.0.0.1", true},
		{"Private IP 172.16.x", "http://172.16.0.1", true},
		{"Private IP 192.168.x", "http://192.168.1.1", true},

		// Valid .local
		{"Local mDNS allowed", "http://grafana.local", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateGrafanaURL(tt.url)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestIsCommandURL(t *testing.T) {
	assert.True(t, IsCommandURL("cmd://top"))
	assert.False(t, IsCommandURL("http://top"))
}
