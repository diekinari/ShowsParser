// Package main содержит парсер для страниц балетных спектаклей.
//
// Этот файл реализует:
// - Парсинг HTML-страниц балетных спектаклей с использованием goquery
// - Извлечение информации о доступности билетов и опциях покупки
// - Фильтрацию и дедупликацию опций покупки
// - Standalone режим работы (имеет собственную функцию main)
//
// Взаимодействует с:
// - Не взаимодействует с другими файлами проекта (независимый модуль)
// - Использует конфигурационный файл balet_config.json
package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/PuerkitoBio/goquery"
)

const BALET_TIMEOUT = 10 * time.Second

type BaletConfig struct {
	URLs []string `json:"urls"`
}

type BaletShow struct {
	Title    string
	CanBuy   bool
	Sessions []BaletSession // Опции покупки (дата, время, место, ссылка)
}

type BaletSession struct {
	Info    string // Дата, время, место
	BuyLink string // Ссылка на покупку
}

func loadBaletConfig(path string) (*BaletConfig, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var cfg BaletConfig
	if err = json.NewDecoder(f).Decode(&cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

func parseBaletPage(ctx context.Context, url string) BaletShow {
	fmt.Printf("Парсинг страницы: %s\n", url)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		fmt.Printf("Ошибка создания запроса: %v\n", err)
		return BaletShow{Title: "Ошибка", CanBuy: false}
	}

	// Устанавливаем User-Agent для корректной работы с сайтом
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		fmt.Printf("Ошибка выполнения запроса: %v\n", err)
		return BaletShow{Title: "Ошибка", CanBuy: false}
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		fmt.Printf("Ошибка: статус код %d\n", resp.StatusCode)
		return BaletShow{Title: "Ошибка", CanBuy: false}
	}

	doc, err := goquery.NewDocumentFromReader(resp.Body)
	if err != nil {
		fmt.Printf("Ошибка парсинга HTML: %v\n", err)
		return BaletShow{Title: "Ошибка", CanBuy: false}
	}

	// Извлекаем название балета из заголовка h1
	title := strings.TrimSpace(doc.Find("h1").First().Text())
	if title == "" {
		// Пробуем альтернативный селектор
		title = strings.TrimSpace(doc.Find(".event-title, .title, header h1").First().Text())
	}

	// Проверяем наличие возможности купить билеты
	canBuy := false
	var sessions []BaletSession

	// Сначала ищем все блоки/элементы, которые могут содержать информацию о сеансах
	// Это могут быть таблицы, списки, карточки и т.д.

	// Ищем в таблицах (часто используется для расписания)
	doc.Find("table tr, .schedule-item, .event-item, .performance-item, .show-item").Each(func(i int, s *goquery.Selection) {
		rowText := strings.TrimSpace(s.Text())
		// Проверяем, есть ли в строке кнопка "купить билет"
		hasBuyButton := false
		var link string
		s.Find("a, button").Each(func(j int, btn *goquery.Selection) {
			btnText := strings.ToLower(strings.TrimSpace(btn.Text()))
			if strings.Contains(btnText, "купить билет") {
				hasBuyButton = true
				canBuy = true
				if href, ok := btn.Attr("href"); ok {
					link = resolveBaletBuyLink(url, href)
				}
			}
		})

		// Если есть кнопка покупки, извлекаем всю информацию из строки
		if hasBuyButton {
			// Ищем дату, время и место в этой строке
			extractSessionsFromText(rowText, link, &sessions)
		}
	})

	// Ищем все ссылки и кнопки с текстом "купить билет"
	doc.Find("a, button").Each(func(i int, s *goquery.Selection) {
		text := strings.ToLower(strings.TrimSpace(s.Text()))
		if strings.Contains(text, "купить билет") {
			canBuy = true
			var link string
			if href, ok := s.Attr("href"); ok {
				link = resolveBaletBuyLink(url, href)
			}

			// Ищем информацию в родительском контейнере и его родителях (до 3 уровней вверх)
			current := s
			for level := 0; level < 3; level++ {
				parent := current.Parent()
				if parent.Length() == 0 {
					break
				}
				parentText := strings.TrimSpace(parent.Text())
				extractSessionsFromText(parentText, link, &sessions)
				current = parent
			}

			// Проверяем предыдущие элементы (часто дата/время идут перед кнопкой)
			s.PrevAll().Each(func(i int, prev *goquery.Selection) {
				prevText := strings.TrimSpace(prev.Text())
				extractSessionsFromText(prevText, link, &sessions)
			})

			// Проверяем следующие элементы
			s.NextAll().Each(func(i int, next *goquery.Selection) {
				nextText := strings.TrimSpace(next.Text())
				extractSessionsFromText(nextText, link, &sessions)
			})

			// Проверяем соседние элементы (братья и сестры)
			s.Siblings().Each(func(i int, sibling *goquery.Selection) {
				siblingText := strings.TrimSpace(sibling.Text())
				extractSessionsFromText(siblingText, link, &sessions)
			})
		}
	})

	// Если не нашли через ссылки, ищем текст "купить билеты" на странице
	if !canBuy {
		pageText := strings.ToLower(doc.Text())
		if strings.Contains(pageText, "купить билет") {
			canBuy = true
		}
	}

	// Более агрессивный поиск: ищем все элементы с датой/временем и названием театра
	// Это важно для страниц репертуара, где информация может быть в разных местах
	doc.Find("div, span, p, li, td, .event, .performance, .show, .schedule, .afisha-item, .ticket-info").Each(func(i int, s *goquery.Selection) {
		text := strings.TrimSpace(s.Text())
		// Ищем элементы, которые содержат дату/время и название театра
		if strings.Contains(text, "/") && strings.Contains(text, ":") {
			// Проверяем наличие названия театра или места
			lowerText := strings.ToLower(text)
			if strings.Contains(lowerText, "мариинский") ||
				strings.Contains(lowerText, "театр") ||
				strings.Contains(lowerText, "сцена") ||
				strings.Contains(lowerText, "бдт") ||
				strings.Contains(lowerText, "дворец") ||
				strings.Contains(lowerText, "зал") ||
				strings.Contains(lowerText, "концерт") ||
				strings.Contains(lowerText, "филармония") {
				extractSessionsFromText(text, "", &sessions)
			}
		}
	})

	// Дополнительный поиск: ищем блоки, которые могут содержать расписание
	// Часто на страницах репертуара информация о билетах находится в специальных блоках
	doc.Find("[class*='ticket'], [class*='buy'], [class*='schedule'], [class*='date'], [class*='time']").Each(func(i int, s *goquery.Selection) {
		// Проверяем, есть ли рядом кнопка покупки
		hasBuyButton := false
		var link string
		s.Find("a, button").Each(func(j int, btn *goquery.Selection) {
			btnText := strings.ToLower(strings.TrimSpace(btn.Text()))
			if strings.Contains(btnText, "купить") || strings.Contains(btnText, "билет") {
				hasBuyButton = true
				canBuy = true
				if href, ok := btn.Attr("href"); ok {
					link = resolveBaletBuyLink(url, href)
				}
			}
		})

		if hasBuyButton {
			text := strings.TrimSpace(s.Text())
			extractSessionsFromText(text, link, &sessions)
		}
	})

	// Финальная фильтрация: удаляем дубликаты без театра, если есть версии с театром
	filteredSessions := filterDuplicateSessions(sessions)

	return BaletShow{
		Title:    title,
		CanBuy:   canBuy,
		Sessions: filteredSessions,
	}
}

