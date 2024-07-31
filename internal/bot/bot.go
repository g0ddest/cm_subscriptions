package bot

import (
	"cm_subscriptions/internal/config"
	"cm_subscriptions/internal/metrics"
	"context"
	"encoding/json"
	"fmt"
	"github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v4"
	"io/ioutil"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"
)

func StartBot(cfg config.Config) {
	bot, err := tgbotapi.NewBotAPI(cfg.TelegramBotToken)
	if err != nil {
		log.Fatalf("Ошибка при создании Telegram бота: %v", err)
	}

	commands := []tgbotapi.BotCommand{
		{Command: "subscribe", Description: "Подписаться на адрес"},
		{Command: "list", Description: "Показать все подписки"},
		{Command: "delete", Description: "Удалить подписку"},
	}

	_, err = bot.Request(tgbotapi.NewSetMyCommands(commands...))
	if err != nil {
		log.Fatalf("Ошибка при установке команд бота: %v", err)
	}

	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60

	updates := bot.GetUpdatesChan(u)

	for update := range updates {
		if update.Message != nil {
			handleUpdate(cfg, bot, update.Message)
		} else if update.CallbackQuery != nil {
			handleCallbackQuery(cfg, bot, update.CallbackQuery)
		}
	}
}

func handleUpdate(cfg config.Config, bot *tgbotapi.BotAPI, message *tgbotapi.Message) {
	if message.IsCommand() {
		switch message.Command() {
		case "start":
			bot.Send(tgbotapi.NewMessage(message.Chat.ID, "Приветствую! Тут вы можете подписаться на события по отключению коммунальных услуг в ПМР. Учтите, что это не официальный источник информации и мы можем совершать ошибки. Если хотите быть 100% уверены в информации, лучше следите отдельно за каждым поставщиком информации. Начните свой путь с подписки на интересующий вас адрес командой /subscribe и далее адрес в формате город улица"))
		case "subscribe":
			handleSubscribe(cfg, bot, message)
		case "list":
			handleList(cfg, bot, message)
		case "delete":
			handleDelete(cfg, bot, message)
		default:
			msg := tgbotapi.NewMessage(message.Chat.ID, "Неизвестная команда. Доступные команды: /subscribe, /list, /delete")
			bot.Send(msg)
		}
	}
}

func handleCallbackQuery(cfg config.Config, bot *tgbotapi.BotAPI, callbackQuery *tgbotapi.CallbackQuery) {
	data := callbackQuery.Data
	prefix := data[:2]
	payload := data[2:]

	switch prefix {
	case "s:":
		handleSubscriptionCallback(cfg, bot, callbackQuery, payload)
	case "d:":
		handleDeleteCallback(cfg, bot, callbackQuery, payload)
	default:
		log.Printf("Неизвестный префикс в данных кнопки: %v", data)
	}
}

func handleSubscriptionCallback(cfg config.Config, bot *tgbotapi.BotAPI, callbackQuery *tgbotapi.CallbackQuery, kladr string) {
	// Запрос для получения полной информации по КЛАДР
	url := fmt.Sprintf("https://address.md/kladr/%s", kladr)
	resp, err := http.Get(url)
	if err != nil {
		log.Printf("Ошибка при запросе к API КЛАДР: %v", err)
		return
	}
	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		log.Printf("Ошибка при чтении ответа от API КЛАДР: %v", err)
		return
	}

	var kladrInfo map[string]interface{}
	err = json.Unmarshal(body, &kladrInfo)
	if err != nil {
		log.Printf("Ошибка при разборе ответа от API КЛАДР: %v", err)
		return
	}

	fullAddress := kladrInfo["full_address"].(string)

	conn, err := pgx.Connect(context.Background(), cfg.PostgresConnStr)
	if err != nil {
		log.Printf("Ошибка подключения к PostgreSQL: %v", err)
		metrics.SubscriptionCounter.WithLabelValues("failure").Inc()
		return
	}
	defer conn.Close(context.Background())

	query := `INSERT INTO subscriptions (id, created_at, subscribe_to_kladr, subscribe_to_fulltext, tg_id) VALUES ($1, $2, $3, $4, $5)`
	_, err = conn.Exec(context.Background(), query, uuid.New(), time.Now(), kladr, fullAddress, strconv.FormatInt(callbackQuery.From.ID, 10))
	if err != nil {
		log.Printf("Ошибка вставки подписки: %v", err)
		metrics.SubscriptionCounter.WithLabelValues("failure").Inc()
		return
	}

	// Скрыть кнопки после нажатия
	emptyKeyboard := tgbotapi.InlineKeyboardMarkup{
		InlineKeyboard: [][]tgbotapi.InlineKeyboardButton{},
	}
	edit := tgbotapi.NewEditMessageReplyMarkup(callbackQuery.Message.Chat.ID, callbackQuery.Message.MessageID, emptyKeyboard)
	if _, err := bot.Request(edit); err != nil {
		log.Printf("Ошибка при скрытии кнопок: %v", err)
	}

	msg := tgbotapi.NewMessage(callbackQuery.Message.Chat.ID, fmt.Sprintf("Подписка на %s успешно оформлена.", fullAddress))
	bot.Send(msg)
	metrics.SubscriptionCounter.WithLabelValues("success").Inc()

	callback := tgbotapi.NewCallback(callbackQuery.ID, "Подписка оформлена!")
	if _, err := bot.Request(callback); err != nil {
		log.Printf("Ошибка при ответе на CallbackQuery: %v", err)
	}
}

