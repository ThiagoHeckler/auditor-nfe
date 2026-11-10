// Package httpapi é a camada HTTP: traduz requisições em chamadas ao
// serviço e erros do serviço em códigos HTTP. Não tem regra de negócio.
package httpapi

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/ThiagoHeckler/auditor-nfe/api/internal/audit"
)

// NotaService é o que a camada HTTP precisa do serviço (facilita testes).
type NotaService interface {
	Enviar(ctx context.Context, nomeArquivo string, conteudo []byte) (audit.Nota, error)
	Buscar(ctx context.Context, id string) (audit.Nota, error)
	Listar(ctx context.Context, limite, deslocamento int32) ([]audit.Nota, error)
}

type Handler struct {
	svc       NotaService
	log       *slog.Logger
	maxUpload int64
	pronto    func(ctx context.Context) error // checagem de prontidão (ping no banco)
}

func NewHandler(svc NotaService, log *slog.Logger, maxUpload int64, pronto func(context.Context) error) *Handler {
	return &Handler{svc: svc, log: log, maxUpload: maxUpload, pronto: pronto}
}

// Routes usa o roteador da biblioteca padrão (Go 1.22+ aceita método e
// parâmetros no padrão, ex.: "GET /v1/notas/{id}"), sem framework.
func (h *Handler) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", h.health)
	mux.HandleFunc("GET /ready", h.ready)
	mux.HandleFunc("POST /v1/notas", h.enviarNota)
	mux.HandleFunc("GET /v1/notas", h.listarNotas)
	mux.HandleFunc("GET /v1/notas/{id}", h.buscarNota)

	return requestID(logRequests(h.log, recoverPanic(h.log, mux)))
}

// health: o processo está vivo? (liveness — não toca em dependências)
func (h *Handler) health(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// ready: consigo atender requisições? (readiness — verifica o banco)
func (h *Handler) ready(w http.ResponseWriter, r *http.Request) {
	if err := h.pronto(r.Context()); err != nil {
		writeError(w, http.StatusServiceUnavailable, "banco de dados indisponível")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "pronto"})
}

// POST /v1/notas  (multipart/form-data, campo "arquivo")
func (h *Handler) enviarNota(w http.ResponseWriter, r *http.Request) {
	// Limita o corpo ANTES de ler: protege a API de uploads gigantes.
	r.Body = http.MaxBytesReader(w, r.Body, h.maxUpload+1<<10)

	arquivo, cabecalho, err := r.FormFile("arquivo")
	if err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			writeError(w, http.StatusRequestEntityTooLarge, "arquivo maior que o limite permitido")
			return
		}
		writeError(w, http.StatusBadRequest, `envie o XML no campo "arquivo" (multipart/form-data)`)
		return
	}
	defer arquivo.Close()

	conteudo, err := io.ReadAll(io.LimitReader(arquivo, h.maxUpload+1))
	if err != nil {
		writeError(w, http.StatusBadRequest, "não foi possível ler o arquivo")
		return
	}
	if int64(len(conteudo)) > h.maxUpload {
		writeError(w, http.StatusRequestEntityTooLarge, "arquivo maior que o limite permitido")
		return
	}

	nota, err := h.svc.Enviar(r.Context(), cabecalho.Filename, conteudo)
	switch {
	case errors.Is(err, audit.ErrXMLInvalido):
		writeError(w, http.StatusUnprocessableEntity, err.Error())
	case errors.Is(err, audit.ErrNotaDuplicada):
		// 409 + id da nota existente: o cliente sabe onde consultar.
		writeJSON(w, http.StatusConflict, map[string]any{"erro": err.Error(), "nota": nota})
	case err != nil:
		h.log.Error("falha ao enviar nota", "erro", err, "request_id", requestIDFrom(r.Context()))
		writeError(w, http.StatusInternalServerError, "erro interno")
	default:
		// 202 Accepted: recebido, mas o processamento é assíncrono.
		w.Header().Set("Location", "/v1/notas/"+nota.ID)
		writeJSON(w, http.StatusAccepted, nota)
	}
}

// GET /v1/notas/{id}
func (h *Handler) buscarNota(w http.ResponseWriter, r *http.Request) {
	nota, err := h.svc.Buscar(r.Context(), r.PathValue("id"))
	switch {
	case errors.Is(err, audit.ErrNotaNaoEncontrada):
		writeError(w, http.StatusNotFound, err.Error())
	case err != nil:
		h.log.Error("falha ao buscar nota", "erro", err, "request_id", requestIDFrom(r.Context()))
		writeError(w, http.StatusInternalServerError, "erro interno")
	default:
		writeJSON(w, http.StatusOK, nota)
	}
}

// GET /v1/notas?limite=20&deslocamento=0
func (h *Handler) listarNotas(w http.ResponseWriter, r *http.Request) {
	limite := queryInt(r, "limite", 20, 1, 100)
	deslocamento := queryInt(r, "deslocamento", 0, 0, 1_000_000)

	notas, err := h.svc.Listar(r.Context(), int32(limite), int32(deslocamento))
	if err != nil {
		h.log.Error("falha ao listar notas", "erro", err, "request_id", requestIDFrom(r.Context()))
		writeError(w, http.StatusInternalServerError, "erro interno")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"dados": notas, "limite": limite, "deslocamento": deslocamento})
}

// queryInt lê um inteiro da query string, com padrão e limites.
func queryInt(r *http.Request, nome string, padrao, minimo, maximo int) int {
	v, err := strconv.Atoi(r.URL.Query().Get(nome))
	if err != nil {
		return padrao
	}
	return max(minimo, min(v, maximo)) // min/max são builtins desde o Go 1.21
}