// extractSessionsFromText извлекает информацию о билетах из текста и создает сессии
// Ищет строки с датой/временем и названием театра
func extractSessionsFromText(text string, link string, sessions *[]BaletSession) {
	// Разбиваем текст на строки и слова для более точного поиска
	lines := strings.Split(text, "\n")

	// Собираем информацию о театре из всего текста
	lowerText := strings.ToLower(text)
	theaterNames := []string{}
	if strings.Contains(lowerText, "мариинский") {
		// Извлекаем полное название театра
		theaterNames = append(theaterNames, extractTheaterName(text, "мариинский"))
	}
	if strings.Contains(lowerText, "бдт") {
		theaterNames = append(theaterNames, extractTheaterName(text, "бдт"))
	}
	if strings.Contains(lowerText, "театр") && !strings.Contains(lowerText, "мариинский") {
		theaterNames = append(theaterNames, extractTheaterName(text, "театр"))
	}

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		// Ищем строки с датой и временем
		// Паттерны: "DD/MM HH:MM", "DD.MM HH:MM", "DD/MM HH:MM:SS"
		hasDate := strings.Contains(line, "/") || strings.Contains(line, ".")
		hasTime := strings.Contains(line, ":")

		if hasDate && hasTime {
			// Проверяем, что это действительно дата/время
			if matchesDatePattern(line) {
				lowerLine := strings.ToLower(line)
				hasTheater := strings.Contains(lowerLine, "мариинский") ||
					strings.Contains(lowerLine, "театр") ||
					strings.Contains(lowerLine, "сцена") ||
					strings.Contains(lowerLine, "бдт") ||
					strings.Contains(lowerLine, "дворец") ||
					strings.Contains(lowerLine, "зал") ||
					strings.Contains(lowerLine, "концерт") ||
					strings.Contains(lowerLine, "филармония")

				// Если в строке есть театр - добавляем как есть
				if hasTheater {
					if !containsSession(*sessions, line) && len(line) > 10 && len(line) < 300 {
						*sessions = append(*sessions, BaletSession{Info: line, BuyLink: link})
					}
				} else if len(strings.Fields(line)) >= 2 && len(line) > 8 {
					// Если театра нет в строке, но есть в общем тексте - добавляем название театра
					// Сначала ищем название театра в соседних строках (в пределах 5 строк)
					foundTheater := false
					lineIdx := -1
					for i, l := range lines {
						if strings.TrimSpace(l) == line {
							lineIdx = i
							break
						}
					}

					if lineIdx >= 0 {
						// Проверяем строки рядом (в пределах 5 строк до и после)
						for i := max(0, lineIdx-5); i < min(len(lines), lineIdx+6); i++ {
							if i == lineIdx {
								continue
							}
							checkLine := strings.TrimSpace(lines[i])
							if checkLine == "" {
								continue
							}
							lowerCheck := strings.ToLower(checkLine)
							if strings.Contains(lowerCheck, "мариинский") ||
								strings.Contains(lowerCheck, "театр") ||
								strings.Contains(lowerCheck, "бдт") ||
								strings.Contains(lowerCheck, "сцена") ||
								strings.Contains(lowerCheck, "дворец") ||
								strings.Contains(lowerCheck, "зал") {
								// Объединяем информацию
								combinedLine := line
								if !strings.Contains(strings.ToLower(combinedLine), strings.ToLower(checkLine)) {
									combinedLine += " " + checkLine
								}
								if !containsSession(*sessions, combinedLine) && len(combinedLine) < 300 {
									*sessions = append(*sessions, BaletSession{Info: combinedLine, BuyLink: link})
									foundTheater = true
									break
								}
							}
						}
					}

					// Если не нашли в соседних строках, используем найденные названия театров
					if !foundTheater {
						for _, theaterName := range theaterNames {
							if theaterName != "" {
								combinedLine := line + " " + theaterName
								if !containsSession(*sessions, combinedLine) && len(combinedLine) < 300 {
									*sessions = append(*sessions, BaletSession{Info: combinedLine, BuyLink: link})
									foundTheater = true
									break
								}
							}
						}
					}

					// Если так и не нашли театр, проверяем, нет ли уже варианта с театром для этой даты/времени
					if !foundTheater {
						// Извлекаем дату/время из строки (первые части до пробела или двоеточия)
						dateTimePart := extractDateTimePart(line)
						// Проверяем, есть ли уже вариант с театром для этой даты/времени
						hasBetterVersion := false
						for _, existing := range *sessions {
							existingDateTime := extractDateTimePart(existing.Info)
							// Если дата/время совпадают и в существующем варианте есть театр
							if existingDateTime == dateTimePart {
								lowerExisting := strings.ToLower(existing.Info)
								if strings.Contains(lowerExisting, "мариинский") ||
									strings.Contains(lowerExisting, "театр") ||
									strings.Contains(lowerExisting, "бдт") ||
									strings.Contains(lowerExisting, "сцена") ||
									strings.Contains(lowerExisting, "дворец") ||
									strings.Contains(lowerExisting, "зал") {
									hasBetterVersion = true
									break
								}
							}
						}
						// Добавляем только если нет лучшего варианта с театром
						if !hasBetterVersion && !containsSession(*sessions, line) {
							*sessions = append(*sessions, BaletSession{Info: line, BuyLink: link})
						}
					}
				}
			}
		}
	}
}

