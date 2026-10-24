package httpapi

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/ASentimentalMan/NAS-TikTok/server/internal/access"
	"github.com/ASentimentalMan/NAS-TikTok/server/internal/media/catalog"
)

type originDecision struct {
	allowed            bool
	allowedVia         string
	sameOrigin         bool
	sameOriginReason   string
	originScheme       string
	originHost         string
	originPort         string
	requestHost        string
	requestPort        string
	requestScheme      string
	requestTLS         bool
	proxyTrusted       bool
	forwardedProtoSeen bool
	forwardedProtoUsed bool
}

func (handler *Handler) checkOrigin(request *http.Request) originDecision {
	decision := originDecision{
		allowedVia:         "none",
		sameOriginReason:   "origin_not_allowed",
		requestScheme:      "http",
		requestTLS:         request.TLS != nil,
		proxyTrusted:       access.IsTrustedProxy(request, handler.config.TrustedProxies),
		forwardedProtoSeen: len(request.Header.Values("X-Forwarded-Proto")) > 0,
	}
	decision.requestHost, decision.requestPort = hostAndPort(request.Host)

	if request.TLS != nil {
		decision.requestScheme = "https"
	} else if decision.proxyTrusted {
		forwarded := strings.Split(request.Header.Get("X-Forwarded-Proto"), ",")
		candidate := strings.TrimSpace(forwarded[len(forwarded)-1])
		if candidate == "http" || candidate == "https" {
			decision.requestScheme = candidate
			decision.forwardedProtoUsed = true
		}
	}

	origin := request.Header.Get("Origin")
	if origin == "" {
		decision.allowed = true
		decision.allowedVia = "no_origin"
		decision.sameOriginReason = ""
		return decision
	}

	parsed, err := url.Parse(origin)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") ||
		parsed.Host == "" || parsed.Path != "" || parsed.RawQuery != "" || parsed.Fragment != "" {
		decision.sameOriginReason = "origin_invalid"
		return decision
	}

	decision.originScheme = parsed.Scheme
	decision.originHost = parsed.Hostname()
	decision.originPort = parsed.Port()

	hostMatches := strings.EqualFold(parsed.Host, request.Host)
	if hostMatches && parsed.Scheme == decision.requestScheme {
		decision.allowed = true
		decision.allowedVia = "same_origin"
		decision.sameOrigin = true
		decision.sameOriginReason = ""
		return decision
	}
	if !hostMatches {
		decision.sameOriginReason = "origin_host_mismatch"
	} else {
		decision.sameOriginReason = "origin_scheme_mismatch"
	}

	normalizedOrigin := strings.ToLower(parsed.Scheme) + "://" + strings.ToLower(parsed.Host)
	for _, allowed := range handler.config.AllowedOrigins {
		if normalizedOrigin == allowed {
			decision.allowed = true
			decision.allowedVia = "configured_origin"
			// Keep the automatic same-origin failure for diagnostics even when the fallback allows it.
			return decision
		}
	}
	return decision
}

func hostAndPort(value string) (string, string) {
	parsed, err := url.Parse("//" + value)
	if err != nil || parsed.Host == "" {
		return value, ""
	}
	return parsed.Hostname(), parsed.Port()
}

func (handler *Handler) recordOrigin(writer http.ResponseWriter, decision originDecision) {
	if status, ok := writer.(*statusWriter); ok {
		status.origin = decision
		status.originEvaluated = true
	}
}

func writeErrorReason(writer http.ResponseWriter, status int, message, reason string) {
	if response, ok := writer.(*statusWriter); ok {
		response.reason = reason
	}
	writeError(writer, status, message)
}

