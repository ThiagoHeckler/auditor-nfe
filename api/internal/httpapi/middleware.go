package httpapi

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"log/slog"
	"net/http"
	"time"
)

// Middlewares são funções que "embrulham" um http.Handler para adicionar
// comportamento comum a todas as rotas (log, id de requisição, recuperação
// de panic) sem repetir código em cada handler.

type ctxKey string

const chaveRequestID ctxKey = "request_id"

// requestID gera (ou reaproveita) um X-Request-ID para rastrear a
// requisição nos logs, de ponta a ponta.
func requestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get("X-Request-ID")
		if id == "" {
			b := make([]byte, 8)
			_, _ = rand.Read(b)
			id = hex.EncodeToString(b)
		}
		w.Header().Set("X-Request-ID", id)
		ctx := context.WithValue(r.Context(), chaveRequestID, id)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func requestIDFrom(ctx context.Context) string {
	id, _ := ctx.Value(chaveRequestID).(string)
	return id
}

// statusRecorder guarda o status HTTP escrito, para o log.
type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (s *statusRecorder) WriteHeader(code int) {
	s.status = code
	s.ResponseWriter.WriteHeader(code)
}

// logRequests registra método, caminho, status e duração em JSON.
// Log estruturado é o que CloudWatch / Cloud Logging indexam bem.
func logRequests(log *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		inicio := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)
		log.Info("http",
			"metodo", r.Method,
			"caminho", r.URL.Path,
			"status", rec.status,
			"duracao_ms", time.Since(inicio).Milliseconds(),
			"request_id", requestIDFrom(r.Context()),
		)
	})
}

// recoverPanic impede que um panic num handler derrube o servidor inteiro.
func recoverPanic(log *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if p := recover(); p != nil {
				log.Error("panic", "valor", p, "request_id", requestIDFrom(r.Context()))
				writeError(w, http.StatusInternalServerError, "erro interno")
			}
		}()
		next.ServeHTTP(w, r)
	})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// writeError usa sempre o mesmo formato de erro: {"erro": "..."}.
func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"erro": msg})
}
