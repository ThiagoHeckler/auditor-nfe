// Package config lê a configuração da aplicação a partir de variáveis de
// ambiente (padrão 12-factor). Nada de arquivo de config embutido no binário:
// o mesmo binário roda local, na AWS ou no GCP mudando só o ambiente.
package config

import (
	"fmt"
	"os"
	"strconv"
)

type Config struct {
	Port          string // porta HTTP da API
	DatabaseURL   string // postgres://usuario:senha@host:5432/banco
	AWSRegion     string
	AWSEndpoint   string // vazio na AWS real; http://localstack:4566 no ambiente local
	S3Bucket      string
	SQSQueueURL   string
	MaxUploadByte int64 // tamanho máximo de um XML aceito
}

// Load lê as variáveis e falha cedo se faltar algo obrigatório.
// Falhar na inicialização é melhor do que falhar no primeiro request.
func Load() (Config, error) {
	cfg := Config{
		Port:        getEnv("PORT", "8080"),
		DatabaseURL: os.Getenv("DATABASE_URL"),
		AWSRegion:   getEnv("AWS_REGION", "us-east-1"),
		AWSEndpoint: os.Getenv("AWS_ENDPOINT_URL"),
		S3Bucket:    os.Getenv("S3_BUCKET"),
		SQSQueueURL: os.Getenv("SQS_QUEUE_URL"),
	}

	maxMB, err := strconv.Atoi(getEnv("MAX_UPLOAD_MB", "2"))
	if err != nil || maxMB <= 0 {
		return Config{}, fmt.Errorf("MAX_UPLOAD_MB inválido")
	}
	cfg.MaxUploadByte = int64(maxMB) << 20

	obrigatorias := map[string]string{
		"DATABASE_URL":  cfg.DatabaseURL,
		"S3_BUCKET":     cfg.S3Bucket,
		"SQS_QUEUE_URL": cfg.SQSQueueURL,
	}
	for nome, valor := range obrigatorias {
		if valor == "" {
			return Config{}, fmt.Errorf("variável de ambiente obrigatória ausente: %s", nome)
		}
	}
	return cfg, nil
}

func getEnv(chave, padrao string) string {
	if v := os.Getenv(chave); v != "" {
		return v
	}
	return padrao
}
