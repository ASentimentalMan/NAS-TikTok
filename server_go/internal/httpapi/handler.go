package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/netip"
	"net/url"
	"path"
	"strings"
	"time"

	"github.com/ASentimentalMan/NAS-TikTok/server_go/internal/access"
	"github.com/ASentimentalMan/NAS-TikTok/server_go/internal/auth"
	"github.com/ASentimentalMan/NAS-TikTok/server_go/internal/captcha"
	"github.com/ASentimentalMan/NAS-TikTok/server_go/internal/config"
	"github.com/ASentimentalMan/NAS-TikTok/server_go/internal/media/catalog"
	"github.com/ASentimentalMan/NAS-TikTok/server_go/internal/media/index"
	"github.com/ASentimentalMan/NAS-TikTok/server_go/internal/media/streaming"
	"github.com/ASentimentalMan/NAS-TikTok/server_go/internal/session"
)

type Handler struct {
	config   config.Config
	auth     *auth.Service
	captcha  *captcha.Service
	sessions *session.Manager
	catalog  *catalog.Catalog
	index    mediaIndex
	stream   *streaming.Handler
	limiter  *rateLimiter
	web      http.Handler
	logger   *log.Logger
}

type mediaIndex interface {
	Sample(context.Context, string) ([]string, bool, error)
	Reinitialize() (bool, error)
	RemoveMissing(string) error
}

func New(cfg config.Config, authenticator *auth.Service, captchas *captcha.Service, sessions *session.Manager, media *catalog.Catalog, mediaIndex mediaIndex) *Handler {
	var removeMissing func(string) error
	if mediaIndex != nil {
		removeMissing = mediaIndex.RemoveMissing
	}
	return &Handler{
		config: cfg, auth: authenticator, captcha: captchas, sessions: sessions,
		catalog: media, index: mediaIndex, stream: streaming.New(media, removeMissing), limiter: newRateLimiter(),
		logger: log.Default(),
	}
}

func (handler *Handler) ServeHTTP(writer http.ResponseWriter, request *http.Request) {
	started := time.Now()
	status := &statusWriter{ResponseWriter: writer, status: http.StatusOK}
	defer handler.logRequest(request, status, started)
	writer = status
	apiPath := handler.config.BasePath + "/api"
	requestPath := request.URL.Path
	apiRequest := requestPath == apiPath || strings.HasPrefix(requestPath, apiPath+"/")
	if apiRequest {
		writer.Header().Set("Cache-Control", "no-store")
		writer.Header().Set("X-Content-Type-Options", "nosniff")
	}

	healthPath := requestPath == "/healthz" || (handler.config.BasePath != "" && requestPath == handler.config.BasePath+"/healthz")
	status.peerIP = requestPeerIP(request.RemoteAddr)
	status.clientIP = "unknown"
	address, err := access.ClientIP(request, handler.config.TrustedProxies)
	if err != nil {
		writeErrorReason(writer, http.StatusForbidden, "禁止访问", "invalid_client_ip")
		return
	}
	status.clientIP = address.String()
	if !access.Allowed(address, handler.config.AllowedIPs) && !(healthPath && address.IsLoopback()) {
		writeErrorReason(writer, http.StatusForbidden, "禁止访问", "ip_not_allowed")
		return
	}
	if healthPath || apiRequest || request.Method == http.MethodOptions {
		if !handler.applyCORS(writer, request) {
			return
		}
	} else {
		handler.applyCORSIfAllowed(writer, request)
	}

	if healthPath {
		if !requireMethod(writer, request, http.MethodGet) {
			return
		}
		writer.Header().Set("Content-Type", "text/plain; charset=utf-8")
		writer.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(writer, "ok\n")
		return
	}
	if request.Method == http.MethodOptions {
		handler.options(writer, request, apiPath)
		return
	}
	if apiRequest {
		if !handler.crossSiteRequestAllowed(request) {
			writeErrorReason(writer, http.StatusForbidden, "请求来源不允许", "cross_site_request_not_allowed")
			return
		}
		handler.serveAPI(writer, request, address, apiPath)
		return
	}
	if handler.config.BasePath != "" && requestPath != handler.config.BasePath && !strings.HasPrefix(requestPath, handler.config.BasePath+"/") {
		http.NotFound(writer, request)
		return
	}
	handler.serveWeb(writer, request)
}

