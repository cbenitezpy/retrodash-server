package origins

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestOriginType_IsValid(t *testing.T) {
	tests := []struct {
		t    OriginType
		want bool
	}{
		{OriginTypeGrafana, true},
		{OriginTypeCommand, true},
		{OriginType("unknown"), false},
		{OriginType(""), false},
	}

	for _, tt := range tests {
		t.Run(string(tt.t), func(t *testing.T) {
			if got := tt.t.IsValid(); got != tt.want {
				t.Errorf("OriginType.IsValid() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestNewOrigin(t *testing.T) {
	config := OriginConfig{
		Grafana: &GrafanaConfig{
			URL:       "https://grafana.example.com/d/abc123",
			VerifyTLS: false,
		},
	}

	origin := NewOrigin("Test Dashboard", OriginTypeGrafana, config)

	if origin.ID == "" {
		t.Error("NewOrigin should generate an ID")
	}
	if origin.Name != "Test Dashboard" {
		t.Errorf("Name = %q, want %q", origin.Name, "Test Dashboard")
	}
	if origin.Type != OriginTypeGrafana {
		t.Errorf("Type = %q, want %q", origin.Type, OriginTypeGrafana)
	}
	if origin.Status != StatusConfigured {
		t.Errorf("Status = %q, want %q", origin.Status, StatusConfigured)
	}
	if origin.CreatedAt.IsZero() {
		t.Error("CreatedAt should be set")
	}
	if origin.UpdatedAt.IsZero() {
		t.Error("UpdatedAt should be set")
	}
}

func TestOrigin_Validate(t *testing.T) {
	tests := []struct {
		name    string
		origin  *Origin
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid grafana origin",
			origin: &Origin{
				ID:   "test-id",
				Name: "Test Dashboard",
				Type: OriginTypeGrafana,
				Config: OriginConfig{
					Grafana: &GrafanaConfig{
						URL: "https://grafana.example.com/d/abc123",
					},
				},
			},
			wantErr: false,
		},
		{
			name: "valid command origin",
			origin: &Origin{
				ID:   "test-id",
				Name: "System Monitor",
				Type: OriginTypeCommand,
				Config: OriginConfig{
					Command: &CommandConfig{
						Command: "cmd://htop",
					},
				},
			},
			wantErr: false,
		},
		{
			name: "empty name",
			origin: &Origin{
				ID:   "test-id",
				Name: "",
				Type: OriginTypeGrafana,
				Config: OriginConfig{
					Grafana: &GrafanaConfig{
						URL: "https://grafana.example.com/d/abc123",
					},
				},
			},
			wantErr: true,
			errMsg:  "name is required",
		},
		{
			name: "name too long",
			origin: &Origin{
				ID:   "test-id",
				Name: strings.Repeat("a", 101),
				Type: OriginTypeGrafana,
				Config: OriginConfig{
					Grafana: &GrafanaConfig{
						URL: "https://grafana.example.com/d/abc123",
					},
				},
			},
			wantErr: true,
			errMsg:  "100 characters",
		},
		{
			name: "invalid type",
			origin: &Origin{
				ID:   "test-id",
				Name: "Test",
				Type: OriginType("invalid"),
				Config: OriginConfig{
					Grafana: &GrafanaConfig{
						URL: "https://grafana.example.com/d/abc123",
					},
				},
			},
			wantErr: true,
			errMsg:  "invalid type",
		},
		{
			name: "grafana type with command config",
			origin: &Origin{
				ID:   "test-id",
				Name: "Test",
				Type: OriginTypeGrafana,
				Config: OriginConfig{
					Command: &CommandConfig{
						Command: "cmd://htop",
					},
				},
			},
			wantErr: true,
			errMsg:  "grafana type requires grafana config",
		},
		{
			name: "command type with grafana config",
			origin: &Origin{
				ID:   "test-id",
				Name: "Test",
				Type: OriginTypeCommand,
				Config: OriginConfig{
					Grafana: &GrafanaConfig{
						URL: "https://grafana.example.com/d/abc123",
					},
				},
			},
			wantErr: true,
			errMsg:  "command type requires command config",
		},
		{
			name: "invalid grafana URL",
			origin: &Origin{
				ID:   "test-id",
				Name: "Test",
				Type: OriginTypeGrafana,
				Config: OriginConfig{
					Grafana: &GrafanaConfig{
						URL: "http://localhost:3000", // SSRF blocked
					},
				},
			},
			wantErr: true,
			errMsg:  "SSRF",
		},
		{
			name: "invalid command",
			origin: &Origin{
				ID:   "test-id",
				Name: "Test",
				Type: OriginTypeCommand,
				Config: OriginConfig{
					Command: &CommandConfig{
						Command: "cmd://rm", // Not in allowlist
					},
				},
			},
			wantErr: true,
			errMsg:  "not allowed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.origin.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Origin.Validate() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if err != nil && tt.errMsg != "" && !strings.Contains(err.Error(), tt.errMsg) {
				t.Errorf("Origin.Validate() error = %v, should contain %q", err, tt.errMsg)
			}
		})
	}
}

func TestOrigin_SanitizeForResponse(t *testing.T) {
	origin := &Origin{
		ID:   "test-id",
		Name: "Test Dashboard",
		Type: OriginTypeGrafana,
		Config: OriginConfig{
			Grafana: &GrafanaConfig{
				URL:       "https://grafana.example.com/d/abc123",
				APIKey:    "secret-api-key",
				VerifyTLS: false,
			},
		},
		Status: StatusConfigured,
	}

	sanitized := origin.SanitizeForResponse()

	// Original should still have the API key
	if origin.Config.Grafana.APIKey != "secret-api-key" {
		t.Error("Original origin should still have API key")
	}

	// Sanitized should not have the API key
	if sanitized.Config.Grafana.APIKey != "" {
		t.Errorf("Sanitized origin should not have API key, got %q", sanitized.Config.Grafana.APIKey)
	}

	// Other fields should be preserved
	if sanitized.Config.Grafana.URL != origin.Config.Grafana.URL {
		t.Error("URL should be preserved")
	}
}

func TestOriginConfig_MarshalJSON(t *testing.T) {
	tests := []struct {
		name   string
		config OriginConfig
		want   string
	}{
		{
			name: "grafana config",
			config: OriginConfig{
				Grafana: &GrafanaConfig{
					URL:       "https://grafana.example.com",
					VerifyTLS: true,
				},
			},
			want: `{"url":"https://grafana.example.com","verifyTls":true}`,
		},
		{
			name: "command config",
			config: OriginConfig{
				Command: &CommandConfig{
					Command: "cmd://htop",
				},
			},
			want: `{"command":"cmd://htop"}`,
		},
		{
			name:   "empty config",
			config: OriginConfig{},
			want:   "null",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := json.Marshal(tt.config)
			if err != nil {
				t.Fatalf("MarshalJSON() error = %v", err)
			}
			if string(got) != tt.want {
				t.Errorf("MarshalJSON() = %s, want %s", got, tt.want)
			}
		})
	}
}

func TestOriginConfig_UnmarshalJSON(t *testing.T) {
	tests := []struct {
		name     string
		json     string
		wantType string // "grafana" or "command"
		wantErr  bool
	}{
		{
			name:     "grafana config",
			json:     `{"url":"https://grafana.example.com","verifyTls":true}`,
			wantType: "grafana",
			wantErr:  false,
		},
		{
			name:     "command config",
			json:     `{"command":"cmd://htop"}`,
			wantType: "command",
			wantErr:  false,
		},
		{
			name:    "invalid config",
			json:    `{"foo":"bar"}`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var config OriginConfig
			err := json.Unmarshal([]byte(tt.json), &config)
			if (err != nil) != tt.wantErr {
				t.Errorf("UnmarshalJSON() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr {
				if tt.wantType == "grafana" && config.Grafana == nil {
					t.Error("Expected Grafana config")
				}
				if tt.wantType == "command" && config.Command == nil {
					t.Error("Expected Command config")
				}
			}
		})
	}
}

func TestValidateName(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{"valid name", "Test Dashboard", false},
		{"valid with numbers", "Dashboard 123", false},
		{"valid with special chars", "My-Dashboard_v2", false},
		{"empty name", "", true},
		{"only spaces", "   ", true},
		{"too long", strings.Repeat("a", 101), true},
		{"max length", strings.Repeat("a", 100), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateName(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateName(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			}
		})
	}
}