func handleDeleteCallback(cfg config.Config, bot *tgbotapi.BotAPI, callbackQuery *tgbotapi.CallbackQuery, subscriptionID string) {
	conn, err := pgx.Connect(context.Background(), cfg.PostgresConnStr)
	if err != nil {
		log.Printf("Ошибка подключения к PostgreSQL: %v", err)
		return
	}
	defer conn.Close(context.Background())

	query := `DELETE FROM subscriptions WHERE id = $1 AND tg_id = $2`
	_, err = conn.Exec(context.Background(), query, subscriptionID, strconv.FormatInt(callbackQuery.From.ID, 10))
	if err != nil {
		log.Printf("Ошибка удаления подписки: %v", err)
		return
	}

	// Скрыть кнопки после нажатия
	emptyKeyboard := tgbotapi.InlineKeyboardMarkup{
		InlineKeyboard: [][]tgbotapi.InlineKeyboardButton{},
	}
	edit := tgbotapi.NewEditMessageReplyMarkup(callbackQuery.Message.Chat.ID, callbackQuery.Message.MessageID, emptyKeyboard)
	if _, err := bot.Request(edit); err != nil {
		log.Printf("Ошибка при скрытии кнопок: %v", err)
	}

	msg := tgbotapi.NewMessage(callbackQuery.Message.Chat.ID, "Подписка успешно удалена.")
	bot.Send(msg)
}
func handleSubscribe(cfg config.Config, bot *tgbotapi.BotAPI, message *tgbotapi.Message) {
	address := message.CommandArguments()
	if address == "" {
		msg := tgbotapi.NewMessage(message.Chat.ID, "Пожалуйста, передайте с командой адрес в формате «/subscribe город улица». Подписка может быть только на всю улицу, или на весь город. Подписки на отдельные дома планируются.")
		bot.Send(msg)
		return
	}

	url := "https://address.md/parse"
	resp, err := http.PostForm(url, map[string][]string{"address": {address}})
	if err != nil {
		msg := tgbotapi.NewMessage(message.Chat.ID, "Ошибка при запросе к API адресов.")
		bot.Send(msg)
		metrics.SubscriptionCounter.WithLabelValues("failure").Inc()
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		msg := tgbotapi.NewMessage(message.Chat.ID, "Такой адрес не найден.")
		bot.Send(msg)
		metrics.SubscriptionCounter.WithLabelValues("failure").Inc()
		return
	}

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		msg := tgbotapi.NewMessage(message.Chat.ID, "Ошибка при чтении ответа от API адресов.")
		bot.Send(msg)
		metrics.SubscriptionCounter.WithLabelValues("failure").Inc()
		return
	}

	var addresses []map[string]interface{}
	err = json.Unmarshal(body, &addresses)
	if err != nil {
		msg := tgbotapi.NewMessage(message.Chat.ID, "Ошибка при разборе ответа от API адресов.")
		bot.Send(msg)
		metrics.SubscriptionCounter.WithLabelValues("failure").Inc()
		return
	}

	if len(addresses) == 0 {
		msg := tgbotapi.NewMessage(message.Chat.ID, "По указанному запросу не найдено ни одного адреса.")
		bot.Send(msg)
		metrics.SubscriptionCounter.WithLabelValues("failure").Inc()
		return
	}

	if len(addresses) == 1 {
		subscribeToAddress(cfg, bot, message, addresses[0])
	} else {
		msg := tgbotapi.NewMessage(message.Chat.ID, "Найдено несколько адресов, пожалуйста, выберите один:")
		msg.ReplyMarkup = buildAddressButtons(addresses)
		bot.Send(msg)
	}
}

