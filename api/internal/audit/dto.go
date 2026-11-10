package audit

import (
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ThiagoHeckler/auditor-nfe/api/internal/db"
)

// Nota e Achado são os formatos que a API devolve em JSON.
//
// Por que não devolver direto os structs do sqlc (db.Nota)? Porque eles usam
// tipos do driver (pgtype.UUID, pgtype.Text) que viram JSON estranho, e
// porque o formato do banco não deve vazar para o contrato público da API.
// Se a tabela mudar, só este arquivo muda; os clientes não percebem.
type Nota struct {
	ID           string    `json:"id"`
	ChaveAcesso  string    `json:"chave_acesso"`
	NomeArquivo  string    `json:"nome_arquivo"`
	Status       string    `json:"status"`
	MensagemErro *string   `json:"mensagem_erro,omitempty"`
	CriadoEm     time.Time `json:"criado_em"`
	AtualizadoEm time.Time `json:"atualizado_em"`
	Achados      []Achado  `json:"achados,omitempty"`
}

type Achado struct {
	Regra      string  `json:"regra"`
	Severidade string  `json:"severidade"`
	Campo      *string `json:"campo,omitempty"`
	Mensagem   string  `json:"mensagem"`
	Origem     string  `json:"origem"`
}

func paraNota(n db.Nota, achados []db.Achado) Nota {
	out := Nota{
		ID:           uuidString(n.ID),
		ChaveAcesso:  n.ChaveAcesso,
		NomeArquivo:  n.NomeArquivo,
		Status:       n.Status,
		MensagemErro: textoOpcional(n.MensagemErro),
		CriadoEm:     n.CriadoEm.Time,
		AtualizadoEm: n.AtualizadoEm.Time,
	}
	for _, a := range achados {
		out.Achados = append(out.Achados, Achado{
			Regra:      a.Regra,
			Severidade: a.Severidade,
			Campo:      textoOpcional(a.Campo),
			Mensagem:   a.Mensagem,
			Origem:     a.Origem,
		})
	}
	return out
}

func textoOpcional(t pgtype.Text) *string {
	if !t.Valid {
		return nil
	}
	return &t.String
}

func uuidString(u pgtype.UUID) string {
	if !u.Valid {
		return ""
	}
	b := u.Bytes
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}
