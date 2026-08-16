package config

import (
	"cmp"
	"crypto/hmac"
	"crypto/sha1" //nolint:gosec // SHA-1 is mandated by the coturn TURN REST API (RFC draft)
	"encoding/base64"
	"strconv"
	"strings"
	"time"

	"github.com/knadh/koanf/parsers/toml"
	"github.com/knadh/koanf/providers/env"
	"github.com/knadh/koanf/providers/file"
	"github.com/knadh/koanf/v2"

	"github.com/shridarpatil/whatomate/pkg/whatsapp"
)

// Config holds all configuration for the application
type Config struct {
	App          AppConfig          `koanf:"app"`
	Server       ServerConfig       `koanf:"server"`
	Database     DatabaseConfig     `koanf:"database"`
	Redis        RedisConfig        `koanf:"redis"`
	JWT          JWTConfig          `koanf:"jwt"`
	WhatsApp     WhatsAppConfig     `koanf:"whatsapp"`
	AI           AIConfig           `koanf:"ai"`
	Storage      StorageConfig      `koanf:"storage"`
	DefaultAdmin DefaultAdminConfig `koanf:"default_admin"`
	RateLimit    RateLimitConfig    `koanf:"rate_limit"`
	Cookie       CookieConfig       `koanf:"cookie"`
	Calling      CallingConfig      `koanf:"calling"`
	TTS          TTSConfig          `koanf:"tts"`
}

type TTSConfig struct {
	Provider string `koanf:"provider"` // "openai", "elevenlabs", "google"

	// Provider specific configs
	OpenAIKey             string `koanf:"openai_key"`
	OpenAIVoice           string `koanf:"openai_voice"`
	ElevenLabsKey         string `koanf:"elevenlabs_key"`
	ElevenLabsVoiceID     string `koanf:"elevenlabs_voice_id"`
	GoogleCredentialsJSON string `koanf:"google_credentials_json"`
	GoogleVoiceName       string `koanf:"google_voice_name"`
}

// defaultTURNCredentialTTLSecs is the lifetime of generated coturn REST
// credentials when an ICE server has a secret but no explicit credential_ttl.
const defaultTURNCredentialTTLSecs = 86400 // 24h

type ICEServerConfig struct {
	URLs       []string `koanf:"urls"`
	Username   string   `koanf:"username"`
	Credential string   `koanf:"credential"`
	// Secret enables coturn's "use-auth-secret" (TURN REST API) mode. When set,
	// short-lived credentials are generated per request instead of using the
	// static Username/Credential above.
	Secret string `koanf:"secret"`
	// CredentialTTL is the lifetime (seconds) of generated credentials. Defaults
	// to defaultTURNCredentialTTLSecs when <= 0. Only used when Secret is set.
	CredentialTTL int `koanf:"credential_ttl"`
}

// ResolveCredentials returns the username and credential to advertise for this
// ICE server. When Secret is set it derives short-lived coturn REST credentials
// (use-auth-secret): the username is "<expiry-unix>" — optionally prefixed onto
// any configured Username as "<expiry-unix>:<username>" — and the credential is
// the base64-encoded HMAC-SHA1 of that username keyed by the secret. When Secret
// is empty, the static Username/Credential are returned unchanged.
func (s ICEServerConfig) ResolveCredentials(now time.Time) (username, credential string) {
	if s.Secret == "" {
		return s.Username, s.Credential
	}

	ttl := s.CredentialTTL
	if ttl <= 0 {
		ttl = defaultTURNCredentialTTLSecs
	}

	expiry := now.Add(time.Duration(ttl) * time.Second).Unix()
	username = strconv.FormatInt(expiry, 10)
	if s.Username != "" {
		username = username + ":" + s.Username
	}

	mac := hmac.New(sha1.New, []byte(s.Secret))
	mac.Write([]byte(username))
	credential = base64.StdEncoding.EncodeToString(mac.Sum(nil))
	return username, credential
}

type CallingConfig struct {
	MaxCallDuration     int               `koanf:"max_call_duration"`
	AudioDir            string            `koanf:"audio_dir"`
	HoldMusicFile       string            `koanf:"hold_music_file"`
	TransferTimeoutSecs int               `koanf:"transfer_timeout_secs"`
	PerAgentTimeoutSecs int               `koanf:"per_agent_timeout_secs"`
	RingbackFile        string            `koanf:"ringback_file"`
	UDPPortMin          uint16            `koanf:"udp_port_min"` // WebRTC UDP port range start (default: 10000)
	UDPPortMax          uint16            `koanf:"udp_port_max"` // WebRTC UDP port range end (default: 10100)
	PublicIP            string            `koanf:"public_ip"`    // Public IP for NAT mapping (required on AWS/cloud)
	RelayOnly           bool              `koanf:"relay_only"`   // Force all media through TURN relay (no direct UDP)
	ICEServers          []ICEServerConfig `koanf:"ice_servers"`
	RecordingEnabled    bool              `koanf:"recording_enabled"` // Enable call recording to S3
}

