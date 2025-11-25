// Package main содержит модуль для получения данных о спектаклях из API театра Вахтангова.
//
// Этот файл реализует:
// - GetAvailableShows() - загрузку и парсинг JSON данных с vakhtangov.ru/ticketland_afisha/data.json
// - Преобразование данных API в структуру ShowEntry с распарсенными датами и временем
// - Предоставляет централизованный источник данных о доступных спектаклях
//
// Взаимодействует с:
// - parser.go: используется функцией parsePages() для получения списка доступных спектаклей
// - api.go: используется функцией FetchAllShows() для получения данных о спектаклях
package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"
)

type Envelope struct {
	CreatedAt time.Time `json:"createdAt"`
	Data      string    `json:"data"`
}

type ShowDetail struct {
	Title       string `json:"title"`
	StartDate   string `json:"start_date"`
	Script      string `json:"script"`
	HasTickets  bool   `json:"has_tickets"`
	SalesOn     bool   `json:"sales_on"`
	RevealDT    string `json:"reveal_dt"`
	RevealDTStr string `json:"reveal_dt_str"`
	Now         string `json:"now"`
}

type Data map[string]map[string]ShowDetail

// ShowEntry holds one show with its date time parsed
type ShowEntry struct {
	StageUID    string
	DateTimeKey string
	Start       time.Time
	Detail      ShowDetail
}

func GetAvailableShows() ([]ShowEntry, error) {
	url := "https://vakhtangov.ru/ticketland_afisha/data.json"

	resp, err := http.Get(url)
	if err != nil {
		panic(err)
	}
	defer resp.Body.Close()

	var env Envelope
	if err = json.NewDecoder(resp.Body).Decode(&env); err != nil {
		return nil, fmt.Errorf("insight.go: error unmarshalling data: %w", err)
	}
	fmt.Println("CreatedAt:", env.CreatedAt)

	var d Data
	if err := json.Unmarshal([]byte(env.Data), &d); err != nil {
		return nil, fmt.Errorf("insight.go: error unmarshalling data: %w", err)
	}

	// 1) collect all shows into one slice
	var all []ShowEntry
	for stageUID, shows := range d {
		for key, detail := range shows {
			// parse the key "YYYY-MM-DD-HH-MM-SS"
			t, err := time.Parse("2006-01-02-15-04-05", key)
			if err != nil {
				// fallback to parsing detail.StartDate if needed
				t, err = time.Parse(time.RFC3339, detail.StartDate)
				if err != nil {
					log.Fatalf("cannot parse date %q: %v", key, err)
				}
			}
			all = append(all, ShowEntry{
				StageUID:    stageUID,
				DateTimeKey: key,
				Start:       t,
				Detail:      detail,
			})
		}
	}

	return all, nil
}

// insight.go collects the data from ticketland_afisha json, sorts it and prints in the standard output
//func main() {
//	all := GetAvailableShows()
//
//	// 2) sort the slice by Start
//	sort.Slice(all, func(i, j int) bool {
//		return all[i].Start.Before(all[j].Start)
//	})
//
//	// 3) iterate the sorted slice and print only "Идиот"
//	for _, e := range all {
//		//if e.Detail.Title != "Идиот" {
//		//	continue
//		//}
//		fmt.Printf("Show at %s:\n", e.Start.Format("02-01-2006 15:04:05"))
//		fmt.Printf("    Title: %s\n", e.Detail.Title)
//		fmt.Printf("    Has tickets: %v, Sales on: %v\n",
//			e.Detail.HasTickets, e.Detail.SalesOn)
//		fmt.Printf("    Reveal at %s (display %s)\n",
//			e.Detail.RevealDT, e.Detail.RevealDTStr)
//		fmt.Printf("    Now: %s\n\n", e.Detail.Now)
//	}
//}
