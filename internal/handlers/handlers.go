package handlers

import (
	"cm_subscriptions/internal/config"
	"cm_subscriptions/internal/models"
	"context"
	"encoding/json"
	"fmt"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/sqs"
	"github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/jackc/pgx/v4"
	"github.com/prometheus/client_golang/prometheus"
	"log"
	"strings"
	"time"
)

var (
	sentMessagesCounter = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "sent_messages_total",
			Help: "Total number of sent messages",
		},
		[]string{"service"},
	)
)

func init() {
	prometheus.MustRegister(sentMessagesCounter)
}

func StartSQSPoller(cfg config.Config) {
	sess := session.Must(session.NewSession(&aws.Config{
		Region:      aws.String(cfg.AWSRegion),
		Credentials: credentials.NewStaticCredentials(cfg.AWSAccessKeyID, cfg.AWSSecretAccessKey, ""),
	}))

	sqsSvc := sqs.New(sess)

	for {
		result, err := sqsSvc.ReceiveMessage(&sqs.ReceiveMessageInput{
			QueueUrl:            aws.String(cfg.SQSQueueURL),
			MaxNumberOfMessages: aws.Int64(1),
			WaitTimeSeconds:     aws.Int64(20),
		})
		if err != nil {
			log.Printf("Error receiving message: %v", err)
			time.Sleep(10 * time.Second)
			continue
		}

		if len(result.Messages) > 0 {
			for _, message := range result.Messages {
				handleMessage(cfg, message)
				_, err := sqsSvc.DeleteMessage(&sqs.DeleteMessageInput{
					QueueUrl:      aws.String(cfg.SQSQueueURL),
					ReceiptHandle: message.ReceiptHandle,
				})
				if err != nil {
					log.Printf("Error deleting message: %v", err)
				}
			}
		}
	}
}

func handleMessage(cfg config.Config, message *sqs.Message) {
	var msg models.EnrichmentMsg
	err := json.Unmarshal([]byte(*message.Body), &msg)
	if err != nil {
		log.Printf("Error unmarshaling message: %v", err)
		return
	}

	if msg.Event != "shutdown" {
		return
	}

	conn, err := pgx.Connect(context.Background(), cfg.PostgresConnStr)
	if err != nil {
		log.Printf("Error connecting to PostgreSQL: %v", err)
		return
	}
	defer conn.Close(context.Background())

	notifySubscribers(cfg, conn, msg)
}

func notifySubscribers(cfg config.Config, conn *pgx.Conn, msg models.EnrichmentMsg) {
	query := `SELECT tg_id::bigint FROM subscriptions WHERE subscribe_to_kladr = $1 OR subscribe_to_kladr = $2 OR subscribe_to_kladr = $3`
	rows, err := conn.Query(context.Background(), query, msg.RegionKladr, msg.StreetKladr, msg.CityKladr)
	if err != nil {
		log.Printf("Error querying subscriptions: %v", err)
		return
	}
	defer rows.Close()

	bot, err := tgbotapi.NewBotAPI(cfg.TelegramBotToken)
	if err != nil {
		log.Printf("Error creating Telegram bot: %v", err)
		return
	}

	for rows.Next() {
		var tgID int64
		err := rows.Scan(&tgID)
		if err != nil {
			log.Printf("Error scanning row: %v", err)
			continue
		}

		var serviceEmoji string
		switch msg.Service {
		case "WATER":
			serviceEmoji = "💧 Отключение воды"
		case "ELECTRICITY":
			serviceEmoji = "⚡️ Отключение электричества"
		default:
			serviceEmoji = "Отключение"
		}

		houseNumbers := strings.Join(msg.HouseNumbers, ", ")
		houseRanges := strings.Join(msg.HouseRanges, ", ")

		var address string
		if msg.StreetType != nil && *msg.StreetType != "" {
			address = fmt.Sprintf("%s %s, %s %s %s", msg.CityType, msg.City, *msg.StreetType, msg.Street, houseNumbers)
		} else {
			address = fmt.Sprintf("%s %s, %s %s", msg.CityType, msg.City, msg.StreetTypeRaw, msg.Street)
		}

		if houseNumbers != "" && houseRanges != "" {
			address = fmt.Sprintf("%s, %s %s", address, houseNumbers, houseRanges)
		} else if houseNumbers != "" {
			address = fmt.Sprintf("%s, %s", address, houseNumbers)
		} else if houseRanges != "" {
			address = fmt.Sprintf("%s, %s", address, houseRanges)
		}

		eventStart, _ := time.Parse("2006-01-02T15:04:05", msg.EventStart)
		eventStartFormatted := eventStart.Format("02.01.2006 15:04")

		text := fmt.Sprintf("%s по адресу %s с %s\n\n%s", serviceEmoji, address, eventStartFormatted, msg.ShortDescription)

		tgMsg := tgbotapi.NewMessage(tgID, text)
		_, err = bot.Send(tgMsg)
		if err != nil {
			log.Printf("Error sending message to Telegram: %v", err)
		} else {
			sentMessagesCounter.WithLabelValues(msg.Service).Inc()
		}
	}
}
