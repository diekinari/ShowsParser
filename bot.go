// Package main содержит реализацию Telegram-бота для получения информации о спектаклях.
//
// Этот файл реализует:
// - Long polling для получения обновлений от Telegram API
// - Обработку команд /start, /shows, /afisha, /help
// - Отправку форматированных сообщений с информацией о спектаклях
//
// Взаимодействует с:
// - api.go: использует FetchAllShows() для получения списка спектаклей и RenderShowsMarkdown() для форматирования
package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
)

func RunTelegramBot(ctx context.Context) error {
	// if err := godotenv.Load(); err != nil {
	// 	return fmt.Errorf(".env file not found or error loading: %w", err)
	// }
	// log.Println("dotenv loaded")
	token := os.Getenv("TELEGRAM_BOT_TOKEN")
	if token == "" {
		return fmt.Errorf("TELEGRAM_BOT_TOKEN is not set")
	}
	log.Println("token:", token)

	
	opts := []bot.Option{
		bot.WithDefaultHandler(defaultHandler),
		bot.WithCallbackQueryDataHandler("afisha", bot.MatchTypePrefix, callbackHandler),
	}

	b, err := bot.New(token, opts...)
	if err != nil {
		return fmt.Errorf("failed to create bot: %w", err)
	}
	log.Println("bot created")
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
		log.Println("CallbackQuery is nil")
		return
	}
	
	b.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{
		CallbackQueryID: update.CallbackQuery.ID,
		ShowAlert:       false,
	})


	chatID := update.CallbackQuery.From.ID

	var msg string
	switch update.CallbackQuery.Data {
	case "afisha_theatre_vakhtangov":
		msg = "*Афиша театра Вахтангова:*\n\n" + buildShowsMessage(ctx)
	case "afisha_ballet":
		msg = "*Афиша балета:*\n\n" + buildBaletMessage(ctx)
	}
	b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID: chatID,
		Text:   msg,
		ParseMode: models.ParseModeMarkdownV1,
	})
}

func defaultHandler(ctx context.Context, b *bot.Bot, update *models.Update) {
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
	})
}