func (handler *Handler) serveAPI(writer http.ResponseWriter, request *http.Request, address netip.Addr, apiPath string) {
	requestPath := request.URL.Path
	switch {
	case requestPath == apiPath+"/captcha":
		if !requireMethod(writer, request, http.MethodGet) {
			return
		}
		if !handler.limiter.allow("captcha:"+address.String(), handler.config.CaptchaRateLimit, time.Now()) {
			writeErrorReason(writer, http.StatusTooManyRequests, "请求过于频繁，请稍后再试", "captcha_rate_limited")
			return
		}
		result, err := handler.captcha.Create()
		if err != nil {
			writeErrorReason(writer, http.StatusServiceUnavailable, "验证码暂不可用", "internal_error")
			return
		}
		writer.Header().Set("Cache-Control", "no-store")
		writeJSON(writer, http.StatusOK, result)
	case requestPath == apiPath+"/logIn":
		handler.login(writer, request, address)
	case requestPath == apiPath+"/session":
		if !requireMethod(writer, request, http.MethodGet) {
			return
		}
		if _, ok := handler.requestClaims(request); !ok {
			writeErrorReason(writer, http.StatusUnauthorized, "用户未登录", "authentication_required")
			return
		}
		writeJSON(writer, http.StatusOK, map[string]any{"data": map[string]bool{"authenticated": true}})
	case requestPath == apiPath+"/reinit":
		handler.reinitialize(writer, request)
	case requestPath == apiPath+"/logout":
		handler.logout(writer, request)
	case requestPath == apiPath+"/list":
		handler.list(writer, request, apiPath)
	case strings.HasPrefix(requestPath, apiPath+"/stream/"):
		handler.media(writer, request, apiPath)
	default:
		writeError(writer, http.StatusNotFound, "接口不存在")
	}
}

type loginRequest struct {
	Account  string `json:"account"`
	Password string `json:"password"`
	CodeID   string `json:"codeId"`
	Code     string `json:"code"`
}

func (handler *Handler) login(writer http.ResponseWriter, request *http.Request, address netip.Addr) {
	if !requireMethod(writer, request, http.MethodPost) {
		return
	}
	if request.Header.Get("Origin") == "" {
		writeErrorReason(writer, http.StatusForbidden, "请求来源不允许", "origin_missing")
		return
	}
	decision := handler.checkOrigin(request)
	handler.recordOrigin(writer, decision)
	if !decision.allowed {
		writeErrorReason(writer, http.StatusForbidden, "请求来源不允许", decision.sameOriginReason)
		return
	}
	if !handler.limiter.allow("login:ip:"+address.String(), handler.config.LoginRateLimit, time.Now()) {
		writeErrorReason(writer, http.StatusTooManyRequests, "登录尝试过于频繁，请稍后再试", "login_rate_limited")
		return
	}
	var body loginRequest
	if err := decodeJSON(writer, request, &body); err != nil {
		writeErrorReason(writer, http.StatusBadRequest, "登录请求格式无效", "login_request_invalid")
		return
	}
	if !handler.captcha.Consume(body.CodeID, body.Code) {
		writeErrorReason(writer, http.StatusBadRequest, "验证码错误或已过期", "captcha_invalid")
		return
	}
	if !handler.limiter.allowLoginAccount(body.Account, handler.config.LoginRateLimit, time.Now()) {
		writeErrorReason(writer, http.StatusTooManyRequests, "登录尝试过于频繁，请稍后再试", "login_rate_limited")
		return
	}
	if !handler.auth.Verify(body.Account, body.Password) {
		handler.limiter.recordLoginFailure(body.Account, time.Now())
		writeErrorReason(writer, http.StatusUnauthorized, "账号或密码错误", "credentials_invalid")
		return
	}
	handler.limiter.recordLoginSuccess(body.Account)
	token, expires, err := handler.sessions.Create(body.Account)
	if err != nil {
		writeErrorReason(writer, http.StatusServiceUnavailable, "登录服务暂不可用", "internal_error")
		return
	}
	http.SetCookie(writer, &http.Cookie{
		Name: session.CookieName, Value: token, Path: session.CookiePath(handler.config.BasePath),
		Expires: expires, MaxAge: int(time.Until(expires).Seconds()), HttpOnly: true,
		Secure: handler.config.CookieSecure, SameSite: handler.cookieSameSite(),
	})
	writer.Header().Set("Cache-Control", "no-store")
	writeJSON(writer, http.StatusOK, map[string]bool{"ok": true})
}

