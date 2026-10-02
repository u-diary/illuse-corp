// Package httpx はサーバー B・C の HTTP ハンドラーに共通する処理（CORS・送信元の制限・エラー応答）を提供する。
package httpx

import (
	"encoding/json"
	"net"
	"net/http"
	"slices"
	"strings"

	"github.com/u-diary/illuse-corp/backend/internal/ratelimit"
)

// CORS は許可した Origin にだけ CORS ヘッダーを付ける。
type CORS struct {
	AllowedOrigins []string
	// ExposeHeaders はブラウザの JavaScript から読めるようにするレスポンスヘッダー。
	ExposeHeaders []string
}

// Allowed は origin が許可されているかを返す。
func (c CORS) Allowed(origin string) bool {
	return origin != "" && slices.Contains(c.AllowedOrigins, origin)
}

// Wrap は next の応答に CORS ヘッダーを付ける。
func (c CORS) Wrap(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Add("Vary", "Origin")
		if origin := r.Header.Get("Origin"); c.Allowed(origin) {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			if len(c.ExposeHeaders) > 0 {
				w.Header().Set("Access-Control-Expose-Headers", strings.Join(c.ExposeHeaders, ", "))
			}
		}
		next.ServeHTTP(w, r)
	})
}

// Preflight は POST のプリフライトリクエストに応答する。
func (c CORS) Preflight(w http.ResponseWriter, r *http.Request) {
	if !c.Allowed(r.Header.Get("Origin")) {
		w.WriteHeader(http.StatusForbidden)
		return
	}
	w.Header().Set("Access-Control-Allow-Methods", "POST")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
	w.Header().Set("Access-Control-Max-Age", "600")
	w.WriteHeader(http.StatusNoContent)
}

// Admit は API の入口で、Origin とレート制限を確かめる。
// 受け付けない場合はエラーを書き込んで ok=false を返す。
//
// ブラウザ以外からの雑な呼び出しを防ぐため Origin を必須にしている。
// Origin は偽装できるので認証ではなく、あくまで抑止。
func (c CORS) Admit(w http.ResponseWriter, r *http.Request, limiter *ratelimit.Limiter) (ip string, ok bool) {
	if !c.Allowed(r.Header.Get("Origin")) {
		WriteError(w, http.StatusForbidden, "このサイトからの送信は受け付けていません。")
		return "", false
	}
	ip = ClientIP(r)
	if !limiter.Allow(ip) {
		w.Header().Set("Retry-After", "60")
		WriteError(w, http.StatusTooManyRequests, "送信が集中しています。しばらく待ってから再度お試しください。")
		return ip, false
	}
	return ip, true
}

// ClientIP は利用者の IP アドレスを返す。
// サーバーは 127.0.0.1 で待ち受け、Tailscale Funnel が付ける X-Forwarded-For の
// 末尾（プロキシ自身が追記した値）だけを信用する。
func ClientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	if ip := net.ParseIP(host); ip != nil && ip.IsLoopback() {
		if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
			parts := strings.Split(xff, ",")
			return strings.TrimSpace(parts[len(parts)-1])
		}
	}
	return host
}

// WriteError は {"error": msg} を返す。
func WriteError(w http.ResponseWriter, status int, msg string) {
	WriteJSON(w, status, map[string]string{"error": msg})
}

// WriteJSON は v を JSON で返す。
func WriteJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

// Healthz は死活監視用のハンドラー。
func Healthz(w http.ResponseWriter, _ *http.Request) {
	w.Write([]byte("ok\n"))
}