type AppConfig struct {
	Name          string `koanf:"name"`
	Environment   string `koanf:"environment"` // development, staging, production
	Debug         bool   `koanf:"debug"`
	EncryptionKey string `koanf:"encryption_key"` // AES-256 key for encrypting secrets at rest

	// FrontendDir serves the SPA from this directory on disk instead of from
	// the copy embedded in the binary. Empty (the default) uses the embedded
	// copy, which is what shipped binaries do.
	//
	// dev/config.toml sets it to "frontend/dist" so the backend port reflects
	// the last `make frontend-build` rather than the frontend snapshot taken at
	// the last `make build-prod` — a snapshot that otherwise goes stale with no
	// visible sign. Override with WHATOMATE_APP__FRONTEND_DIR or -frontend-dir.
	FrontendDir string `koanf:"frontend_dir"`

	// EnforceRoutePermissions turns the route table's permission checks from
	// advisory into binding.
	//
	// It defaults to false — shadow mode — because 129 of 224 handlers never
	// checked a permission, so switching them all on at once would start
	// returning 403 to callers who succeed today. In shadow mode every
	// would-be denial is logged with the route and the permission it wanted,
	// which is the data needed to fix role assignments (and any wrong entry in
	// the table) before flipping this to true.
	//
	// Set WHATOMATE_APP__ENFORCE_ROUTE_PERMISSIONS=true to enforce.
	EnforceRoutePermissions bool `koanf:"enforce_route_permissions"`
}

type ServerConfig struct {
	Host           string `koanf:"host"`
	Port           int    `koanf:"port"`
	ReadTimeout    int    `koanf:"read_timeout"`
	WriteTimeout   int    `koanf:"write_timeout"`
	BasePath       string `koanf:"base_path"`       // Base path for frontend (e.g., "/whatomate" for proxy pass)
	AllowedOrigins string `koanf:"allowed_origins"` // Comma-separated list of allowed CORS origins
}

type DatabaseConfig struct {
	Host            string `koanf:"host"`
	Port            int    `koanf:"port"`
	User            string `koanf:"user"`
	Password        string `koanf:"password"`
	Name            string `koanf:"name"`
	SSLMode         string `koanf:"ssl_mode"`
	MaxOpenConns    int    `koanf:"max_open_conns"`
	MaxIdleConns    int    `koanf:"max_idle_conns"`
	ConnMaxLifetime int    `koanf:"conn_max_lifetime"`
}

type RedisConfig struct {
	Host     string `koanf:"host"`
	Port     int    `koanf:"port"`
	Username string `koanf:"username"`
	Password string `koanf:"password"`
	DB       int    `koanf:"db"`
	TLS      bool   `koanf:"tls"`
}

type JWTConfig struct {
	Secret            string `koanf:"secret"`
	AccessExpiryMins  int    `koanf:"access_expiry_mins"`
	RefreshExpiryDays int    `koanf:"refresh_expiry_days"`
}

type WhatsAppConfig struct {
	WebhookVerifyToken string `koanf:"webhook_verify_token"`
	APIVersion         string `koanf:"api_version"`
	BaseURL            string `koanf:"base_url"` // Meta Graph API base URL
	AppID              string `koanf:"app_id"`   // WhatsApp App ID for frontend
	AppSecret          string `koanf:"app_secret"`
	ConfigID           string `koanf:"config_id"` // WhatsApp Config ID for frontend
}

type AIConfig struct {
	OpenAIKey    string `koanf:"openai_key"`
	AnthropicKey string `koanf:"anthropic_key"`
	GoogleKey    string `koanf:"google_key"`
}

type StorageConfig struct {
	Type      string `koanf:"type"` // local, s3
	LocalPath string `koanf:"local_path"`
	S3Bucket  string `koanf:"s3_bucket"`
	S3Region  string `koanf:"s3_region"`
	S3Key     string `koanf:"s3_key"`
	S3Secret  string `koanf:"s3_secret"`
}

type DefaultAdminConfig struct {
	Email    string `koanf:"email"`
	Password string `koanf:"password"`
	FullName string `koanf:"full_name"`
}

type CookieConfig struct {
	Domain string `koanf:"domain"` // Cookie domain (e.g., ".example.com"). Empty = current host.
	Secure bool   `koanf:"secure"` // Set Secure flag. Auto-set true when environment=production.
}

type RateLimitConfig struct {
	Enabled             bool `koanf:"enabled"`
	LoginMaxAttempts    int  `koanf:"login_max_attempts"`
	RegisterMaxAttempts int  `koanf:"register_max_attempts"`
	RefreshMaxAttempts  int  `koanf:"refresh_max_attempts"`
	SSOMaxAttempts      int  `koanf:"sso_max_attempts"`
	WindowSeconds       int  `koanf:"window_seconds"`
	TrustProxy          bool `koanf:"trust_proxy"`
	APIMaxRequests      int  `koanf:"api_max_requests"`
	APIWindowSeconds    int  `koanf:"api_window_seconds"`
}

