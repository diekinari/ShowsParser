// Package main содержит API-слой для получения и форматирования информации о спектаклях.
//
// Этот файл реализует:
// - FetchAllShows() - параллельный парсинг всех URL из конфигурации
// - RenderShowMarkdown() и RenderShowsMarkdown() - форматирование спектаклей в Markdown для Telegram
// - escapeMarkdown() - экранирование специальных символов MarkdownV2
//
// Взаимодействует с:
// - parser.go: использует loadConfig() для загрузки конфигурации и parsePages() для парсинга страниц
// - insight.go: использует GetAvailableShows() для получения доступных спектаклей из API
package main

import (
	"context"
	"fmt"
	"strings"
	"sync"
)

// FetchAllShows loads config and returns parsed shows for all URLs
func FetchAllShows(ctx context.Context) ([]Show, error) {
	cfg, err := loadConfig("config.json")
	if err != nil {
		return nil, err
	}
	available, err := GetAvailableShows()
	if err != nil {
		return nil, err
	}
	wg := &sync.WaitGroup{}
	wg.Add(len(cfg.URLs))
	out := make(chan Show, len(cfg.URLs))

	for _, url := range cfg.URLs {
		go func(url string) {
			defer wg.Done()
			show := parsePages(ctx, url, available)
			out <- show
		}(url)
	}

	go func() {
		wg.Wait()
		close(out)
	}()

	var shows []Show
	for sh := range out {
		shows = append(shows, sh)
	}
	return shows, nil
}

// RenderShowMarkdown formats a single show for Telegram Markdown
func RenderShowMarkdown(show Show) string {
	var b strings.Builder
	// Title
	b.WriteString(fmt.Sprintf("*%s*\n", escapeMarkdown(show.Title)))
	// Sessions
	for _, inf := range show.Info {
		status := "❌ нет"
		if inf.CanBuy {
			status = "✅ да"
		}
		// Date line: 02 Mon 15:04 style localized we already have Date/Weekday/Time
		b.WriteString(fmt.Sprintf("• %s, %s, %s — %s\n",
			escapeMarkdown(inf.Date),
			escapeMarkdown(inf.Weekday),
			escapeMarkdown(inf.Time),
			status,
		))
		if inf.CanBuy && inf.BuyLink != "" {
			b.WriteString(fmt.Sprintf("  → [Купить билет](%s)\n", inf.BuyLink))
		}
	}
	return b.String()
}

// RenderShowsMarkdown formats multiple shows into a single Telegram message
func RenderShowsMarkdown(shows []Show) string {
	var b strings.Builder
	for i, sh := range shows {
		if i > 0 {
			b.WriteString("\n")
		}
		b.WriteString(RenderShowMarkdown(sh))
	}
	return b.String()
}

// escapeMarkdown escapes Telegram MarkdownV2-sensitive characters minimally.
// Here we target basic Markdown (not V2) symbols used: _, *, [, ], (, ), ~, `, >, #, +, -, =, |, {, }, ., !
func escapeMarkdown(s string) string {
	replacer := strings.NewReplacer(
		"_", "\\_",
		"*", "\\*",
		"[", "\\[",
		"]", "\\]",
		"(", "\\(",
		")", "\\)",
		"~", "\\~",
		"`", "\\`",
		">", "\\>",
		"#", "\\#",
		"+", "\\+",
		"-", "\\-",
		"=", "\\=",
		"|", "\\|",
		"{", "\\{",
		"}", "\\}",
		".", "\\.",
		"!", "\\!",
	)
	return replacer.Replace(strings.TrimSpace(s))
}