func (handler *Handler) logout(writer http.ResponseWriter, request *http.Request) {
	if !requireMethod(writer, request, http.MethodPost) {
		return
	}
	if request.Header.Get("Origin") == "" {
		writeErrorReason(writer, http.StatusForbidden, "请求来源不允许", "origin_missing")
		return
	}
	decision := handler.checkOrigin(request)
	handler.recordOrigin(writer, decision)
	if !decision.allowed {
		writeErrorReason(writer, http.StatusForbidden, "请求来源不允许", decision.sameOriginReason)
		return
	}
	if cookie, err := request.Cookie(session.CookieName); err == nil {
		handler.sessions.Revoke(cookie.Value)
	}
	http.SetCookie(writer, &http.Cookie{
		Name: session.CookieName, Value: "", Path: session.CookiePath(handler.config.BasePath),
		MaxAge: -1, Expires: time.Unix(1, 0), HttpOnly: true, Secure: handler.config.CookieSecure,
		SameSite: handler.cookieSameSite(),
	})
	writeJSON(writer, http.StatusOK, map[string]bool{"ok": true})
}

func (handler *Handler) list(writer http.ResponseWriter, request *http.Request, apiPath string) {
	if !requireMethod(writer, request, http.MethodGet) {
		return
	}
	if _, ok := handler.requestClaims(request); !ok {
		writeErrorReason(writer, http.StatusUnauthorized, "用户未登录", "authentication_required")
		return
	}
	mediaType := request.URL.Query().Get("type")
	if mediaType != "image" && mediaType != "video" {
		writeErrorReason(writer, http.StatusBadRequest, "媒体类型无效", "invalid_media_type")
		return
	}
	var items []string
	useIndex := false
	if handler.index != nil {
		indexedPaths, ready, err := handler.index.Sample(request.Context(), mediaType)
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return
		}
		if err != nil {
			handler.logger.Printf("event=media_index_sample_failed error=%q", err)
			writeErrorReason(writer, http.StatusServiceUnavailable, "媒体列表暂时不可用", "media_index_unavailable")
			return
		}
		if ready {
			items = indexedPaths
			useIndex = true
		}
	}
	if !useIndex {
		var err error
		items, err = handler.catalog.List(request.Context(), mediaType)
		if err != nil {
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				return
			}
			writeErrorReason(writer, http.StatusServiceUnavailable, "媒体列表暂时不可用", mediaScanReason(err))
			return
		}
	}
	urls := make([]string, 0, len(items))
	for _, item := range items {
		parts := strings.Split(item, "/")
		for index := range parts {
			parts[index] = url.PathEscape(parts[index])
		}
		urls = append(urls, handler.config.PublicAPIBase+"/stream/"+strings.Join(parts, "/"))
	}
	writer.Header().Set("Cache-Control", "private, no-store")
	writeJSON(writer, http.StatusOK, map[string][]string{"data": urls})
}

func (handler *Handler) reinitialize(writer http.ResponseWriter, request *http.Request) {
	if !requireMethod(writer, request, http.MethodGet) {
		return
	}
	if _, ok := handler.requestClaims(request); !ok {
		writeErrorReason(writer, http.StatusUnauthorized, "用户未登录", "authentication_required")
		return
	}
	if handler.index == nil {
		writeErrorReason(writer, http.StatusServiceUnavailable, "媒体索引暂时不可用", "media_index_unavailable")
		return
	}
	started, err := handler.index.Reinitialize()
	if errors.Is(err, index.ErrScanInProgress) || (!started && err == nil) {
		writeErrorReason(writer, http.StatusConflict, "媒体索引初始化正在进行", "media_index_reinitialization_in_progress")
		return
	}
	if err != nil {
		handler.logger.Printf("event=media_index_reinitialize_failed error=%q", err)
		writeErrorReason(writer, http.StatusServiceUnavailable, "媒体索引暂时不可用", "media_index_unavailable")
		return
	}
	writeJSON(writer, http.StatusAccepted, map[string]bool{"ok": true, "started": true})
}

