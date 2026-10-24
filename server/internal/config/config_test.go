package config

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestFromLookupNormalizesPrefixAndSeparatesPublicAPIBase(t *testing.T) {
	cfg, err := FromLookup(lookup(map[string]string{
		"ACCOUNT": "owner", "PASSWORD": "test password", "JWT_SECRET": strings.Repeat("k", 32),
		"APP_URL_PREFIX": "photos", "LISTEN_PORT": "3123", "API_BASE": "https://media.example/photos/api",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.BasePath != "/photos" || cfg.Port != 3123 || cfg.PublicAPIBase != "https://media.example/photos/api" {
		t.Fatalf("unexpected config: base=%q port=%d api=%q", cfg.BasePath, cfg.Port, cfg.PublicAPIBase)
	}
}

func TestFromLookupUsesRelativeSameOriginAPIByDefault(t *testing.T) {
	cfg, err := FromLookup(lookup(map[string]string{
		"ACCOUNT": "owner", "PASSWORD": "pw", "JWT_SECRET": strings.Repeat("k", 32),
		"APP_URL_PREFIX": "/app/",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.PublicAPIBase != "/app/api" {
		t.Fatalf("PublicAPIBase = %q", cfg.PublicAPIBase)
	}
	if cfg.LogLevel != "info" {
		t.Fatalf("default LogLevel = %q, want info", cfg.LogLevel)
	}
	if len(cfg.AllowedIPs) != 0 || len(cfg.TrustedProxies) != 0 {
		t.Fatalf("optional IP configuration defaults = allowed:%v trusted:%v, want empty", cfg.AllowedIPs, cfg.TrustedProxies)
	}
}

func TestFromLookupSetsDatabasePathDefaultAndOverride(t *testing.T) {
	defaultPath, err := filepath.Abs(filepath.Join("db", "nas-tiktok.db"))
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := FromLookup(lookup(map[string]string{
		"ACCOUNT": "owner", "PASSWORD": "pw", "JWT_SECRET": strings.Repeat("k", 32),
	}))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.DatabasePath != defaultPath {
		t.Fatalf("default DatabasePath = %q, want %q", cfg.DatabasePath, defaultPath)
	}

	cfg, err = FromLookup(lookup(map[string]string{
		"ACCOUNT": "owner", "PASSWORD": "pw", "JWT_SECRET": strings.Repeat("k", 32),
		"DB_PATH": "storage/custom.db",
	}))
	if err != nil {
		t.Fatal(err)
	}
	want, err := filepath.Abs("storage/custom.db")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.DatabasePath != want {
		t.Fatalf("configured DatabasePath = %q, want %q", cfg.DatabasePath, want)
	}
}

func TestFromLookupValidatesLogLevel(t *testing.T) {
	base := map[string]string{"ACCOUNT": "owner", "PASSWORD": "pw", "JWT_SECRET": strings.Repeat("k", 32)}
	for _, test := range []struct {
		value string
		want  string
	}{
		{value: "", want: "info"},
		{value: "info", want: "info"},
		{value: "DEBUG", want: "debug"},
		{value: "off", want: "off"},
	} {
		values := make(map[string]string, len(base)+1)
		for key, value := range base {
			values[key] = value
		}
		values["LOG_LEVEL"] = test.value
		cfg, err := FromLookup(lookup(values))
		if err != nil {
			t.Fatalf("LOG_LEVEL=%q: %v", test.value, err)
		}
		if cfg.LogLevel != test.want {
			t.Errorf("LOG_LEVEL=%q parsed as %q, want %q", test.value, cfg.LogLevel, test.want)
		}
	}

	values := make(map[string]string, len(base)+1)
	for key, value := range base {
		values[key] = value
	}
	values["LOG_LEVEL"] = "verbose"
	if _, err := FromLookup(lookup(values)); err == nil {
		t.Fatal("unsupported LOG_LEVEL was accepted")
	}
}

func TestFromLookupLeavesAllowedIPsEmptyWhenExplicitlyEmpty(t *testing.T) {
	cfg, err := FromLookup(lookup(map[string]string{
		"ACCOUNT": "owner", "PASSWORD": "pw", "JWT_SECRET": strings.Repeat("k", 32),
		"ALLOWED_IPS": "",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.AllowedIPs) != 0 {
		t.Fatalf("explicitly empty ALLOWED_IPS = %v, want no configured prefixes", cfg.AllowedIPs)
	}
}

func TestFromLookupSelectsDefaultListenHostFromIPv6Setting(t *testing.T) {
	values := map[string]string{
		"ACCOUNT": "owner", "PASSWORD": "pw", "JWT_SECRET": strings.Repeat("k", 32),
	}
	cfg, err := FromLookup(lookup(values))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.AllowIPv6 || cfg.ListenHost != "0.0.0.0" {
		t.Fatalf("default listen config = IPv6:%t host:%q, want false and 0.0.0.0", cfg.AllowIPv6, cfg.ListenHost)
	}

	values["ALLOW_IPv6"] = "true"
	cfg, err = FromLookup(lookup(values))
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.AllowIPv6 || cfg.ListenHost != "::" {
		t.Fatalf("IPv6 listen config = IPv6:%t host:%q, want true and ::", cfg.AllowIPv6, cfg.ListenHost)
	}
}

func TestFromLookupPreservesExplicitListenHostWhenIPv6IsEnabled(t *testing.T) {
	cfg, err := FromLookup(lookup(map[string]string{
		"ACCOUNT": "owner", "PASSWORD": "pw", "JWT_SECRET": strings.Repeat("k", 32),
		"ALLOW_IPv6": "true", "LISTEN_HOST": "127.0.0.1",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.AllowIPv6 || cfg.ListenHost != "127.0.0.1" {
		t.Fatalf("explicit listen config = IPv6:%t host:%q, want true and 127.0.0.1", cfg.AllowIPv6, cfg.ListenHost)
	}
}

func TestFromLookupPreservesCredentialWhitespaceAndNormalizesOrigins(t *testing.T) {
	cfg, err := FromLookup(lookup(map[string]string{
		"ACCOUNT": " owner ", "PASSWORD": "  test password  ", "JWT_SECRET": strings.Repeat("k", 32),
		"CORS_ORIGINS": "HTTPS://UI.EXAMPLE.TEST/",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Account != " owner " || cfg.Password != "  test password  " || len(cfg.AllowedOrigins) != 1 || cfg.AllowedOrigins[0] != "https://ui.example.test" {
		t.Fatalf("configuration values were not preserved/normalized: %#v", cfg)
	}
}

func TestCookieSameSiteDefaultsToLaxAndRequiresSecureForNone(t *testing.T) {
	base := map[string]string{"ACCOUNT": "owner", "PASSWORD": "pw", "JWT_SECRET": strings.Repeat("k", 32)}
	cfg, err := FromLookup(lookup(base))
	if err != nil || cfg.CookieSameSite != "lax" {
		t.Fatalf("default cookie SameSite = %q, err=%v; want lax", cfg.CookieSameSite, err)
	}

	values := map[string]string{"ACCOUNT": "owner", "PASSWORD": "pw", "JWT_SECRET": strings.Repeat("k", 32), "COOKIE_SAME_SITE": "none"}
	if _, err := FromLookup(lookup(values)); err == nil {
		t.Fatal("SameSite=None was accepted without Secure cookies")
	}
	values["COOKIE_SECURE"] = "true"
	cfg, err = FromLookup(lookup(values))
	if err != nil || cfg.CookieSameSite != "none" || !cfg.CookieSecure {
		t.Fatalf("secure cross-site cookie config = %#v, err=%v", cfg, err)
	}
	values["COOKIE_SAME_SITE"] = "invalid"
	if _, err := FromLookup(lookup(values)); err == nil {
		t.Fatal("invalid cookie SameSite value was accepted")
	}
}

func TestFromLookupRejectsInvalidNetworkAndPortConfig(t *testing.T) {
	base := map[string]string{"ACCOUNT": "owner", "PASSWORD": "pw", "JWT_SECRET": strings.Repeat("k", 32)}
	for key, value := range map[string]string{
		"ALLOWED_IPS": "192.0.2.1/99", "TRUSTED_PROXIES": "not-an-address", "LISTEN_PORT": "70000", "SSL": "yes",
		"APP_URL_PREFIX": "/a/../b", "API_BASE": "/api/%2e%2e/private", "CORS_ORIGINS": "https://example.test/path",
	} {
		t.Run(key, func(t *testing.T) {
			values := make(map[string]string, len(base)+1)
			for current, existing := range base {
				values[current] = existing
			}
			values[key] = value
			if _, err := FromLookup(lookup(values)); err == nil {
				t.Fatalf("invalid %s was accepted", key)
			}
		})
	}
}

func TestFromLookupNormalizesMappedIPv4Prefixes(t *testing.T) {
	base := map[string]string{
		"ACCOUNT": "owner", "PASSWORD": "pw", "JWT_SECRET": strings.Repeat("k", 32),
		"ALLOWED_IPS": "::ffff:192.0.2.0/120", "TRUSTED_PROXIES": "::ffff:198.51.100.0/120",
	}
	cfg, err := FromLookup(lookup(base))
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.AllowedIPs) != 1 || cfg.AllowedIPs[0].String() != "192.0.2.0/24" {
		t.Fatalf("mapped allowed prefix = %v, want 192.0.2.0/24", cfg.AllowedIPs)
	}
	if len(cfg.TrustedProxies) != 1 || cfg.TrustedProxies[0].String() != "198.51.100.0/24" {
		t.Fatalf("mapped trusted prefix = %v, want 198.51.100.0/24", cfg.TrustedProxies)
	}

	base["ALLOWED_IPS"] = "::ffff:192.0.2.0/95"
	if _, err := FromLookup(lookup(base)); err == nil {
		t.Fatal("ambiguous IPv4-mapped prefix shorter than /96 was accepted")
	}
}

func lookup(values map[string]string) func(string) (string, bool) {
	return func(key string) (string, bool) {
		value, ok := values[key]
		return value, ok
	}
}
