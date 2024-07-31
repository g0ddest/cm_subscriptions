package main

import (
	"cm_subscriptions/internal/bot"
	"cm_subscriptions/internal/config"
	"cm_subscriptions/internal/handlers"
	"cm_subscriptions/internal/metrics"
	"sync"
)

func main() {
	cfg := config.LoadConfig()

	var wg sync.WaitGroup
	wg.Add(3)

	// Запуск обработчика SQS
	go func() {
		defer wg.Done()
		handlers.StartSQSPoller(cfg)
	}()

	// Запуск Telegram бота
	go func() {
		defer wg.Done()
		bot.StartBot(cfg)
	}()

	// Запуск сервера Prometheus для метрик
	go func() {
		defer wg.Done()
		metrics.StartMetricsServer()
	}()

	wg.Wait()
}