// extractTheaterName извлекает название театра из текста
func extractTheaterName(text, keyword string) string {
	lowerText := strings.ToLower(text)
	keywordLower := strings.ToLower(keyword)

	// Ищем позицию ключевого слова
	idx := strings.Index(lowerText, keywordLower)
	if idx == -1 {
		return ""
	}

	// Извлекаем контекст вокруг ключевого слова (до 100 символов)
	start := idx - 50
	if start < 0 {
		start = 0
	}
	end := idx + 150
	if end > len(text) {
		end = len(text)
	}

	context := text[start:end]
	lines := strings.Split(context, "\n")

	// Ищем строку, содержащую название театра
	for _, line := range lines {
		lowerLine := strings.ToLower(line)
		if strings.Contains(lowerLine, keywordLower) {
			// Очищаем строку от лишних символов
			line = strings.TrimSpace(line)
			// Убираем лишние пробелы
			line = strings.Join(strings.Fields(line), " ")
			// Ограничиваем длину
			if len(line) > 100 {
				// Берем первые 100 символов
				words := strings.Fields(line)
				result := ""
				for _, word := range words {
					if len(result)+len(word)+1 <= 100 {
						if result != "" {
							result += " "
						}
						result += word
					} else {
						break
					}
				}
				return result
			}
			return line
		}
	}

	return ""
}

