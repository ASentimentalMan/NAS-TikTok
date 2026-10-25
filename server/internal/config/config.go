package config

import (
	"bufio"
	"errors"
	"fmt"
	"net/netip"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	AppName          string
	ListenHost       string
	Port             int
	BasePath         string
	PublicAPIBase    string
	MediaRoot        string
	DatabasePath     string
	StaticRoot       string
	Account          string
	Password         string
	JWTSecret        []byte
	SessionTTL       time.Duration
	SSL              bool
	CookieSecure     bool
	CookieSameSite   string
	TLSCertFile      string
	TLSKeyFile       string
	AllowIPv6        bool
	AllowedIPs       []netip.Prefix
	TrustedProxies   []netip.Prefix
	AllowedOrigins   []string
	CaptchaTTL       time.Duration
	CaptchaLimit     int
	SessionLimit     int
	CaptchaRateLimit int
	LoginRateLimit   int
	LogLevel         string
}

func Load() (Config, error) {
	if err := loadDotEnvFiles(); err != nil {
		return Config{}, err
	}
	return FromLookup(os.LookupEnv)
}

func FromLookup(lookup func(string) (string, bool)) (Config, error) {
	get := func(key string) string {
		value, _ := lookup(key)
		return strings.TrimSpace(value)
	}
	raw := func(key string) string {
		value, _ := lookup(key)
		return value
	}
	first := func(keys ...string) string {
		for _, key := range keys {
			if value := get(key); value != "" {
				return value
			}
		}
		return ""
	}
	boolean := func(key string, fallback bool) (bool, error) {
		value := get(key)
		if value == "" {
			return fallback, nil
		}
		switch strings.ToLower(value) {
		case "true":
			return true, nil
		case "false":
			return false, nil
		default:
			return false, fmt.Errorf("%s must be true or false", key)
		}
	}
	positiveInt := func(key string, fallback int) (int, error) {
		value := get(key)
		if value == "" {
			return fallback, nil
		}
		parsed, err := strconv.Atoi(value)
		if err != nil || parsed <= 0 {
			return 0, fmt.Errorf("%s must be a positive integer", key)
		}
		return parsed, nil
	}

	logLevel := strings.ToLower(get("LOG_LEVEL"))
	if logLevel == "" {
		logLevel = "info"
	}
	switch logLevel {
	case "info", "debug", "off":
	default:
		return Config{}, errors.New("LOG_LEVEL must be info, debug, or off")
	}

	allowIPv6, err := boolean("ALLOW_IPv6", false)
	if err != nil {
		return Config{}, err
	}
	ssl, err := boolean("SSL", false)
	if err != nil {
		return Config{}, err
	}
	cookieSecure, err := boolean("COOKIE_SECURE", ssl)
	if err != nil {
		return Config{}, err
	}
	cookieSameSite := strings.ToLower(get("COOKIE_SAME_SITE"))
	if cookieSameSite == "" {
		cookieSameSite = "lax"
	}
	switch cookieSameSite {
	case "lax", "strict":
	case "none":
		if !cookieSecure {
			return Config{}, errors.New("COOKIE_SAME_SITE=none requires COOKIE_SECURE=true")
		}
	default:
		return Config{}, errors.New("COOKIE_SAME_SITE must be lax, strict, or none")
	}
	port, err := positiveInt("LISTEN_PORT", 0)
	if err != nil {
		return Config{}, err
	}
	if port == 0 {
		port, err = positiveInt("VITE_API_PORT", 3000)
		if err != nil {
			return Config{}, err
		}
	}
	if port > 65535 {
		return Config{}, errors.New("LISTEN_PORT must be between 1 and 65535")
	}
	ttlSeconds, err := positiveInt("JWT_EXPIRE", 1200)
	if err != nil {
		return Config{}, err
	}
	if ttlSeconds > int((365*24*time.Hour)/time.Second) {
		return Config{}, errors.New("JWT_EXPIRE is too large")
	}
	captchaLimit, err := positiveInt("CAPTCHA_STORE_LIMIT", 4096)
	if err != nil {
		return Config{}, err
	}
	sessionLimit, err := positiveInt("SESSION_STORE_LIMIT", 4096)
	if err != nil {
		return Config{}, err
	}
	captchaRateLimit, err := positiveInt("CAPTCHA_RATE_LIMIT_PER_MINUTE", 20)
	if err != nil {
		return Config{}, err
	}
	loginRateLimit, err := positiveInt("LOGIN_RATE_LIMIT_PER_MINUTE", 10)
	if err != nil {
		return Config{}, err
	}

	secret := []byte(raw("JWT_SECRET"))
	if len(secret) < 32 {
		return Config{}, errors.New("JWT_SECRET must contain at least 32 bytes")
	}
	account, password := raw("ACCOUNT"), raw("PASSWORD")
	if strings.TrimSpace(account) == "" || strings.TrimSpace(password) == "" {
		return Config{}, errors.New("ACCOUNT and PASSWORD are required")
	}

	basePath, err := normalizeBasePath(get("APP_URL_PREFIX"))
	if err != nil {
		return Config{}, err
	}
	publicAPIBase, err := publicAPIBase(get("API_BASE"), get("VITE_API_URL"), get("VITE_API_PORT"), basePath)
	if err != nil {
		return Config{}, err
	}
	allowedIPs, err := parsePrefixes(get("ALLOWED_IPS"), "ALLOWED_IPS")
	if err != nil {
		return Config{}, err
	}
	trustedProxies, err := parsePrefixes(get("TRUSTED_PROXIES"), "TRUSTED_PROXIES")
	if err != nil {
		return Config{}, err
	}
	origins, err := parseOrigins(get("CORS_ORIGINS"))
	if err != nil {
		return Config{}, err
	}

	host := get("LISTEN_HOST")
	if host == "" {
		if allowIPv6 {
			host = "::"
		} else {
			host = "0.0.0.0"
		}
	}

	mediaRoot := first("MEDIA_ROOT")
	if mediaRoot == "" {
		mediaRoot = filepath.Join("server", "static")
	}
	mediaRoot, err = filepath.Abs(mediaRoot)
	if err != nil {
		return Config{}, errors.New("MEDIA_ROOT is invalid")
	}
	databasePath := first("DB_PATH")
	if databasePath == "" {
		databasePath = filepath.Join("db", "nas-tiktok.db")
	}
	databasePath, err = filepath.Abs(databasePath)
	if err != nil {
		return Config{}, errors.New("DB_PATH is invalid")
	}
	staticRoot := first("STATIC_ROOT")
	if staticRoot == "" {
		staticRoot = filepath.Join("client", "dist")
	}
	staticRoot, err = filepath.Abs(staticRoot)
	if err != nil {
		return Config{}, errors.New("STATIC_ROOT is invalid")
	}

	certFile := first("TLS_CERT_FILE")
	if certFile == "" {
		certFile = filepath.Join("certs", "fullchain.pem")
	}
	keyFile := first("TLS_KEY_FILE")
	if keyFile == "" {
		keyFile = filepath.Join("certs", "privkey.pem")
	}
	if ssl {
		for _, file := range []string{certFile, keyFile} {
			if _, err := os.Stat(file); err != nil {
				return Config{}, fmt.Errorf("TLS file is unavailable: %s", filepath.Base(file))
			}
		}
	}

	appName := first("APP_NAME")
	if appName == "" {
		appName = "NAS-TikTok"
	}
	return Config{
		AppName:          appName,
		ListenHost:       host,
		Port:             port,
		BasePath:         basePath,
		PublicAPIBase:    publicAPIBase,
		MediaRoot:        mediaRoot,
		DatabasePath:     databasePath,
		StaticRoot:       staticRoot,
		Account:          account,
		Password:         password,
		JWTSecret:        secret,
		SessionTTL:       time.Duration(ttlSeconds) * time.Second,
		SSL:              ssl,
		CookieSecure:     cookieSecure,
		CookieSameSite:   cookieSameSite,
		TLSCertFile:      certFile,
		TLSKeyFile:       keyFile,
		AllowIPv6:        allowIPv6,
		AllowedIPs:       allowedIPs,
		TrustedProxies:   trustedProxies,
		AllowedOrigins:   origins,
		CaptchaTTL:       5 * time.Minute,
		CaptchaLimit:     captchaLimit,
		SessionLimit:     sessionLimit,
		CaptchaRateLimit: captchaRateLimit,
		LoginRateLimit:   loginRateLimit,
		LogLevel:         logLevel,
	}, nil
}

