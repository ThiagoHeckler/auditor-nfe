package httpapi

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ThiagoHeckler/auditor-nfe/api/internal/audit"
)

// fakeService simula o serviço para testar só a camada HTTP:
// códigos de status, formato do JSON e leitura do multipart.
type fakeService struct{ erroEnviar error }

func (f fakeService) Enviar(_ context.Context, nome string, _ []byte) (audit.Nota, error) {
	return audit.Nota{ID: "abc", NomeArquivo: nome, Status: "RECEBIDA"}, f.erroEnviar
}

func (f fakeService) Buscar(_ context.Context, id string) (audit.Nota, error) {
	if id != "abc" {
		return audit.Nota{}, audit.ErrNotaNaoEncontrada
	}
	return audit.Nota{ID: "abc"}, nil
}

func (f fakeService) Listar(context.Context, int32, int32) ([]audit.Nota, error) {
	return []audit.Nota{}, nil
}

func novoRouter(svc NotaService) http.Handler {
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	pronto := func(context.Context) error { return nil }
	return NewHandler(svc, log, 1<<20, pronto).Routes()
}

func requisicaoUpload(t *testing.T, campo string, conteudo []byte) *http.Request {
	t.Helper()
	var corpo bytes.Buffer
	mw := multipart.NewWriter(&corpo)
	parte, _ := mw.CreateFormFile(campo, "nota.xml")
	_, _ = parte.Write(conteudo)
	_ = mw.Close()
	req := httptest.NewRequest(http.MethodPost, "/v1/notas", &corpo)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	return req
}

func TestEnviarNota_StatusHTTP(t *testing.T) {
	casos := []struct {
		nome       string
		campo      string
		erroSvc    error
		statusQuer int
	}{
		{"sucesso devolve 202", "arquivo", nil, http.StatusAccepted},
		{"campo errado devolve 400", "outro", nil, http.StatusBadRequest},
		{"xml inválido devolve 422", "arquivo", audit.ErrXMLInvalido, http.StatusUnprocessableEntity},
		{"duplicada devolve 409", "arquivo", audit.ErrNotaDuplicada, http.StatusConflict},
		{"erro inesperado devolve 500", "arquivo", io.ErrUnexpectedEOF, http.StatusInternalServerError},
	}
	for _, c := range casos {
		t.Run(c.nome, func(t *testing.T) {
			rec := httptest.NewRecorder()
			novoRouter(fakeService{erroEnviar: c.erroSvc}).ServeHTTP(rec, requisicaoUpload(t, c.campo, []byte("<x/>")))
			if rec.Code != c.statusQuer {
				t.Errorf("status = %d, quer %d (corpo: %s)", rec.Code, c.statusQuer, rec.Body)
			}
		})
	}
}

func TestEnviarNota_ArquivoGrandeDevolve413(t *testing.T) {
	rec := httptest.NewRecorder()
	grande := bytes.Repeat([]byte("a"), 2<<20) // 2 MB > limite de 1 MB do teste
	novoRouter(fakeService{}).ServeHTTP(rec, requisicaoUpload(t, "arquivo", grande))
	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Errorf("status = %d, quer 413", rec.Code)
	}
}

func TestBuscarNota(t *testing.T) {
	router := novoRouter(fakeService{})

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/v1/notas/abc", nil))
	if rec.Code != http.StatusOK {
		t.Errorf("existente: status = %d, quer 200", rec.Code)
	}

	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/v1/notas/nao-existe", nil))
	if rec.Code != http.StatusNotFound {
		t.Errorf("inexistente: status = %d, quer 404", rec.Code)
	}
}