func (handler *Handler) media(writer http.ResponseWriter, request *http.Request, apiPath string) {
	if _, ok := handler.requestClaims(request); !ok {
		writeErrorReason(writer, http.StatusUnauthorized, "用户未登录", "authentication_required")
		return
	}
	relative := strings.TrimPrefix(request.URL.Path, apiPath+"/stream/")
	if relative == "" || path.Clean(relative) != relative || strings.HasPrefix(relative, "/") {
		writeError(writer, http.StatusNotFound, "文件不存在")
		return
	}
	handler.stream.ServeHTTP(writer, request, relative)
}

func (handler *Handler) requestClaims(request *http.Request) (session.Claims, bool) {
	cookie, err := request.Cookie(session.CookieName)
	if err != nil {
		return session.Claims{}, false
	}
	return handler.sessions.Validate(cookie.Value)
}

func (handler *Handler) applyCORS(writer http.ResponseWriter, request *http.Request) bool {
	origin := request.Header.Get("Origin")
	if origin == "" {
		return true
	}
	decision := handler.checkOrigin(request)
	handler.recordOrigin(writer, decision)
	if !decision.allowed {
		writeErrorReason(writer, http.StatusForbidden, "请求来源不允许", decision.sameOriginReason)
		return false
	}
	handler.setCORSHeaders(writer, origin)
	return true
}

func (handler *Handler) applyCORSIfAllowed(writer http.ResponseWriter, request *http.Request) {
	origin := request.Header.Get("Origin")
	if origin == "" {
		writer.Header().Add("Vary", "Origin")
		return
	}
	decision := handler.checkOrigin(request)
	handler.recordOrigin(writer, decision)
	if !decision.allowed {
		writer.Header().Add("Vary", "Origin")
		return
	}
	handler.setCORSHeaders(writer, origin)
}

func (handler *Handler) setCORSHeaders(writer http.ResponseWriter, origin string) {
	writer.Header().Set("Access-Control-Allow-Origin", origin)
	writer.Header().Set("Access-Control-Allow-Credentials", "true")
	writer.Header().Set("Access-Control-Expose-Headers", "Accept-Ranges, Content-Length, Content-Range")
	writer.Header().Add("Vary", "Origin")
}

func (handler *Handler) options(writer http.ResponseWriter, request *http.Request, apiPath string) {
	methods := handler.allowedMethods(request.URL.Path, apiPath)
	if len(methods) == 0 {
		writeError(writer, http.StatusNotFound, "接口不存在")
		return
	}
	allow := append(append([]string(nil), methods...), http.MethodOptions)
	writer.Header().Set("Allow", strings.Join(allow, ", "))

	requestedMethod := strings.ToUpper(strings.TrimSpace(request.Header.Get("Access-Control-Request-Method")))
	if requestedMethod != "" && !containsString(methods, requestedMethod) {
		writeError(writer, http.StatusMethodNotAllowed, "请求方法不支持")
		return
	}

	requestedHeaders := request.Header.Get("Access-Control-Request-Headers")
	if requestedHeaders != "" {
		for _, header := range strings.Split(requestedHeaders, ",") {
			name := strings.ToLower(strings.TrimSpace(header))
			if name != "content-type" && name != "range" {
				writeErrorReason(writer, http.StatusForbidden, "请求头不允许", "preflight_header_not_allowed")
				return
			}
		}
		writer.Header().Set("Access-Control-Allow-Headers", "Content-Type, Range")
	}
	writer.Header().Set("Access-Control-Allow-Methods", strings.Join(methods, ", "))
	writer.Header().Add("Vary", "Access-Control-Request-Method")
	writer.Header().Add("Vary", "Access-Control-Request-Headers")
	writer.WriteHeader(http.StatusNoContent)
}

