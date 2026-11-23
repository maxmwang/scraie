package util

import (
	"encoding/json"
	"fmt"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/maxmwang/scraie/flights/internal/db"
)

// TimestampMinimumPriceSeries returns two series, the first mapping onto the
// second (X and Y-axis values). X values are observation timestamps in
// ascending order; Y values are the minimum price at each timestamp.
func TimestampMinimumPriceSeries(history []db.Option) ([]time.Time, []float64) {
	minByTimestamp := make(map[time.Time]float64)
	for _, option := range history {
		if !option.SearchedAt.Valid {
			continue
		}
		t := option.SearchedAt.Time
		y, m, d := t.Date()
		h, _, _ := t.Clock()
		timestamp := time.Date(y, m, d, h, 0, 0, 0, t.Location())
		price := numericFloat64(option.Price)
		if current, ok := minByTimestamp[timestamp]; !ok || price < current {
			minByTimestamp[timestamp] = price
		}
	}

	timestamps := make([]time.Time, 0, len(minByTimestamp))
	for timestamp := range minByTimestamp {
		timestamps = append(timestamps, timestamp)
	}
	sort.Slice(timestamps, func(a, b int) bool {
		return timestamps[a].Before(timestamps[b])
	})

	xs := make([]time.Time, len(timestamps))
	ys := make([]float64, len(timestamps))
	for i, timestamp := range timestamps {
		xs[i] = timestamp
		ys[i] = minByTimestamp[timestamp]
	}
	return xs, ys
}

// BuildDailyMinimumPriceChartURL renders recent daily-minimum price history
// as a QuickChart line chart and returns a URL suitable for an embed image.
func BuildDailyMinimumPriceChartURL(it db.Itinerary, history []db.Option, nDaysToChart int) (string, error) {
	xs, ys := TimestampMinimumPriceSeries(history)
	if len(xs) == 0 {
		return "", nil
	}
	if len(xs) >= nDaysToChart {
		xs = xs[len(xs)-nDaysToChart:]
		ys = ys[len(ys)-nDaysToChart:]
	}

	labels := make([]string, len(xs))
	for i, timestamp := range xs {
		labels[i] = timestamp.Format(time.Stamp)
	}

	config := map[string]any{
		"type": "line",
		"data": map[string]any{
			"labels": labels,
			"datasets": []map[string]any{{
				"label":                "Daily min (" + CurrencySymbol(it.Currency) + ")",
				"data":                 ys,
				"fill":                 false,
				"borderColor":          "#3498DB",
				"borderWidth":          2,
				"pointRadius":          3,
				"pointBackgroundColor": "#3498DB",
				"steppedLine":          true,
			}},
		},
		"options": map[string]any{
			"legend": map[string]any{"display": false},
			"title": map[string]any{
				"display": true,
				"text":    ItineraryToString(it),
			},
			"scales": map[string]any{
				"yAxes": []map[string]any{{
					"ticks": map[string]any{"beginAtZero": false},
				}},
				"xAxes": []map[string]any{{"type": "time"}},
			},
		},
	}

	encoded, err := json.Marshal(config)
	if err != nil {
		return "", fmt.Errorf("build daily minimum price chart URL: %w", err)
	}
	return "https://quickchart.io/chart?w=900&h=400&bkg=white&c=" + url.QueryEscape(string(encoded)), nil
}

func CurrencySymbol(code string) string {
	switch strings.ToUpper(code) {
	case "", "USD":
		return "$"
	case "EUR":
		return "€"
	case "GBP":
		return "£"
	case "JPY":
		return "¥"
	default:
		return code
	}
}

func numericFloat64(value pgtype.Numeric) float64 {
	result, err := value.Float64Value()
	if err != nil || !result.Valid {
		return 0
	}
	return result.Float64
}