func publicAPIBase(explicit, legacyHost, legacyPort, basePath string) (string, error) {
	if explicit == "" && legacyHost == "" {
		return basePath + "/api", nil
	}
	legacy := explicit == ""
	value := explicit
	if legacy {
		value = strings.TrimRight(legacyHost, "/")
		if legacyPort != "" {
			port, err := strconv.Atoi(legacyPort)
			if err != nil || port < 1 || port > 65535 {
				return "", errors.New("VITE_API_PORT must be between 1 and 65535")
			}
			parsedHost, parseErr := url.Parse(value)
			if parseErr != nil || parsedHost.Host == "" || parsedHost.User != nil || parsedHost.RawQuery != "" || parsedHost.Fragment != "" || (parsedHost.Scheme != "http" && parsedHost.Scheme != "https") {
				return "", errors.New("VITE_API_URL must be an HTTP or HTTPS origin")
			}
			parsedHost.Host = parsedHost.Hostname() + ":" + strconv.Itoa(port)
			if strings.Contains(parsedHost.Hostname(), ":") {
				parsedHost.Host = "[" + parsedHost.Hostname() + "]:" + strconv.Itoa(port)
			}
			value = parsedHost.String()
		}
		value += basePath + "/api"
	}
	if strings.HasPrefix(value, "/") {
		apiPath, err := normalizeAPIPath(value)
		if err != nil {
			return "", err
		}
		return apiPath, nil
	}
	parsed, err := url.Parse(value)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", errors.New("API_BASE must be a path or HTTP URL without credentials, query, or fragment")
	}
	parsed.Path = strings.TrimRight(parsed.Path, "/")
	if legacy && parsed.Path == "" {
		parsed.Path = basePath + "/api"
	} else if parsed.Path == "" {
		parsed.Path = basePath + "/api"
	} else if !strings.HasSuffix(parsed.Path, "/api") {
		parsed.Path += "/api"
	}
	if _, err := normalizeAPIPath(parsed.Path); err != nil {
		return "", err
	}
	return strings.TrimRight(parsed.String(), "/"), nil
}