func (handler *Handler) allowedMethods(requestPath, apiPath string) []string {
	switch {
	case requestPath == apiPath+"/captcha", requestPath == apiPath+"/session", requestPath == apiPath+"/list", requestPath == apiPath+"/reinit":
		return []string{http.MethodGet}
	case requestPath == apiPath+"/logIn", requestPath == apiPath+"/logout":
		return []string{http.MethodPost}
	case strings.HasPrefix(requestPath, apiPath+"/stream/"):
		return []string{http.MethodGet, http.MethodHead}
	default:
		return nil
	}
}

func containsString(values []string, value string) bool {
	for _, candidate := range values {
		if candidate == value {
			return true
		}
	}
	return false
}

func (handler *Handler) cookieSameSite() http.SameSite {
	switch strings.ToLower(handler.config.CookieSameSite) {
	case "strict":
		return http.SameSiteStrictMode
	case "none":
		return http.SameSiteNoneMode
	default:
		return http.SameSiteLaxMode
	}
}

func (handler *Handler) crossSiteRequestAllowed(request *http.Request) bool {
	if !strings.EqualFold(handler.config.CookieSameSite, "none") || !strings.EqualFold(request.Header.Get("Sec-Fetch-Site"), "cross-site") {
		return true
	}
	if request.Header.Get("Origin") != "" {
		return handler.originAllowed(request)
	}
	referrer, err := url.Parse(request.Referer())
	if err != nil || (referrer.Scheme != "http" && referrer.Scheme != "https") || referrer.Host == "" || referrer.User != nil {
		return false
	}
	copy := request.Clone(request.Context())
	copy.Header = request.Header.Clone()
	copy.Header.Set("Origin", referrer.Scheme+"://"+referrer.Host)
	return handler.originAllowed(copy)
}

func (handler *Handler) originAllowed(request *http.Request) bool {
	return handler.checkOrigin(request).allowed
}

func (handler *Handler) serveWeb(writer http.ResponseWriter, request *http.Request) {
	// Installed by the application after API routes have been assembled.
	if handler.web == nil {
		http.NotFound(writer, request)
		return
	}
	handler.web.ServeHTTP(writer, request)
}

func (handler *Handler) SetWeb(web http.Handler) {
	handler.web = web
}

type statusWriter struct {
	http.ResponseWriter
	status          int
	reason          string
	clientIP        string
	peerIP          string
	origin          originDecision
	originEvaluated bool
}

func (writer *statusWriter) WriteHeader(code int) {
	writer.status = code
	writer.ResponseWriter.WriteHeader(code)
}

func (writer *statusWriter) Write(value []byte) (int, error) {
	return writer.ResponseWriter.Write(value)
}

func (writer *statusWriter) Flush() {
	if flusher, ok := writer.ResponseWriter.(http.Flusher); ok {
		flusher.Flush()
	}
}

func (writer *statusWriter) Unwrap() http.ResponseWriter {
	return writer.ResponseWriter
}

func decodeJSON(writer http.ResponseWriter, request *http.Request, target any) error {
	request.Body = http.MaxBytesReader(writer, request.Body, 16<<10)
	decoder := json.NewDecoder(request.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	if err := decoder.Decode(new(any)); !errors.Is(err, io.EOF) {
		return fmt.Errorf("invalid trailing JSON data")
	}
	return nil
}

func requireMethod(writer http.ResponseWriter, request *http.Request, methods ...string) bool {
	for _, method := range methods {
		if request.Method == method {
			return true
		}
	}
	writer.Header().Set("Allow", strings.Join(methods, ", "))
	writeErrorReason(writer, http.StatusMethodNotAllowed, "请求方法不支持", "method_not_allowed")
	return false
}

func writeError(writer http.ResponseWriter, status int, message string) {
	writeJSON(writer, status, map[string]string{"message": message})
}

func writeJSON(writer http.ResponseWriter, status int, value any) {
	writer.Header().Set("Content-Type", "application/json; charset=utf-8")
	writer.WriteHeader(status)
	_ = json.NewEncoder(writer).Encode(value)
}
