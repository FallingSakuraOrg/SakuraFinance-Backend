package server

import (
	"encoding/json"
	"net/http"
)

// decodeJSON 解析请求体 JSON，拒绝未知字段。
func decodeJSON(r *http.Request, dst any) error {
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	return dec.Decode(dst)
}

// writeJSON 统一 JSON 响应。
func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

// ok 返回前端约定的成功响应：{ "status": "success" }。
func ok(w http.ResponseWriter, extra map[string]any) {
	resp := map[string]any{"status": "success"}
	for k, v := range extra {
		resp[k] = v
	}
	writeJSON(w, http.StatusOK, resp)
}

// fail 返回错误响应，message 字段被前端 axios 拦截器读取并提示。
func fail(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]any{
		"status":  "error",
		"message": message,
	})
}

// withCORS 允许前端开发服务器跨域访问。
func withCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}