func (handler *Handler) logRequest(request *http.Request, response *statusWriter, started time.Time) {
	logger := handler.logger
	if logger == nil {
		return
	}

	route := requestRoute(request.URL.Path, handler.config.BasePath)
	isLogin := request.Method == http.MethodPost &&
		request.URL.Path == handler.config.BasePath+"/api/logIn"

	event := "http_request"
	if isLogin {
		event = "login_failed"
		if response.status == http.StatusOK {
			event = "login_success"
		}
	}

	if isLogin {
		if event == "login_success" {
			logger.Infof("event=login_success status=%d client_ip=%s", response.status, response.clientIP)
			return
		}
		reason := response.reason
		if reason == "" {
			reason = "unclassified"
		}
		logger.Infof("event=login_failed status=%d client_ip=%s reason=%s",
			response.status, response.clientIP, reason)
		return
	}

	reason := response.reason
	if response.status >= 500 && reason == "" {
		reason = "internal_error"
	}
	if reason == "" {
		reason = "none"
	}

	originFields := ""
	if response.originEvaluated {
		d := response.origin
		originFields = fmt.Sprintf(
			" same_origin=%t same_origin_reason=%s allowed_via=%s origin_allowed=%t"+
				" origin_scheme=%s origin_host=%s origin_port=%s"+
				" request_host=%s request_port=%s request_scheme=%s request_tls=%t"+
				" proxy_trusted=%t x_forwarded_proto_present=%t x_forwarded_proto_used=%t",
			d.sameOrigin, reasonOrNone(d.sameOriginReason), d.allowedVia, d.allowed,
			quoteLogField(d.originScheme), quoteLogField(d.originHost), quoteLogField(d.originPort),
			quoteLogField(d.requestHost), quoteLogField(d.requestPort), d.requestScheme, d.requestTLS,
			d.proxyTrusted, d.forwardedProtoSeen, d.forwardedProtoUsed,
		)
	}

	logger.Debugf(
		"event=%s method=%s route=%s status=%d client_ip=%s peer_ip=%s duration_ms=%.3f reason=%s%s",
		event, quoteLogField(request.Method), quoteLogField(route), response.status,
		response.clientIP, response.peerIP, float64(time.Since(started))/float64(time.Millisecond),
		reason, originFields,
	)
}

func reasonOrNone(reason string) string {
	if reason == "" {
		return "none"
	}
	return reason
}

func requestRoute(requestPath, basePath string) string {
	if basePath != "" {
		switch {
		case requestPath == basePath:
			requestPath = "/"
		case strings.HasPrefix(requestPath, basePath+"/"):
			requestPath = strings.TrimPrefix(requestPath, basePath)
		}
	}

	switch {
	case requestPath == "/":
		return "/"
	case requestPath == "/api/logIn":
		return "/api/logIn"
	case requestPath == "/api/logout":
		return "/api/logout"
	case requestPath == "/api/captcha":
		return "/api/captcha"
	case requestPath == "/api/session":
		return "/api/session"
	case requestPath == "/api/list":
		return "/api/list"
	case strings.HasPrefix(requestPath, "/api/stream/"):
		return "/api/stream/*"
	case strings.HasPrefix(requestPath, "/assets/"):
		return "/assets/*"
	case requestPath == "/env.js":
		return "/env.js"
	case requestPath == "/healthz":
		return "/healthz"
	default:
		return "other"
	}
}

func quoteLogField(value string) string {
	runes := []rune(strings.ToValidUTF8(value, "�"))
	if len(runes) > 256 {
		runes = runes[:256]
	}
	return strconv.QuoteToASCII(string(runes))
}

func requestPeerIP(remoteAddr string) string {
	host, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		host = strings.Trim(remoteAddr, "[]")
	}
	address, err := netip.ParseAddr(host)
	if err != nil {
		return "unknown"
	}
	return address.Unmap().String()
}

func mediaScanReason(err error) string {
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		return "media_scan_timeout"
	case errors.Is(err, catalog.ErrRootScan) && errors.Is(err, os.ErrPermission):
		return "media_root_permission_denied"
	case errors.Is(err, catalog.ErrRootScan):
		return "media_root_unreadable"
	case errors.Is(err, catalog.ErrPartialScan):
		return "media_scan_partial"
	case errors.Is(err, os.ErrPermission):
		return "media_permission_denied"
	default:
		return "media_scan_failed"
	}
}
