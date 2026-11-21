package server

import (
	"encoding/json"
	"time"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/security"
	"github.com/pocketbase/pocketbase/tools/types"
)

const (
	dailyModelUsageCollection = "daily_model_usage"
	recordIDAlphabet          = "abcdefghijklmnopqrstuvwxyz0123456789"
	recordIDLength            = 15
)

type tokenUsage struct {
	PromptTokens     int
	CompletionTokens int
	TotalTokens      int
}

func parseUpstreamUsage(body []byte) tokenUsage {
	var payload struct {
		Usage *struct {
			PromptTokens     int `json:"prompt_tokens"`
			CompletionTokens int `json:"completion_tokens"`
			TotalTokens      int `json:"total_tokens"`
		} `json:"usage"`
	}
	if err := json.Unmarshal(body, &payload); err != nil || payload.Usage == nil {
		return tokenUsage{}
	}

	prompt := max(payload.Usage.PromptTokens, 0)
	completion := max(payload.Usage.CompletionTokens, 0)
	total := max(payload.Usage.TotalTokens, 0)
	if total == 0 {
		total = prompt + completion
	}
	return tokenUsage{
		PromptTokens:     prompt,
		CompletionTokens: completion,
		TotalTokens:      total,
	}
}

func recordDailyModelUsage(app core.App, userID string, candidate modelCandidate, respBody []byte, now time.Time) {
	if userID == "" || candidate.Provider == "" || candidate.Model == "" {
		return
	}

	usage := parseUpstreamUsage(respBody)
	utcNow := now.UTC()
	date := utcNow.Format("2006-01-02")
	ts, _ := types.ParseDateTime(utcNow)

	_, err := app.DB().NewQuery(`
		INSERT INTO daily_model_usage (
			id, user, provider, model, date,
			requests, prompt_tokens, completion_tokens, total_tokens,
			created, updated
		) VALUES (
			{:id}, {:user}, {:provider}, {:model}, {:date},
			1, {:prompt}, {:completion}, {:total},
			{:now}, {:now}
		)
		ON CONFLICT(user, provider, model, date) DO UPDATE SET
			requests = daily_model_usage.requests + 1,
			prompt_tokens = daily_model_usage.prompt_tokens + excluded.prompt_tokens,
			completion_tokens = daily_model_usage.completion_tokens + excluded.completion_tokens,
			total_tokens = daily_model_usage.total_tokens + excluded.total_tokens,
			updated = excluded.updated
	`).Bind(dbx.Params{
		"id":         security.RandomStringWithAlphabet(recordIDLength, recordIDAlphabet),
		"user":       userID,
		"provider":   candidate.Provider,
		"model":      candidate.Model,
		"date":       date,
		"prompt":     usage.PromptTokens,
		"completion": usage.CompletionTokens,
		"total":      usage.TotalTokens,
		"now":        ts.String(),
	}).Execute()
	if err != nil {
		app.Logger().Warn("failed to record daily model usage",
			"user", userID,
			"provider", candidate.Provider,
			"model", candidate.Model,
			"error", err,
		)
	}
}

func currentUTCWeekWindow(now time.Time) (startDate, nextStartDate string, retryAt time.Time) {
	utc := now.UTC()
	daysSinceMonday := (int(utc.Weekday()) + 6) % 7
	weekStart := time.Date(utc.Year(), utc.Month(), utc.Day(), 0, 0, 0, 0, time.UTC).AddDate(0, 0, -daysSinceMonday)
	nextWeekStart := weekStart.AddDate(0, 0, 7)
	return weekStart.Format("2006-01-02"), nextWeekStart.Format("2006-01-02"), nextWeekStart
}

func checkWeeklyTokenQuota(app core.App, user *core.Record, now time.Time) (exceeded bool, retryAt time.Time, err error) {
	quota := user.GetInt("weekly_token_quota")
	if quota <= 0 {
		return false, time.Time{}, nil
	}

	startDate, nextStartDate, nextRetry := currentUTCWeekWindow(now)
	var row struct {
		Used int `db:"used"`
	}
	err = app.DB().NewQuery(`
		SELECT COALESCE(SUM(total_tokens), 0) AS used
		FROM daily_model_usage
		WHERE user = {:user}
		  AND date >= {:startDate}
		  AND date < {:nextStartDate}
	`).Bind(dbx.Params{
		"user":          user.Id,
		"startDate":     startDate,
		"nextStartDate": nextStartDate,
	}).One(&row)
	if err != nil {
		return false, time.Time{}, err
	}
	return row.Used >= quota, nextRetry, nil
}
