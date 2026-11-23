package scraie

import (
	"encoding/json"
	"fmt"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/maxmwang/scraie/flights/internal/db"
	"github.com/maxmwang/scraie/flights/internal/util"
)

// timestampMinimumPriceSeries returns two series, the first mapping onto the
// second (ie. X and Y-axis values). The Xs are dates in ascending order. The
// Ys are the minimum observed search.FlightOptions price on each timetsamp.
// Each X value is anchored to the start of that timetsamp in the observation's
// own location.
func timestampMinimumPriceSeries(history []db.Option) ([]time.Time, []float64) {
	minByTimestamp := make(map[time.Time]float64)
	for _, o := range history {
		if !o.SearchedAt.Valid {
			continue
		}
		t := o.SearchedAt.Time
		y, m, d := t.Date()
		h, _, _ := t.Clock()
		timestamp := time.Date(y, m, d, h, 0, 0, 0, t.Location())
		price := priceFloat(o.Price)
		if cur, ok := minByTimestamp[timestamp]; !ok || price < cur {
			minByTimestamp[timestamp] = price
		}
	}

	timestamps := make([]time.Time, 0, len(minByTimestamp))
	for timestamp := range minByTimestamp {
		timestamps = append(timestamps, timestamp)
	}
	sort.Slice(timestamps, func(a, b int) bool { return timestamps[a].Before(timestamps[b]) })

	xs := make([]time.Time, len(timestamps))
	ys := make([]float64, len(timestamps))
	for i, timestamp := range timestamps {
		xs[i] = timestamp
		ys[i] = minByTimestamp[timestamp]
	}
	return xs, ys
}

// TODO: download image instead of using link because link will exceed character limit

// buildDailyMinimumPriceChartURL renders the recent daily-minimum price history
// as a QuickChart (https://quickchart.io) line chart and returns its image URL,
// suitable for use as an image. It returns "" (without error) when
// there is no data to plot.
func BuildDailyMinimumPriceChartURL(it db.Itinerary, history []db.Option, nDaysToChart int) (string, error) {
	xs, ys := timestampMinimumPriceSeries(history)
	if len(xs) == 0 {
		return "", nil
	}
	if len(xs) >= nDaysToChart {
		xs = xs[len(xs)-nDaysToChart:]
		ys = ys[len(ys)-nDaysToChart:]
	}

	labels := make([]string, len(xs))
	for i, t := range xs {
		labels[i] = t.Format(time.Stamp)
	}

	config := map[string]any{
		"type": "line",
		"data": map[string]any{
			"labels": labels,
			"datasets": []map[string]any{{
				"label":                "Daily min (" + currencySymbol(it.Currency) + ")",
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
				"text":    util.ItineraryToString(it),
			},
			"scales": map[string]any{
				"yAxes": []map[string]any{{
					"ticks": map[string]any{"beginAtZero": false},
				}},
				"xAxes": []map[string]any{{"type": "time"}},
			},
		},
	}

	c, err := json.Marshal(config)
	if err != nil {
		return "", fmt.Errorf("buildDailyMinimumPriceChartURL: %w", err)
	}
	return "https://quickchart.io/chart?w=900&h=400&bkg=white&c=" + url.QueryEscape(string(c)), nil
}

func currencySymbol(code string) string {
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
