package main

import (
	"context"
	"fmt"
	"github.com/PuerkitoBio/goquery"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

var urls = []string{"https://vakhtangov.ru/show/dead_souls/", "https://vakhtangov.ru/show/doctoevsky/", "https://vakhtangov.ru/show/_nash_klass/", "https://vakhtangov.ru/show/matrenindvor/"}

type ShowInfo struct {
	Date    string
	Weekday string
	Time    string
}
type Show struct {
	Title string
	Info  []ShowInfo
}

func logError(err error) {
	fmt.Printf("Erorr occured: %v\n", err)
}

func parsePages(ctx context.Context, url string) Show {
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
	doc.Find("ul.show-afisha > li").Each(func(i int, s *goquery.Selection) {
		dateText := strings.TrimSuffix(
			strings.TrimSpace(
				s.Find("span.date > span.date").Text()),
			",")
		weekday := strings.TrimSuffix(
			strings.TrimSpace(s.Find("span.date > span.weekday").Text()),
			",")
		timeText := strings.TrimSpace(s.Find("span.date > span.time").Text())

		// Если какой-то из элементов пустой — пропускаем
		if dateText == "" || weekday == "" || timeText == "" {
			return
		}

		showsInfo = append(showsInfo, ShowInfo{
			Date:    dateText,
			Weekday: weekday,
			Time:    timeText,
		})
	})
	// Выводим результат
	return Show{
		Title: title,
		Info:  showsInfo,
	}
}

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	wg := &sync.WaitGroup{}
	wg.Add(len(urls))

	resultChan := make(chan string)

	for _, url := range urls {
		go func(url string) {
			defer wg.Done()
			show := parsePages(ctx, url)
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
					strings.Repeat("─", frameWidth+2) + "\n" // +2 учитывает граничные символы

			for _, inf := range show.Info {
				result += fmt.Sprintf(sessionFormat,
					inf.Date,
					inf.Weekday,
					inf.Time,
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
			fmt.Println("5 SECOND DEADLINE EXCEEDED")
			return
		case result, ok := <-resultChan:
			if !ok {
				return
			}
			fmt.Println(result)
		}
	}

}
