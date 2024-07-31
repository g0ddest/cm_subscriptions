package config

import (
	"log"
	"os"
)

type Config struct {
	SQSQueueURL        string
	AWSRegion          string
	AWSAccessKeyID     string
	AWSSecretAccessKey string
	PostgresConnStr    string
	TelegramBotToken   string
}

func LoadConfig() Config {
	config := Config{
		SQSQueueURL:        os.Getenv("SQS_QUEUE_URL"),
		AWSRegion:          os.Getenv("AWS_REGION"),
		AWSAccessKeyID:     os.Getenv("AWS_ACCESS_KEY_ID"),
		AWSSecretAccessKey: os.Getenv("AWS_SECRET_ACCESS_KEY"),
		PostgresConnStr:    os.Getenv("POSTGRES_CONN_STR"),
		TelegramBotToken:   os.Getenv("TELEGRAM_BOT_TOKEN"),
	}

	if config.SQSQueueURL == "" ||
		config.AWSRegion == "" ||
		config.AWSAccessKeyID == "" ||
		config.AWSSecretAccessKey == "" ||
		config.PostgresConnStr == "" ||
		config.TelegramBotToken == "" {
		log.Fatalf("One or more environment variables are missing")
	}

	return config
}