func subscribeToAddress(cfg config.Config, bot *tgbotapi.BotAPI, message *tgbotapi.Message, address map[string]interface{}) {
	kladr := address["kladr"].(string)
	fullAddress := address["full_address"].(string)

	conn, err := pgx.Connect(context.Background(), cfg.PostgresConnStr)
	if err != nil {
		log.Printf("Ошибка подключения к PostgreSQL: %v", err)
		metrics.SubscriptionCounter.WithLabelValues("failure").Inc()
		return
	}
	defer conn.Close(context.Background())

	query := `INSERT INTO subscriptions (id, created_at, subscribe_to_kladr, subscribe_to_fulltext, tg_id) VALUES ($1, $2, $3, $4, $5)`
	_, err = conn.Exec(context.Background(), query, uuid.New(), time.Now(), kladr, fullAddress, strconv.FormatInt(message.Chat.ID, 10))
	if err != nil {
		log.Printf("Ошибка вставки подписки: %v", err)
		metrics.SubscriptionCounter.WithLabelValues("failure").Inc()
		return
	}

	msg := tgbotapi.NewMessage(message.Chat.ID, fmt.Sprintf("Подписка на %s успешно оформлена.", fullAddress))
	bot.Send(msg)
	metrics.SubscriptionCounter.WithLabelValues("success").Inc()
}

func buildAddressButtons(addresses []map[string]interface{}) tgbotapi.InlineKeyboardMarkup {
	var rows [][]tgbotapi.InlineKeyboardButton
	for _, address := range addresses {
		kladr := address["kladr"].(string)
		btn := tgbotapi.NewInlineKeyboardButtonData(address["full_address"].(string), "s:"+kladr)
		row := tgbotapi.NewInlineKeyboardRow(btn)
		rows = append(rows, row)
	}
	return tgbotapi.NewInlineKeyboardMarkup(rows...)
}

func handleList(cfg config.Config, bot *tgbotapi.BotAPI, message *tgbotapi.Message) {
	conn, err := pgx.Connect(context.Background(), cfg.PostgresConnStr)
	if err != nil {
		log.Printf("Ошибка подключения к PostgreSQL: %v", err)
		return
	}
	defer conn.Close(context.Background())

	query := `SELECT subscribe_to_fulltext FROM subscriptions WHERE tg_id = $1`
	rows, err := conn.Query(context.Background(), query, strconv.FormatInt(message.Chat.ID, 10))
	if err != nil {
		log.Printf("Ошибка запроса подписок: %v", err)
		return
	}
	defer rows.Close()

	var subscriptions []string
	for rows.Next() {
		var fulltext string
		err := rows.Scan(&fulltext)
		if err != nil {
			log.Printf("Ошибка сканирования строки: %v", err)
			continue
		}
		subscriptions = append(subscriptions, fulltext)
	}

	if len(subscriptions) == 0 {
		msg := tgbotapi.NewMessage(message.Chat.ID, "У вас нет подписок.")
		bot.Send(msg)
	} else {
		msg := tgbotapi.NewMessage(message.Chat.ID, "Ваши подписки:\n"+strings.Join(subscriptions, "\n"))
		bot.Send(msg)
	}
}

func handleDelete(cfg config.Config, bot *tgbotapi.BotAPI, message *tgbotapi.Message) {
	conn, err := pgx.Connect(context.Background(), cfg.PostgresConnStr)
	if err != nil {
		log.Printf("Ошибка подключения к PostgreSQL: %v", err)
		return
	}
	defer conn.Close(context.Background())

	query := `SELECT id, subscribe_to_fulltext FROM subscriptions WHERE tg_id = $1`
	rows, err := conn.Query(context.Background(), query, strconv.FormatInt(message.Chat.ID, 10))
	if err != nil {
		log.Printf("Ошибка запроса подписок: %v", err)
		return
	}
	defer rows.Close()

	var rowsOfButtons [][]tgbotapi.InlineKeyboardButton
	for rows.Next() {
		var id, fulltext string
		err := rows.Scan(&id, &fulltext)
		if err != nil {
			log.Printf("Ошибка сканирования строки: %v", err)
			continue
		}
		button := tgbotapi.NewInlineKeyboardButtonData(fulltext, "d:"+id)
		row := tgbotapi.NewInlineKeyboardRow(button)
		rowsOfButtons = append(rowsOfButtons, row)
	}

	if len(rowsOfButtons) == 0 {
		msg := tgbotapi.NewMessage(message.Chat.ID, "У вас нет подписок для удаления.")
		bot.Send(msg)
	} else {
		msg := tgbotapi.NewMessage(message.Chat.ID, "Выберите подписку для удаления:")
		msg.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(rowsOfButtons...)
		bot.Send(msg)
	}
}
