package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

const (
	targetDate = "2026-06-07"
	pingUserID = "198996752362110976"
)

type hotel struct {
	code       string
	pmsSeqNo   string
	bookingURL string
}

var hotels = []hotel{
	{"IAT4", "853", "https://www.walkerhill.com/transithotel/en/reservation/RoomSearchIntro.jsp"},
}

type remainRoomResponse struct {
	Result []struct {
		EnableDate string `json:"enabledate"`
	} `json:"result"`
}

func main() {
	Main(map[string]interface{}{})
}

func Main(_ map[string]interface{}) map[string]interface{} {
	setupLogger()

	if err := run(); err != nil {
		log.Error().Err(err).Msg("icn-hotel failed")
		return map[string]interface{}{"error": err.Error()}
	}
	return map[string]interface{}{"body": "ok"}
}

func run() error {
	webhook := os.Getenv("ICN_HOTEL_DISCORD_WEBHOOK")
	if webhook == "" {
		return fmt.Errorf("ICN_HOTEL_DISCORD_WEBHOOK is unset")
	}

	today := time.Now()
	start := today.Format("2006-01-02")
	end := today.AddDate(0, 0, 58).Format("2006-01-02")

	if targetDate < start || targetDate > end {
		return fmt.Errorf("target %s outside query window %s..%s", targetDate, start, end)
	}

	var matched []hotel
	for _, h := range hotels {
		enabled, err := checkHotel(h.code, h.pmsSeqNo, start, end)
		if err != nil {
			log.Error().Err(err).Str("hotel", h.code).Msg("check failed")
			continue
		}

		available := false
		for _, d := range enabled {
			if d == targetDate {
				available = true
				break
			}
		}

		log.Info().
			Str("hotel", h.code).
			Str("start", start).
			Str("end", end).
			Int("enabled", len(enabled)).
			Str("target", targetDate).
			Bool("available", available).
			Msg("icn-hotel availability")

		if available {
			matched = append(matched, h)
		}
	}

	if len(matched) == 0 {
		return nil
	}

	if err := notifyDiscord(webhook, matched); err != nil {
		return fmt.Errorf("discord notify: %w", err)
	}
	codes := make([]string, len(matched))
	for i, m := range matched {
		codes[i] = m.code
	}
	log.Info().Str("target", targetDate).Strs("hotels", codes).Msg("discord notified")
	return nil
}

func checkHotel(code, pmsSeqNo, start, end string) ([]string, error) {
	body, err := fetchAvailability(code, pmsSeqNo, start, end)
	if err != nil {
		return nil, err
	}

	fmt.Printf("--- %s raw response ---\n%s\n--- end %s ---\n", code, string(body), code)

	var parsed remainRoomResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, fmt.Errorf("parse response: %w (body=%s)", err, string(body))
	}

	dates := make([]string, len(parsed.Result))
	for i, r := range parsed.Result {
		dates[i] = r.EnableDate
	}
	return dates, nil
}

func fetchAvailability(code, pmsSeqNo, start, end string) ([]byte, error) {
	parameter := fmt.Sprintf(
		`{"pms_seq_no":"%s","start_date":"%s","end_date":"%s","channel_code":"WINGS_B2C","SS_PMS_SEQ_NO":"%s","SS_PMS_CODE":"%s","SS_LANG_TYPE":"EN","SS_OPERATION_MODE":"prod","SS_CURRENCY_TYPE":"KRW"}`,
		pmsSeqNo, start, end, pmsSeqNo, code,
	)

	form := url.Values{}
	form.Set("parameter", parameter)

	endpoint := fmt.Sprintf("https://be4.wingsbooking.com/%s/user/hotel/remainRoomInit", code)
	referer := fmt.Sprintf("https://be4.wingsbooking.com/%s?lang_type=EN", code)

	req, err := http.NewRequest(http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("accept", "application/json, text/javascript, */*; q=0.01")
	req.Header.Set("content-type", "application/x-www-form-urlencoded; charset=UTF-8")
	req.Header.Set("origin", "https://be4.wingsbooking.com")
	req.Header.Set("referer", referer)
	req.Header.Set("user-agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/148.0.0.0 Safari/537.36")
	req.Header.Set("x-requested-with", "XMLHttpRequest")
	if cookie := os.Getenv("ICN_HOTEL_COOKIE"); cookie != "" {
		req.Header.Set("cookie", cookie)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("send request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status %d: %s", resp.StatusCode, string(body))
	}
	return body, nil
}

func notifyDiscord(webhook string, matched []hotel) error {
	fields := make([]map[string]any, len(matched))
	for i, m := range matched {
		fields[i] = map[string]any{
			"name":   m.code,
			"value":  fmt.Sprintf("[Book now](%s)", m.bookingURL),
			"inline": true,
		}
	}

	payload, err := json.Marshal(map[string]any{
		"content": fmt.Sprintf("<@%s>", pingUserID),
		"allowed_mentions": map[string]any{
			"users": []string{pingUserID},
		},
		"embeds": []map[string]any{{
			"title":       fmt.Sprintf("ICN hotel available on %s", targetDate),
			"description": "**Price to beat:** KRW 180,000 (~$120 USD)",
			"color":       5763719,
			"fields":      fields,
		}},
	})
	if err != nil {
		return fmt.Errorf("marshal payload: %w", err)
	}

	req, err := http.NewRequest(http.MethodPost, webhook, bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("content-type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("send: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("status %d: %s", resp.StatusCode, string(body))
	}
	return nil
}

func setupLogger() {
	if os.Getenv("LOG_JSON") != "" {
		log.Logger = zerolog.New(os.Stdout).With().Timestamp().Logger()
		return
	}
	log.Logger = log.Output(zerolog.ConsoleWriter{
		Out:        os.Stdout,
		NoColor:    false,
		TimeFormat: time.DateTime,
	})
}