// Load loads configuration from file and environment variables
func Load(configPath string) (*Config, error) {
	k := koanf.New(".")

	// Load from config file if provided
	if configPath != "" {
		if err := k.Load(file.Provider(configPath), toml.Parser()); err != nil {
			return nil, err
		}
	}

	// Load from environment variables (WHATOMATE_ prefix). A DOUBLE underscore
	// separates config levels; single underscores are preserved as part of the
	// key. This is required because both section and field names contain
	// underscores (e.g. default_admin, rate_limit, whatsapp.app_id) — collapsing
	// every "_" to "." would mangle them (whatsapp.app_id -> whatsapp.app.id), so
	// those keys could never be set via env.
	// e.g. WHATOMATE_DATABASE__HOST -> database.host
	//      WHATOMATE_WHATSAPP__APP_ID -> whatsapp.app_id
	//      WHATOMATE_DEFAULT_ADMIN__EMAIL -> default_admin.email
	if err := k.Load(env.Provider("WHATOMATE_", ".", func(s string) string {
		return strings.ReplaceAll(strings.ToLower(strings.TrimPrefix(s, "WHATOMATE_")), "__", ".")
	}), nil); err != nil {
		return nil, err
	}

	var cfg Config
	if err := k.Unmarshal("", &cfg); err != nil {
		return nil, err
	}

	// Set defaults
	setDefaults(&cfg)

	return &cfg, nil
}

func setDefaults(cfg *Config) {
	cfg.App.Name = cmp.Or(cfg.App.Name, "Whatomate")
	cfg.App.Environment = cmp.Or(cfg.App.Environment, "development")

	cfg.Server.Host = cmp.Or(cfg.Server.Host, "0.0.0.0")
	cfg.Server.Port = cmp.Or(cfg.Server.Port, 8080)
	cfg.Server.ReadTimeout = cmp.Or(cfg.Server.ReadTimeout, 30)
	cfg.Server.WriteTimeout = cmp.Or(cfg.Server.WriteTimeout, 30)

	cfg.Database.Port = cmp.Or(cfg.Database.Port, 5432)
	cfg.Database.SSLMode = cmp.Or(cfg.Database.SSLMode, "disable")
	cfg.Database.MaxOpenConns = cmp.Or(cfg.Database.MaxOpenConns, 25)
	cfg.Database.MaxIdleConns = cmp.Or(cfg.Database.MaxIdleConns, 5)
	cfg.Database.ConnMaxLifetime = cmp.Or(cfg.Database.ConnMaxLifetime, 300)

	cfg.Redis.Port = cmp.Or(cfg.Redis.Port, 6379)

	cfg.JWT.AccessExpiryMins = cmp.Or(cfg.JWT.AccessExpiryMins, 15)
	cfg.JWT.RefreshExpiryDays = cmp.Or(cfg.JWT.RefreshExpiryDays, 1)

	cfg.WhatsApp.APIVersion = cmp.Or(cfg.WhatsApp.APIVersion, whatsapp.DefaultAPIVersion)
	cfg.WhatsApp.BaseURL = cmp.Or(cfg.WhatsApp.BaseURL, "https://graph.facebook.com")

	cfg.Storage.Type = cmp.Or(cfg.Storage.Type, "local")
	cfg.Storage.LocalPath = cmp.Or(cfg.Storage.LocalPath, "./uploads")

	// Default admin credentials (only used during initial setup)
	cfg.DefaultAdmin.Email = cmp.Or(cfg.DefaultAdmin.Email, "admin@admin.com")
	cfg.DefaultAdmin.Password = cmp.Or(cfg.DefaultAdmin.Password, "admin")
	cfg.DefaultAdmin.FullName = cmp.Or(cfg.DefaultAdmin.FullName, "Admin")

	// Cookie defaults — not a zero-value fallback, so it stays a conditional.
	if cfg.App.Environment == "production" {
		cfg.Cookie.Secure = true
	}

	// Rate limiting defaults
	cfg.RateLimit.LoginMaxAttempts = cmp.Or(cfg.RateLimit.LoginMaxAttempts, 10)
	cfg.RateLimit.RegisterMaxAttempts = cmp.Or(cfg.RateLimit.RegisterMaxAttempts, 10)
	cfg.RateLimit.RefreshMaxAttempts = cmp.Or(cfg.RateLimit.RefreshMaxAttempts, 30)
	cfg.RateLimit.SSOMaxAttempts = cmp.Or(cfg.RateLimit.SSOMaxAttempts, 10)
	cfg.RateLimit.WindowSeconds = cmp.Or(cfg.RateLimit.WindowSeconds, 60)

	// Calling defaults
	cfg.Calling.MaxCallDuration = cmp.Or(cfg.Calling.MaxCallDuration, 300)
	cfg.Calling.AudioDir = cmp.Or(cfg.Calling.AudioDir, "./audio")
	cfg.Calling.HoldMusicFile = cmp.Or(cfg.Calling.HoldMusicFile, "hold_music.opus")
	cfg.Calling.TransferTimeoutSecs = cmp.Or(cfg.Calling.TransferTimeoutSecs, 120)
}
