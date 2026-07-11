package analyze

import (
	"encoding/json"
	"fmt"
	"net/url"
	"sort"
	"time"

	"github.com/maxmwang/scraie/flights/internal/db"
	"github.com/maxmwang/scraie/flights/internal/search"
	"github.com/maxmwang/scraie/flights/internal/util"
)

// dailyMinimumPriceSeries returns two series, the first mapping onto the
// second (ie. X and Y-axis values). The Xs are dates in ascending order. The
// Ys are the minimum observed search.FlightOptions price on each day. Each X
// value is anchored to the start of that day in the observation's own
// location.
func dailyMinimumPriceSeries(history []search.FlightOptions) ([]time.Time, []float64) {
	minByDay := make(map[time.Time]float64)
	for _, o := range history {
		if !o.Option.SearchedAt.Valid {
			continue
		}
		t := o.Option.SearchedAt.Time
		y, m, d := t.Date()
		day := time.Date(y, m, d, 0, 0, 0, 0, t.Location())
		price := float64(o.Option.Price)
		if cur, ok := minByDay[day]; !ok || price < cur {
			minByDay[day] = price
		}
	}

	days := make([]time.Time, 0, len(minByDay))
	for day := range minByDay {
		days = append(days, day)
	}
	sort.Slice(days, func(a, b int) bool { return days[a].Before(days[b]) })

	xs := make([]time.Time, len(days))
	ys := make([]float64, len(days))
	for i, day := range days {
		xs[i] = day
		ys[i] = minByDay[day]
	}
	return xs, ys
}

// buildDailyMinimumPriceChartURL renders the recent daily-minimum price history
// as a QuickChart (https://quickchart.io) line chart and returns its image URL,
// suitable for use as a Discord embed image. It returns "" (without error) when
// there is no data to plot.
func buildDailyMinimumPriceChartURL(it db.Itinerary, history []search.FlightOptions, nDaysToChart int) (string, error) {
	xs, ys := dailyMinimumPriceSeries(history)
	if len(xs) == 0 {
		return "", nil
	}
	if len(xs) >= nDaysToChart {
		xs = xs[len(xs)-nDaysToChart:]
		ys = ys[len(ys)-nDaysToChart:]
	}

	labels := make([]string, len(xs))
	for i, t := range xs {
		labels[i] = t.Format("Jan 2")
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
			},
		},
	}

	c, err := json.Marshal(config)
	if err != nil {
		return "", fmt.Errorf("buildDailyMinimumPriceChartURL: %w", err)
	}
	return "https://quickchart.io/chart?w=900&h=400&bkg=white&c=" + url.QueryEscape(string(c)), nil
}