func normalizeAPIPath(value string) (string, error) {
	value = strings.TrimRight(value, "/")
	if value == "" || !strings.HasPrefix(value, "/") || strings.ContainsAny(value, "?#\\%") || strings.Contains(value, "//") {
		return "", errors.New("API_BASE must be a path or HTTP URL without query or fragment")
	}
	for _, segment := range strings.Split(strings.TrimPrefix(value, "/"), "/") {
		if segment == "" || segment == "." || segment == ".." {
			return "", errors.New("API_BASE contains an invalid path segment")
		}
	}
	return value, nil
}

func parseOrigins(value string) ([]string, error) {
	items := splitList(value)
	origins := make([]string, 0, len(items))
	for _, item := range items {
		parsed, err := url.Parse(item)
		if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" || parsed.User != nil || (parsed.Path != "" && parsed.Path != "/") || parsed.RawQuery != "" || parsed.Fragment != "" {
			return nil, errors.New("CORS_ORIGINS must contain HTTP or HTTPS origins without paths or credentials")
		}
		origins = append(origins, strings.ToLower(parsed.Scheme)+"://"+strings.ToLower(parsed.Host))
	}
	return origins, nil
}

func normalizeBasePath(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" || value == "/" {
		return "", nil
	}
	if strings.ContainsAny(value, "?#\\%") {
		return "", errors.New("APP_URL_PREFIX must be a URL path prefix")
	}
	if !strings.HasPrefix(value, "/") {
		value = "/" + value
	}
	value = strings.TrimSuffix(value, "/")
	for index, segment := range strings.Split(value, "/") {
		if index == 0 && segment == "" {
			continue
		}
		if segment == "." || segment == ".." || segment == "" {
			return "", errors.New("APP_URL_PREFIX contains an invalid path segment")
		}
	}
	return value, nil
}

