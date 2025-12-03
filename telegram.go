// Package main содержит реализацию Telegram-бота для получения информации о спектаклях.
//
// Этот файл реализует:
// - Long polling для получения обновлений от Telegram API
// - Обработку команд /start, /shows, /afisha, /help
// - Отправку форматированных сообщений с информацией о спектаклях
//
// Взаимодействует с:
// - vakhtangov_formatter.go: использует FetchAllShows() для получения списка спектаклей и RenderShowsMarkdown() для форматирования
package main

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"parser/logger"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
)

var allowedUsers = make(map[string]bool)
var log = logger.Get().Named("bot")

func RunTelegramBot(ctx context.Context) error {
	// if err := godotenv.Load(); err != nil {
	// 	return fmt.Errorf(".env file not found or error loading: %w", err)
	// }
	// log.Println("dotenv loaded")
	token := os.Getenv("TELEGRAM_BOT_TOKEN")
	if token == "" {
		return fmt.Errorf("TELEGRAM_BOT_TOKEN is not set")
	}
	log.Infof("token: %s", token)

	users := os.Getenv("ALLOWED_USERS")
	if users != "" {
		for _, u := range strings.Split(users, ",") {
			trimmed := strings.TrimSpace(strings.ToLower(u))
			if trimmed != "" {
				allowedUsers[trimmed] = true
			}
		}
	}

	opts := []bot.Option{
		bot.WithDefaultHandler(defaultHandler),
		bot.WithCallbackQueryDataHandler("afisha", bot.MatchTypePrefix, callbackHandler),
	}

	b, err := bot.New(token, opts...)
	if err != nil {
		return fmt.Errorf("failed to create bot: %w", err)
	}
	log.Info("bot created")
	b.Start(ctx)

	return nil
}

func buildShowsMessage(ctx context.Context) string {
	shows, err := FetchAllShows(ctx)
	if err != nil {
		return "Ошибка загрузки афиши. Попробуйте позже."
	}
	markdown := RenderShowsMarkdown(shows)
	// Ограничение Telegram ~4096 символов; если больше — обрезаем
	if len(markdown) > 3800 {
		return markdown[:3800] + "\n…"
	}
	return markdown
}

func buildBaletMessage(ctx context.Context) string {
	shows, err := RunBaletParser()
	if err != nil {
		return "Ошибка загрузки афиши балета. Попробуйте позже."
	}
	markdown := RenderBaletShowsMarkdown(shows)
	return markdown
}

func callbackHandler(ctx context.Context, b *bot.Bot, update *models.Update) {
	if update.CallbackQuery == nil {
		log.Warn("CallbackQuery is nil")
		return
	}

	username := strings.ToLower(update.CallbackQuery.From.Username)
	if len(allowedUsers) > 0 && !allowedUsers[username] {
		b.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{
			CallbackQueryID: update.CallbackQuery.ID,
			Text:            "⛔️ Доступ запрещен / Access denied",
			ShowAlert:       true,
		})
		return
	}

	b.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{
		CallbackQueryID: update.CallbackQuery.ID,
		ShowAlert:       false,
	})

	var kb *models.InlineKeyboardMarkup
	chatID := update.CallbackQuery.From.ID
	isDisabled := true

	var msg string
	var currentAction string

	switch update.CallbackQuery.Data {
	case "afisha_theatre_vakhtangov":
		msg = "*Афиша театра Вахтангова:*\n\n" + buildShowsMessage(ctx)
		currentAction = "afisha_theatre_vakhtangov"
	case "afisha_ballet":
		msg = "*Афиша балета:*\n\n" + buildBaletMessage(ctx)
		currentAction = "afisha_ballet"
	case "afisha_update":
		// Если пришел общий update, показываем меню
		msg = "Выберите афишу:"
		// Клавиатура будет перезаписана ниже, если currentAction пустой
	}

	if currentAction != "" {
		// Добавляем время обновления в конец сообщения, чтобы текст менялся
		// Это предотвращает ошибку "message is not modified" если данные не изменились
		// Используем фиксированную зону MSK (UTC+3), так как на сервере может быть UTC
		mskZone := time.FixedZone("MSK", 3*60*60)
		msg += fmt.Sprintf("\n\n_Обновлено: %s_", time.Now().In(mskZone).Format("15:04:05"))

		kb = &models.InlineKeyboardMarkup{
			InlineKeyboard: [][]models.InlineKeyboardButton{
				{
					{Text: "🔄 Обновить", CallbackData: currentAction},
				},
				{
					{Text: "⬅️ Назад", CallbackData: "afisha_update"},
				},
			},
		}
	} else if update.CallbackQuery.Data == "afisha_update" {
		kb = &models.InlineKeyboardMarkup{
			InlineKeyboard: [][]models.InlineKeyboardButton{
				{
					{Text: "Театр Вахтангова", CallbackData: "afisha_theatre_vakhtangov"},
					{Text: "Балет", CallbackData: "afisha_ballet"},
				},
			},
		}
	}

	// Редактируем сообщение вместо отправки нового
	// Нужно получить MessageID из CallbackQuery
	if update.CallbackQuery.Message.Message != nil {
		_, err := b.EditMessageText(ctx, &bot.EditMessageTextParams{
			ChatID:      chatID,
			MessageID:   update.CallbackQuery.Message.Message.ID,
			Text:        msg,
			ParseMode:   models.ParseModeMarkdown,
			ReplyMarkup: kb,
			LinkPreviewOptions: &models.LinkPreviewOptions{
				IsDisabled: &isDisabled,
			},
		})
		if err != nil {
			log.Errorf("Error editing message: %v", err)
			// Если не удалось отредактировать (например, сообщение слишком старое), отправляем новое
			b.SendMessage(ctx, &bot.SendMessageParams{
				ChatID:      chatID,
				Text:        msg,
				ParseMode:   models.ParseModeMarkdown,
				ReplyMarkup: kb,
				LinkPreviewOptions: &models.LinkPreviewOptions{
					IsDisabled: &isDisabled,
				},
			})
		}
	} else {
		// Если сообщение недоступно, отправляем новое
		b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID:      chatID,
			Text:        msg,
			ParseMode:   models.ParseModeMarkdown,
			ReplyMarkup: kb,
			LinkPreviewOptions: &models.LinkPreviewOptions{
				IsDisabled: &isDisabled,
			},
		})
	}
}

func defaultHandler(ctx context.Context, b *bot.Bot, update *models.Update) {
	log.Infof("Allowed users: %v", allowedUsers)
	if update.Message == nil {
		log.Warn("Message is nil")
		return
	}

	if update.Message.From == nil {
		return
	}

	username := strings.ToLower(update.Message.From.Username)
	if len(allowedUsers) > 0 && !allowedUsers[username] {
		b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: update.Message.Chat.ID,
			Text:   "⛔️ Доступ запрещен. Бот работает только для авторизованных пользователей.",
		})
		return
	}

	isDisabled := true
	kb := &models.InlineKeyboardMarkup{
		InlineKeyboard: [][]models.InlineKeyboardButton{
			{
				{Text: "Театр Вахтангова", CallbackData: "afisha_theatre_vakhtangov"},
				{Text: "Балет", CallbackData: "afisha_ballet"},
			},
		},
	}

	b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID:      update.Message.Chat.ID,
		Text:        "Посмотреть афишу в:",
		ReplyMarkup: kb,
		LinkPreviewOptions: &models.LinkPreviewOptions{
			IsDisabled: &isDisabled,
		},
	})
}
