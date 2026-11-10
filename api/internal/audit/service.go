// Package audit contém a regra de negócio da API: receber uma NF-e,
// registrar, guardar o arquivo e pedir a auditoria ao worker.
//
// Este pacote não sabe nada de HTTP nem de AWS diretamente: ele depende de
// interfaces (Repository, storage.Storage, queue.Publisher). Isso permite
// testar a regra com implementações falsas, sem banco e sem nuvem.
package audit

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ThiagoHeckler/auditor-nfe/api/internal/db"
	"github.com/ThiagoHeckler/auditor-nfe/api/internal/queue"
	"github.com/ThiagoHeckler/auditor-nfe/api/internal/storage"
)

var (
	ErrNotaDuplicada     = errors.New("nota já enviada")
	ErrNotaNaoEncontrada = errors.New("nota não encontrada")
)

// Repository é o subconjunto de consultas que o serviço usa.
// *db.Queries (gerado pelo sqlc) já satisfaz esta interface.
type Repository interface {
	CriarNota(ctx context.Context, arg db.CriarNotaParams) (db.Nota, error)
	BuscarNotaPorID(ctx context.Context, id pgtype.UUID) (db.Nota, error)
	BuscarNotaPorChave(ctx context.Context, chave string) (db.Nota, error)
	ListarNotas(ctx context.Context, arg db.ListarNotasParams) ([]db.Nota, error)
	MarcarNotaComErro(ctx context.Context, arg db.MarcarNotaComErroParams) error
	ListarAchadosDaNota(ctx context.Context, notaID pgtype.UUID) ([]db.Achado, error)
}

type Service struct {
	repo    Repository
	storage storage.Storage
	fila    queue.Publisher
	log     *slog.Logger
}

func NewService(repo Repository, st storage.Storage, fila queue.Publisher, log *slog.Logger) *Service {
	return &Service{repo: repo, storage: st, fila: fila, log: log}
}

// Enviar registra uma NF-e e solicita a auditoria.
//
// Ordem das etapas (e por quê):
//  1. Validar o XML        -> rejeita lixo antes de gastar banco/S3.
//  2. Inserir no banco     -> o UNIQUE da chave "reserva" a nota; duas
//     requisições simultâneas com a mesma nota não passam as duas.
//  3. Enviar para o S3     -> o worker vai ler o arquivo de lá.
//  4. Publicar na fila     -> dispara o worker.
//
// Se 3 ou 4 falharem, a nota fica com status ERRO e a mensagem do erro.
// Limitação conhecida: se o processo morrer entre 2 e 4, a nota fica
// "RECEBIDA" para sempre. A solução robusta é o padrão Transactional Outbox
// (ver docs/06-decisoes-tecnicas.md).
func (s *Service) Enviar(ctx context.Context, nomeArquivo string, conteudo []byte) (Nota, error) {
	chave, err := ExtrairChaveAcesso(conteudo)
	if err != nil {
		return Nota{}, err
	}

	s3Key := fmt.Sprintf("notas/%s.xml", chave)
	registro, err := s.repo.CriarNota(ctx, db.CriarNotaParams{
		ChaveAcesso: chave,
		NomeArquivo: nomeArquivo,
		S3Key:       s3Key,
	})
	if err != nil {
		if violouUnique(err) {
			existente, errBusca := s.repo.BuscarNotaPorChave(ctx, chave)
			if errBusca != nil {
				return Nota{}, fmt.Errorf("buscar nota duplicada: %w", errBusca)
			}
			return paraNota(existente, nil), ErrNotaDuplicada
		}
		return Nota{}, fmt.Errorf("inserir nota: %w", err)
	}

	if err := s.storage.Put(ctx, s3Key, conteudo, "application/xml"); err != nil {
		s.marcarErro(ctx, registro.ID, "falha ao salvar arquivo")
		return Nota{}, err
	}

	msg := queue.AuditoriaSolicitada{NotaID: uuidString(registro.ID), S3Key: s3Key}
	if err := s.fila.PublicarAuditoria(ctx, msg); err != nil {
		s.marcarErro(ctx, registro.ID, "falha ao enfileirar auditoria")
		return Nota{}, err
	}

	s.log.Info("nota recebida", "nota_id", msg.NotaID, "chave", chave)
	return paraNota(registro, nil), nil
}

// Buscar devolve a nota com os achados da auditoria (vazio enquanto processa).
func (s *Service) Buscar(ctx context.Context, id string) (Nota, error) {
	var uid pgtype.UUID
	if err := uid.Scan(id); err != nil {
		return Nota{}, ErrNotaNaoEncontrada // id mal formado = não existe
	}
	registro, err := s.repo.BuscarNotaPorID(ctx, uid)
	if errors.Is(err, pgx.ErrNoRows) {
		return Nota{}, ErrNotaNaoEncontrada
	}
	if err != nil {
		return Nota{}, fmt.Errorf("buscar nota: %w", err)
	}
	achados, err := s.repo.ListarAchadosDaNota(ctx, uid)
	if err != nil {
		return Nota{}, fmt.Errorf("listar achados: %w", err)
	}
	return paraNota(registro, achados), nil
}

func (s *Service) Listar(ctx context.Context, limite, deslocamento int32) ([]Nota, error) {
	registros, err := s.repo.ListarNotas(ctx, db.ListarNotasParams{Limit: limite, Offset: deslocamento})
	if err != nil {
		return nil, fmt.Errorf("listar notas: %w", err)
	}
	notas := make([]Nota, 0, len(registros))
	for _, r := range registros {
		notas = append(notas, paraNota(r, nil))
	}
	return notas, nil
}

func (s *Service) marcarErro(ctx context.Context, id pgtype.UUID, motivo string) {
	err := s.repo.MarcarNotaComErro(ctx, db.MarcarNotaComErroParams{
		ID:           id,
		MensagemErro: pgtype.Text{String: motivo, Valid: true},
	})
	if err != nil {
		s.log.Error("não foi possível marcar nota com erro", "nota_id", uuidString(id), "erro", err)
	}
}

// violouUnique detecta o erro 23505 (unique_violation) do PostgreSQL.
func violouUnique(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}
