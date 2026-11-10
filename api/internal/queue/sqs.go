// Package queue publica mensagens para o worker processar.
//
// A API não audita a nota na hora: ela só registra e enfileira.
// Isso mantém o request rápido (202 Accepted) e permite escalar o worker
// separadamente — auditoria com LLM pode levar vários segundos.
package queue

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
)

// AuditoriaSolicitada é o contrato entre a API (Go) e o worker (Python).
// Se mudar aqui, precisa mudar em worker/auditor/mensagem.py.
type AuditoriaSolicitada struct {
	NotaID string `json:"nota_id"`
	S3Key  string `json:"s3_key"`
}

type Publisher interface {
	PublicarAuditoria(ctx context.Context, msg AuditoriaSolicitada) error
}

type SQSPublisher struct {
	client   *sqs.Client
	queueURL string
}

func NewSQS(client *sqs.Client, queueURL string) *SQSPublisher {
	return &SQSPublisher{client: client, queueURL: queueURL}
}

func (p *SQSPublisher) PublicarAuditoria(ctx context.Context, msg AuditoriaSolicitada) error {
	corpo, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("serializar mensagem: %w", err)
	}
	_, err = p.client.SendMessage(ctx, &sqs.SendMessageInput{
		QueueUrl:    aws.String(p.queueURL),
		MessageBody: aws.String(string(corpo)),
	})
	if err != nil {
		return fmt.Errorf("sqs send: %w", err)
	}
	return nil
}
