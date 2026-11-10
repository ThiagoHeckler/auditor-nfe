package audit

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ThiagoHeckler/auditor-nfe/api/internal/db"
	"github.com/ThiagoHeckler/auditor-nfe/api/internal/queue"
)

// Implementações falsas (fakes) das dependências. Graças às interfaces,
// testamos a regra de negócio sem PostgreSQL, S3 nem SQS.

type fakeRepo struct {
	notas       map[string]db.Nota // por chave de acesso
	erroMarcado string
}

func newFakeRepo() *fakeRepo { return &fakeRepo{notas: map[string]db.Nota{}} }

func (f *fakeRepo) CriarNota(_ context.Context, p db.CriarNotaParams) (db.Nota, error) {
	if _, existe := f.notas[p.ChaveAcesso]; existe {
		return db.Nota{}, &pgconn.PgError{Code: "23505"}
	}
	n := db.Nota{
		ID:          pgtype.UUID{Bytes: [16]byte{byte(len(f.notas) + 1)}, Valid: true},
		ChaveAcesso: p.ChaveAcesso, NomeArquivo: p.NomeArquivo, S3Key: p.S3Key, Status: "RECEBIDA",
	}
	f.notas[p.ChaveAcesso] = n
	return n, nil
}

func (f *fakeRepo) BuscarNotaPorChave(_ context.Context, chave string) (db.Nota, error) {
	if n, ok := f.notas[chave]; ok {
		return n, nil
	}
	return db.Nota{}, pgx.ErrNoRows
}

func (f *fakeRepo) BuscarNotaPorID(_ context.Context, id pgtype.UUID) (db.Nota, error) {
	for _, n := range f.notas {
		if n.ID == id {
			return n, nil
		}
	}
	return db.Nota{}, pgx.ErrNoRows
}

func (f *fakeRepo) ListarNotas(context.Context, db.ListarNotasParams) ([]db.Nota, error) {
	return nil, nil
}

func (f *fakeRepo) MarcarNotaComErro(_ context.Context, p db.MarcarNotaComErroParams) error {
	f.erroMarcado = p.MensagemErro.String
	return nil
}

func (f *fakeRepo) ListarAchadosDaNota(context.Context, pgtype.UUID) ([]db.Achado, error) {
	return nil, nil
}

type fakeStorage struct {
	arquivos map[string][]byte
	falhar   bool
}

func (f *fakeStorage) Put(_ context.Context, key string, data []byte, _ string) error {
	if f.falhar {
		return errors.New("s3 fora do ar")
	}
	f.arquivos[key] = data
	return nil
}

type fakeFila struct{ mensagens []queue.AuditoriaSolicitada }

func (f *fakeFila) PublicarAuditoria(_ context.Context, m queue.AuditoriaSolicitada) error {
	f.mensagens = append(f.mensagens, m)
	return nil
}

const chaveExemplo = "42251012345678000195550010000012341000012342"

var xmlValido = []byte(`<?xml version="1.0"?><nfeProc><NFe><infNFe Id="NFe` + chaveExemplo + `" versao="4.00"></infNFe></NFe></nfeProc>`)

func novoServico() (*Service, *fakeRepo, *fakeStorage, *fakeFila) {
	repo, st, fila := newFakeRepo(), &fakeStorage{arquivos: map[string][]byte{}}, &fakeFila{}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	return NewService(repo, st, fila, log), repo, st, fila
}

func TestEnviar_CaminhoFeliz(t *testing.T) {
	svc, _, st, fila := novoServico()

	nota, err := svc.Enviar(context.Background(), "nota.xml", xmlValido)
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if nota.ChaveAcesso != chaveExemplo || nota.Status != "RECEBIDA" {
		t.Errorf("nota inesperada: %+v", nota)
	}
	if _, ok := st.arquivos["notas/"+chaveExemplo+".xml"]; !ok {
		t.Error("arquivo não foi salvo no storage")
	}
	if len(fila.mensagens) != 1 || fila.mensagens[0].NotaID != nota.ID {
		t.Errorf("mensagem na fila incorreta: %+v", fila.mensagens)
	}
}

func TestEnviar_Duplicada(t *testing.T) {
	svc, _, _, fila := novoServico()
	primeira, _ := svc.Enviar(context.Background(), "a.xml", xmlValido)

	segunda, err := svc.Enviar(context.Background(), "b.xml", xmlValido)
	if !errors.Is(err, ErrNotaDuplicada) {
		t.Fatalf("esperava ErrNotaDuplicada, veio %v", err)
	}
	if segunda.ID != primeira.ID {
		t.Error("deveria devolver a nota já existente")
	}
	if len(fila.mensagens) != 1 {
		t.Error("nota duplicada não pode gerar nova auditoria")
	}
}

func TestEnviar_FalhaNoStorageMarcaErro(t *testing.T) {
	svc, repo, st, fila := novoServico()
	st.falhar = true

	if _, err := svc.Enviar(context.Background(), "a.xml", xmlValido); err == nil {
		t.Fatal("esperava erro")
	}
	if repo.erroMarcado == "" {
		t.Error("a nota deveria ter sido marcada com ERRO")
	}
	if len(fila.mensagens) != 0 {
		t.Error("não deveria publicar na fila se o arquivo não foi salvo")
	}
}

func TestExtrairChaveAcesso(t *testing.T) {
	casos := []struct {
		nome    string
		xml     string
		querErr bool
	}{
		{"válido", string(xmlValido), false},
		{"mal formado", `<NFe><infNFe Id="NFe123">`, true},
		{"sem infNFe", `<NFe></NFe>`, true},
		{"chave curta", `<NFe><infNFe Id="NFe123"></infNFe></NFe>`, true},
		{"chave com letras", `<NFe><infNFe Id="NFe` + chaveExemplo[:43] + `X"></infNFe></NFe>`, true},
	}
	for _, c := range casos {
		t.Run(c.nome, func(t *testing.T) {
			_, err := ExtrairChaveAcesso([]byte(c.xml))
			if (err != nil) != c.querErr {
				t.Errorf("querErr=%v, err=%v", c.querErr, err)
			}
			if err != nil && !errors.Is(err, ErrXMLInvalido) {
				t.Errorf("erro deveria embrulhar ErrXMLInvalido: %v", err)
			}
		})
	}
}
