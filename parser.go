// Package main содержит основной парсер спектаклей театра Вахтангова.
//
// Этот файл реализует:
// - Загрузку конфигурации из config.json
// - Парсинг HTML-страниц спектаклей с извлечением дат, времени и информации о билетах
// - Форматирование вывода результатов в консоль
// - Функцию pmain() которая может работать как бот или как обычный парсер
//
// Взаимодействует с:
// - insight.go: использует GetAvailableShows() для получения списка доступных спектаклей из API
// - bot.go: вызывает RunTelegramBot() при запуске в режиме бота (через переменную окружения RUN_BOT)
package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/joho/godotenv"
)

const TIMEOUT = 5

type Config struct {
	URLs []string `json:"urls"`
}

type ShowInfo struct {
	Date    string
	Weekday string
	Time    string
	CanBuy  bool
}
type Show struct {
	Title string
	Info  []ShowInfo
}

func loadConfig(path string) (*Config, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func(f *os.File) {
		err := f.Close()
		if err != nil {
			logError(err)
		}
	}(f)
	var cfg Config
	if err = json.NewDecoder(f).Decode(&cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

func logError(err error) {
	fmt.Printf("Erorr occured: %v\n", err)
}

func parsePages(ctx context.Context, url string, availableShows []ShowEntry) Show {
	fmt.Printf("Parsing %s\n", url)
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		logError(err)
		return Show{
			Title: "Error"}
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		logError(err)
		return Show{
			Title: "Error"}
	}
	defer func(Body io.ReadCloser) {
		err := Body.Close()
		if err != nil {
			logError(err)
		}
	}(resp.Body)

	doc, err := goquery.NewDocumentFromReader(resp.Body)
	if err != nil {
		logError(err)
		return Show{
			Title: "Error"}
	}

	var showsInfo []ShowInfo

	title := strings.TrimSpace(doc.Find("header.cover-header h1").Text())

	// Проходим по каждому <li> внутри .show-afisha
	// doc.Find("ul.show-afisha > li").Each(func(i int, s *goquery.Selection) {
	// 	dateText := strings.TrimSuffix(
	// 		strings.TrimSpace(
	// 			s.Find("span.date > span.date").Text()),
	// 		",")
	// 	weekday := strings.TrimSuffix(
	// 		strings.TrimSpace(s.Find("span.date > span.weekday").Text()),
	// 		",")
	// 	timeText := strings.TrimSpace(s.Find("span.date > span.time").Text())

	// 	// Если какой-то из элементов пустой — пропускаем
	// 	if dateText == "" || weekday == "" || timeText == "" {
	// 		return
	// 	}

	// 	fmt.Printf("[debug] date=%s\n", dateText)

	// 	canBuy := false
	// 	for _, show := range availableShows {
	// 		// Быстрое логирование совпадений по заголовку
	// 		if show.Detail.Title == title || strings.ReplaceAll(show.Detail.Title, "е", "ё") == title {
	// 			// Совпадение по дате: допускаем формы "D month" и "D month YYYY"
	// 			if stringifyDate(show.Start) == dateText || stringifyDateWithYear(show.Start) == dateText {
	// 				fmt.Printf("[debug] found available show at %s (title: %s)\n", stringifyDateWithYear(show.Start), title)
	// 				canBuy = show.Detail.HasTickets || show.Detail.SalesOn
	// 				break
	// 			}
	// 		}
	// 	}

	// 	showsInfo = append(showsInfo, ShowInfo{
	// 		Date:    dateText,
	// 		Weekday: weekday,
	// 		Time:    timeText,
	// 		CanBuy:  canBuy,
	// 	})
	// })
	for _, show := range availableShows {
		if show.Detail.Title == title || strings.ReplaceAll(show.Detail.Title, "е", "ё") == title {
			showsInfo = append(showsInfo, ShowInfo{
				Date:    stringifyDateWithYear(show.Start),
				Weekday: weekdayRu(show.Start.Weekday()),
				Time:    show.Start.Format("15:04"),
				CanBuy:  show.Detail.HasTickets || show.Detail.SalesOn,
			})
		}
	}
	// Выводим результат
	return Show{
		Title: title,
		Info:  showsInfo,
	}
}

func stringifyDate(date time.Time) string {
	d := map[time.Month]string{
		time.January:   "января",
		time.February:  "февраля",
		time.March:     "марта",
		time.April:     "апреля",
		time.May:       "мая",
		time.June:      "июня",
		time.July:      "июля",
		time.August:    "августа",
		time.September: "сентября",
		time.October:   "октября",
		time.November:  "ноября",
		time.December:  "декабря",
	}
	return fmt.Sprintf("%d %s", date.Day(), d[date.Month()])
}

// stringifyDateWithYear returns "D month YYYY"
func stringifyDateWithYear(date time.Time) string {
	d := map[time.Month]string{
		time.January:   "января",
		time.February:  "февраля",
		time.March:     "марта",
		time.April:     "апреля",
		time.May:       "мая",
		time.June:      "июня",
		time.July:      "июля",
		time.August:    "августа",
		time.September: "сентября",
		time.October:   "октября",
		time.November:  "ноября",
		time.December:  "декабря",
	}
	return fmt.Sprintf("%d %s %d", date.Day(), d[date.Month()], date.Year())
}

// weekdayRu converts time.Weekday to Russian name used on the site
func weekdayRu(w time.Weekday) string {
	switch w {
	case time.Monday:
		return "Понедельник"
	case time.Tuesday:
		return "Вторник"
	case time.Wednesday:
		return "Среда"
	case time.Thursday:
		return "Четверг"
	case time.Friday:
		return "Пятница"
	case time.Saturday:
		return "Суббота"
	case time.Sunday:
		return "Воскресенье"
	default:
		return ""
	}
}

func main() {
	// Загружаем переменные окружения из .env файла
	// Игнорируем ошибку, если файл не найден (переменные могут быть установлены другим способом)
	if err := godotenv.Load(); err != nil {
		log.Printf("Warning: .env file not found or error loading: %v", err)
	}

	// Если запущено как бот
	if os.Getenv("RUN_BOT") == "1" && os.Getenv("TELEGRAM_BOT_TOKEN") != "" {
		fmt.Println("Running as bot")
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		if err := RunTelegramBot(ctx); err != nil {
			logError(err)
		}
		return
	}

	cfg, err := loadConfig("config.json")
	if err != nil {
		logError(errors.Join(errors.New("failed to load config:\t"), err))
		return
	}

	availableShows := GetAvailableShows()

	ctx, cancel := context.WithTimeout(context.Background(), TIMEOUT*time.Second)
	defer cancel()

	wg := &sync.WaitGroup{}
	wg.Add(len(cfg.URLs))

	resultChan := make(chan string)

	for _, url := range cfg.URLs {
		go func(url string) {
			defer wg.Done()
			show := parsePages(ctx, url, availableShows)
			// Ширина рамки (не включая символы границ)
			frameWidth := 50

			// Верхняя рамка
			topBorder := fmt.Sprintf("┌%s┐\n", strings.Repeat("─", frameWidth))

			// Строка с названием
			titleLine := fmt.Sprintf("│ Спектакль: %-"+strconv.Itoa(frameWidth-12)+"s│\n", show.Title)

			// Нижняя рамка
			bottomBorder := fmt.Sprintf("└%s┘\n", strings.Repeat("─", frameWidth))

			result := topBorder + titleLine + bottomBorder

			// Форматирование для сеансов
			sessionFormat :=
				"Дата:        %s\n" +
					"День недели: %s\n" +
					"Время:       %s\n" +
					"Билеты в продаже: %s\n" +
					strings.Repeat("─", frameWidth+2) + "\n" // +2 учитывает граничные символы

			for _, inf := range show.Info {
				status := "нет"
				if inf.CanBuy {
					status = "да"
				}
				result += fmt.Sprintf(sessionFormat,
					inf.Date,
					inf.Weekday,
					inf.Time,
					status,
				)
			}
			resultChan <- result

		}(url)
	}

	go func() {
		wg.Wait()
		close(resultChan)
	}()
	for {
		select {
		case <-ctx.Done():
			fmt.Printf("%v SECOND DEADLINE EXCEEDED\n", TIMEOUT)
			return
		case result, ok := <-resultChan:
			if !ok {
				return
			}
			fmt.Println(result)
		}
	}

}
