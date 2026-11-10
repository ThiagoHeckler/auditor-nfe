// Ponto de entrada da API. Aqui só se "monta" a aplicação (injeção de
// dependências manual): lê config, cria clientes, conecta as camadas e sobe
// o servidor HTTP com desligamento gracioso.
package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ThiagoHeckler/auditor-nfe/api/internal/audit"
	"github.com/ThiagoHeckler/auditor-nfe/api/internal/config"
	"github.com/ThiagoHeckler/auditor-nfe/api/internal/db"
	"github.com/ThiagoHeckler/auditor-nfe/api/internal/httpapi"
	"github.com/ThiagoHeckler/auditor-nfe/api/internal/queue"
	"github.com/ThiagoHeckler/auditor-nfe/api/internal/storage"
)

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	if err := run(log); err != nil {
		log.Error("encerrando com erro", "erro", err)
		os.Exit(1)
	}
}

func run(log *slog.Logger) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	// ctx é cancelado quando o processo recebe SIGINT/SIGTERM
	// (Ctrl+C local, ou o ECS/Cloud Run pedindo para parar o container).
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// --- PostgreSQL (pool de conexões) ---
	pool, err := pgxpool.New(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()

	// --- AWS (S3 + SQS) ---
	// As credenciais vêm da cadeia padrão do SDK: variáveis de ambiente
	// localmente, IAM Role quando roda na AWS. Nunca chave no código.
	awsCfg, err := awsconfig.LoadDefaultConfig(ctx, awsconfig.WithRegion(cfg.AWSRegion))
	if err != nil {
		return err
	}
	s3Client := s3.NewFromConfig(awsCfg, func(o *s3.Options) {
		if cfg.AWSEndpoint != "" { // LocalStack
			o.BaseEndpoint = aws.String(cfg.AWSEndpoint)
			o.UsePathStyle = true
		}
	})
	sqsClient := sqs.NewFromConfig(awsCfg, func(o *sqs.Options) {
		if cfg.AWSEndpoint != "" {
			o.BaseEndpoint = aws.String(cfg.AWSEndpoint)
		}
	})

	// --- Montagem das camadas ---
	svc := audit.NewService(
		db.New(pool),
		storage.NewS3(s3Client, cfg.S3Bucket),
		queue.NewSQS(sqsClient, cfg.SQSQueueURL),
		log,
	)
	handler := httpapi.NewHandler(svc, log, cfg.MaxUploadByte, pool.Ping)

	srv := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           handler.Routes(),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	erros := make(chan error, 1)
	go func() {
		log.Info("api ouvindo", "porta", cfg.Port)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			erros <- err
		}
	}()

	select {
	case err := <-erros:
		return err
	case <-ctx.Done():
		// Desligamento gracioso: para de aceitar conexões e espera as
		// requisições em andamento terminarem (até 10s).
		log.Info("desligando")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		return srv.Shutdown(shutdownCtx)
	}
}