// filterDuplicateSessions удаляет дубликаты без театра, если есть версии с театром
func filterDuplicateSessions(sessions []BaletSession) []BaletSession {
	var filtered []BaletSession
	var seenDateTimeParts []string

	// Сначала добавляем все варианты с театром
	for _, session := range sessions {
		lowerOption := strings.ToLower(session.Info)
		hasTheater := strings.Contains(lowerOption, "мариинский") ||
			strings.Contains(lowerOption, "театр") ||
			strings.Contains(lowerOption, "бдт") ||
			strings.Contains(lowerOption, "сцена") ||
			strings.Contains(lowerOption, "дворец") ||
			strings.Contains(lowerOption, "зал") ||
			strings.Contains(lowerOption, "концерт") ||
			strings.Contains(lowerOption, "филармония")

		if hasTheater {
			filtered = append(filtered, session)
			dateTimePart := extractDateTimePart(session.Info)
			seenDateTimeParts = append(seenDateTimeParts, dateTimePart)
		}
	}

	// Затем добавляем варианты без театра, только если для них нет версии с театром
	for _, session := range sessions {
		lowerOption := strings.ToLower(session.Info)
		hasTheater := strings.Contains(lowerOption, "мариинский") ||
			strings.Contains(lowerOption, "театр") ||
			strings.Contains(lowerOption, "бдт") ||
			strings.Contains(lowerOption, "сцена") ||
			strings.Contains(lowerOption, "дворец") ||
			strings.Contains(lowerOption, "зал") ||
			strings.Contains(lowerOption, "концерт") ||
			strings.Contains(lowerOption, "филармония")

		if !hasTheater {
			dateTimePart := extractDateTimePart(session.Info)
			// Проверяем, есть ли уже версия с театром для этой даты/времени
			hasBetterVersion := false
			for _, seenPart := range seenDateTimeParts {
				if seenPart == dateTimePart {
					hasBetterVersion = true
					break
				}
			}
			// Добавляем только если нет лучшей версии
			if !hasBetterVersion && !containsSession(filtered, session.Info) {
				filtered = append(filtered, session)
			}
		}
	}

	return filtered
}

// extractDateTimePart извлекает часть с датой и временем из строки
// Например, из "30/11 12:00 Мариинский театр" вернет "30/11 12:00"
// Также обрабатывает случаи с множественными пробелами: "30/11    12:00" -> "30/11 12:00"
func extractDateTimePart(text string) string {
	// Нормализуем пробелы (заменяем множественные пробелы на одинарные)
	normalized := strings.Join(strings.Fields(text), " ")

	// Ищем паттерн даты/времени: "DD/MM HH:MM" или "DD.MM HH:MM"
	parts := strings.Fields(normalized)
	if len(parts) < 2 {
		return normalized
	}

	// Берем первые две части (дата и время)
	// Проверяем, что первая часть содержит "/" или ".", а вторая содержит ":"
	if (strings.Contains(parts[0], "/") || strings.Contains(parts[0], ".")) &&
		strings.Contains(parts[1], ":") {
		return parts[0] + " " + parts[1]
	}

	// Если формат другой, возвращаем первые две части
	if len(parts) >= 2 {
		return parts[0] + " " + parts[1]
	}

	return normalized
}