func parsePrefixes(value, name string) ([]netip.Prefix, error) {
	items := splitList(value)
	prefixes := make([]netip.Prefix, 0, len(items))
	for _, item := range items {
		prefix, err := netip.ParsePrefix(item)
		if err != nil {
			address, addrErr := netip.ParseAddr(item)
			if addrErr != nil {
				return nil, fmt.Errorf("%s contains an invalid address or CIDR", name)
			}
			prefix = netip.PrefixFrom(address, address.BitLen())
		} else {
			if prefix.Addr().Is4In6() && prefix.Bits() < 96 {
				return nil, fmt.Errorf("%s contains an invalid IPv4-mapped CIDR", name)
			}
			prefix = prefix.Masked()
		}
		if prefix.Addr().Is4In6() {
			prefix = netip.PrefixFrom(prefix.Addr().Unmap(), prefix.Bits()-96).Masked()
		}
		prefixes = append(prefixes, prefix)
	}
	return prefixes, nil
}

func splitList(value string) []string {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	parts := strings.Split(value, ",")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		if part = strings.TrimSpace(part); part != "" {
			result = append(result, part)
		}
	}
	return result
}

func loadDotEnvFiles() error {
	mode := strings.TrimSpace(os.Getenv("APP_ENV"))
	if mode == "" {
		mode = strings.TrimSpace(os.Getenv("NODE_ENV"))
	}
	files := []string{".env.local"}
	if mode != "" {
		files = append(files, ".env."+mode)
	}
	files = append(files, ".env")
	loaded := make(map[string]struct{})
	for _, name := range files {
		file, err := os.Open(name)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return fmt.Errorf("open environment file %s: %w", name, err)
		}
		scanner := bufio.NewScanner(file)
		lineNumber := 0
		for scanner.Scan() {
			lineNumber++
			line := strings.TrimSpace(scanner.Text())
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			line = strings.TrimPrefix(line, "export ")
			key, value, ok := strings.Cut(line, "=")
			key = strings.TrimSpace(key)
			if !ok || key == "" {
				_ = file.Close()
				return fmt.Errorf("invalid assignment in %s at line %d", name, lineNumber)
			}
			if _, exists := loaded[key]; exists {
				continue
			}
			if _, exists := os.LookupEnv(key); exists {
				loaded[key] = struct{}{}
				continue
			}
			value = strings.TrimSpace(value)
			if len(value) >= 2 && ((value[0] == '"' && value[len(value)-1] == '"') || (value[0] == '\'' && value[len(value)-1] == '\'')) {
				value = value[1 : len(value)-1]
			}
			if err := os.Setenv(key, value); err != nil {
				_ = file.Close()
				return fmt.Errorf("load environment variable from %s", name)
			}
			loaded[key] = struct{}{}
		}
		if err := scanner.Err(); err != nil {
			_ = file.Close()
			return fmt.Errorf("read environment file %s: %w", name, err)
		}
		if err := file.Close(); err != nil {
			return fmt.Errorf("close environment file %s: %w", name, err)
		}
	}
	return nil
}