/*
 *
 func buildDailyMinimumPriceChart(it db.Itinerary, history []search.FlightOptions, nDaysToChart int) ([]byte, error) {
	xs, ys := dailyMinimumPriceSeries(history)
	if len(xs) >= nDaysToChart {
		xs = xs[len(xs)-nDaysToChart:]
		ys = ys[len(ys)-nDaysToChart:]
	}

	series := chart.TimeSeries{
		XValues: xs,
		YValues: ys,
		Style: chart.Style{
			StrokeWidth: 2,
			DotWidth:    3,
		},
	}

	title := itineraryHeader(it, " -> ", " <-> ")

	graph := chart.Chart{
		Title:  title,
		Width:  900,
		Height: 400,
		Background: chart.Style{
			Padding: chart.Box{Top: 60, Left: 20, Right: 20, Bottom: 20},
		},
		YAxis: chart.YAxis{
			ValueFormatter: func(v interface{}) string {
				if f, ok := v.(float64); ok {
					return fmt.Sprintf("%s%.0f", currencySymbol(it.Currency), f)
				}
				return ""
			},
		},
		Series: []chart.Series{series},
	}

	buf := bytes.NewBuffer(nil)
	if err := graph.Render(chart.PNG, buf); err != nil {
		return nil, fmt.Errorf("buildDailyMinimumPriceChart: %w", err)
	}
	return buf.Bytes(), nil
 }

 *

// buildPriceHistoryChart renders a PNG line chart of the recent price history of
// each flight in top, drawn from the observations in history. It returns nil
// (without error) when there are fewer than two distinct observation times, as
// there is then nothing meaningful to plot.
func buildPriceHistoryChart(it db.Itinerary, top, history []search.FlightOptions) ([]byte, error) {
	cur := currencySymbol(it.Currency)
	// Group every historical observation by flight signature so each current
	// top flight can be matched to its own price series.
	historyBySig := make(map[string][]pricePoint)
	allSearches := make(map[int64]struct{})
	var minT, maxT time.Time
	for _, o := range history {
		if !o.Option.SearchedAt.Valid {
			continue
		}
		t := o.Option.SearchedAt.Time
		allSearches[t.Unix()] = struct{}{}
		if minT.IsZero() || t.Before(minT) {
			minT = t
		}
		if maxT.IsZero() || t.After(maxT) {
			maxT = t
		}
		sig := flightSignature(o)
		historyBySig[sig] = append(historyBySig[sig], pricePoint{
			t:     t,
			price: float64(o.Option.Price),
		})
	}
	span := "none"
	if !minT.IsZero() {
		span = fmt.Sprintf("%s .. %s (%d days)", minT.Format("2006-01-02"), maxT.Format("2006-01-02"),
			int(maxT.Sub(minT).Hours()/24)+1)
	}
	fmt.Fprintf(os.Stderr, "itinerary[%d] price history window: %d options across %d searches, %d distinct flights, span %s\n",
		it.ID, len(history), len(allSearches), len(historyBySig), span)

	var series []chart.Series
	distinctTimes := make(map[int64]struct{})
	for i, fo := range top {
		points := historyBySig[flightSignature(fo)]
		if len(points) == 0 {
			continue
		}
		sort.Slice(points, func(a, b int) bool { return points[a].t.Before(points[b].t) })

		label := seriesLabel(i, fo)
		xs := make([]time.Time, len(points))
		ys := make([]float64, len(points))
		logged := make([]string, len(points))
		for j, p := range points {
			xs[j] = p.t
			ys[j] = p.price
			distinctTimes[p.t.Unix()] = struct{}{}
			logged[j] = fmt.Sprintf("{%s, %s%.0f}", p.t.Format("2006-01-02 15:04"), cur, p.price)
		}
		fmt.Fprintf(os.Stderr, "itinerary[%d] chart series %s (%d points): [%s]\n",
			it.ID, label, len(points), strings.Join(logged, ", "))

		color := chartSeriesColors[i%len(chartSeriesColors)]
		series = append(series, chart.TimeSeries{
			Name:    label,
			XValues: xs,
			YValues: ys,
			Style: chart.Style{
				StrokeColor: color,
				StrokeWidth: 2,
				DotColor:    color,
				DotWidth:    3,
			},
		})
	}

	// Add a baseline line tracing the cheapest fare seen each calendar day,
	// across every option (not just the current top 3).
	if minXs, minYs := dailyMinimumPriceSeries(history); len(minXs) > 0 {
		for _, t := range minXs {
			distinctTimes[t.Unix()] = struct{}{}
		}
		logged := make([]string, len(minXs))
		for j := range minXs {
			logged[j] = fmt.Sprintf("{%s, %s%.0f}", minXs[j].Format("2006-01-02"), cur, minYs[j])
		}
		fmt.Fprintf(os.Stderr, "itinerary[%d] chart series Daily min (%d points): [%s]\n",
			it.ID, len(minXs), strings.Join(logged, ", "))

		series = append(series, chart.TimeSeries{
			Name:    "Daily min",
			XValues: minXs,
			YValues: minYs,
			Style: chart.Style{
				StrokeColor:     dailyMinColor,
				StrokeWidth:     2,
				StrokeDashArray: []float64{5, 5},
			},
		})
	}

	if len(series) == 0 || len(distinctTimes) < 2 {
		return nil, nil
	}

	graph := chart.Chart{
		Title:  fmt.Sprintf("%s → %s — price history (past month)", it.DepartureID, it.ArrivalID),
		Width:  900,
		Height: 400,
		Background: chart.Style{
			Padding: chart.Box{Top: 60, Left: 20, Right: 20, Bottom: 20},
		},
		XAxis: chart.XAxis{
			ValueFormatter: chart.TimeDateValueFormatter,
		},
		YAxis: chart.YAxis{
			ValueFormatter: func(v interface{}) string {
				if f, ok := v.(float64); ok {
					return fmt.Sprintf("%s%.0f", cur, f)
				}
				return ""
			},
		},
		Series: series,
	}
	graph.Elements = []chart.Renderable{chart.Legend(&graph)}

	buf := bytes.NewBuffer(nil)
	if err := graph.Render(chart.PNG, buf); err != nil {
		return nil, fmt.Errorf("render price history chart: %w", err)
	}
	return buf.Bytes(), nil
}

// seriesLabel builds a legend label for a flight, e.g. "#1 UA 123 / UA 456".
func seriesLabel(rank int, fo search.FlightOptions) string {
	nums := make([]string, 0, len(fo.Segments))
	for _, s := range fo.Segments {
		if s.FlightNumber != "" {
			nums = append(nums, s.FlightNumber)
		}
	}
	label := strings.Join(nums, " / ")
	if label == "" {
		label = "flight"
	}
	return fmt.Sprintf("#%d %s", rank+1, label)
}
*/