// matchesDatePattern проверяет, содержит ли строка паттерн даты
func matchesDatePattern(text string) bool {
	// Ищем паттерны: "DD/MM", "DD.MM", "D/MM", "DD/M"
	// Используем простую проверку на наличие цифр перед и после разделителя
	hasDigitBefore := false
	hasDigitAfter := false
	hasSeparator := false

	for i, r := range text {
		if r >= '0' && r <= '9' {
			if hasSeparator {
				hasDigitAfter = true
			} else {
				hasDigitBefore = true
			}
		} else if (r == '/' || r == '.') && hasDigitBefore && i < len(text)-1 {
			hasSeparator = true
		}
	}

	return hasDigitBefore && hasSeparator && hasDigitAfter
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func containsSession(slice []BaletSession, itemInfo string) bool {
	for _, s := range slice {
		if s.Info == itemInfo {
			return true
		}
	}
	return false
}

func resolveBaletBuyLink(pageURL, href string) string {
	href = strings.TrimSpace(href)
	if href == "" {
		return ""
	}

	parsedHref, err := url.Parse(href)
	if err != nil {
		return ""
	}
	if parsedHref.IsAbs() {
		return parsedHref.String()
	}

	base, err := url.Parse(pageURL)
	if err != nil {
		return parsedHref.String()
	}
	return base.ResolveReference(parsedHref).String()
}

func printBaletResult(show BaletShow) {
	fmt.Println(strings.Repeat("═", 60))
	fmt.Printf("Название балета: %s\n", show.Title)

	if show.CanBuy {
		fmt.Println("✅ Билеты доступны для покупки")
		if len(show.Sessions) > 0 {
			fmt.Println("\nОпции покупки:")
			for i, session := range show.Sessions {
				fmt.Printf("  %d. %s\n", i+1, session.Info)
				if session.BuyLink != "" {
					fmt.Printf("     Ссылка: %s\n", session.BuyLink)
				}
			}
		}
	} else {
		fmt.Println("❌ Билеты недоступны для покупки")
	}
	fmt.Println(strings.Repeat("═", 60))
	fmt.Println()
}

func RunBaletParser() ([]BaletShow, error) {
	// Пробуем загрузить конфиг из разных возможных файлов
	configFiles := []string{"balet_config.json"}
	var cfg *BaletConfig
	var err error

	for _, configFile := range configFiles {
		cfg, err = loadBaletConfig(configFile)
		if err == nil {
			fmt.Printf("Конфиг загружен из: %s\n", configFile)
			break
		}
	}

	if cfg == nil {
		return nil, errors.New("не удалось загрузить конфиг. Создайте файл balet_config.json с полем urls")
	}

	if len(cfg.URLs) == 0 {
		return nil, errors.New("в конфиге нет URL-адресов для парсинга")
	}

	ctx, cancel := context.WithTimeout(context.Background(), BALET_TIMEOUT*time.Duration(len(cfg.URLs)))
	defer cancel()

	var wg sync.WaitGroup
	results := make(chan BaletShow, len(cfg.URLs))

	for _, url := range cfg.URLs {
		wg.Add(1)
		go func(url string) {
			defer wg.Done()
			show := parseBaletPage(ctx, url)
			results <- show
		}(url)
	}

	go func() {
		wg.Wait()
		close(results)
	}()

	var shows []BaletShow

	for show := range results {
		shows = append(shows, show)
		// printBaletResult(show)
	}

	return shows, nil
}

// RenderBaletShowMarkdown formats a single ballet show for Telegram Markdown
func RenderBaletShowMarkdown(show BaletShow) string {
	var b strings.Builder
	// Title
	b.WriteString(fmt.Sprintf("*%s*\n", escapeBaletMarkdown(show.Title)))

	// Availability status
	status := "❌ Билеты недоступны"
	if show.CanBuy {
		status = "✅ Билеты доступны"
	}
	b.WriteString(fmt.Sprintf("%s\n", status))

	// Buy options
	if len(show.Sessions) > 0 {
		b.WriteString("\n*Опции покупки:*\n")
		for _, session := range show.Sessions {
			b.WriteString(fmt.Sprintf("• %s\n", escapeBaletMarkdown(session.Info)))
			if session.BuyLink != "" {
				b.WriteString(fmt.Sprintf("  → [Купить билет](%s)\n", session.BuyLink))
			}
		}
	}

	return b.String()
}

// RenderBaletShowsMarkdown formats multiple ballet shows into a single Telegram message
func RenderBaletShowsMarkdown(shows []BaletShow) string {
	var b strings.Builder
	for i, show := range shows {
		if i > 0 {
			b.WriteString("\n")
		}
		b.WriteString(RenderBaletShowMarkdown(show))
	}
	return b.String()
}

// escapeBaletMarkdown escapes Telegram MarkdownV2-sensitive characters
func escapeBaletMarkdown(s string) string {
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

// func main() {
// 	RunBaletParser()
// }
