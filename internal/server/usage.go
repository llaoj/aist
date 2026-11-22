package server

import (
	"encoding/json"
	"net/http"
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

type weeklyUsage struct {
	WeekStart        string
	ResetAt          time.Time
	Requests         int
	PromptTokens     int
	CompletionTokens int
	TotalTokens      int
}

type accountUsageResponse struct {
	WeekStart        string `json:"week_start"`
	ResetAt          string `json:"reset_at"`
	Requests         int    `json:"requests"`
	PromptTokens     int    `json:"prompt_tokens"`
	CompletionTokens int    `json:"completion_tokens"`
	UsedTokens       int    `json:"used_tokens"`
	WeeklyTokenQuota int    `json:"weekly_token_quota"`
	RemainingTokens  *int   `json:"remaining_tokens,omitempty"`
}

func queryWeeklyUsage(app core.App, userID string, now time.Time) (weeklyUsage, error) {
	startDate, nextStartDate, resetAt := currentUTCWeekWindow(now)
	var row struct {
		Requests         int `db:"requests"`
		PromptTokens     int `db:"prompt_tokens"`
		CompletionTokens int `db:"completion_tokens"`
		TotalTokens      int `db:"total_tokens"`
	}
	err := app.DB().NewQuery(`
		SELECT
			COALESCE(SUM(requests), 0) AS requests,
			COALESCE(SUM(prompt_tokens), 0) AS prompt_tokens,
			COALESCE(SUM(completion_tokens), 0) AS completion_tokens,
			COALESCE(SUM(total_tokens), 0) AS total_tokens
		FROM daily_model_usage
		WHERE user = {:user}
		  AND date >= {:startDate}
		  AND date < {:nextStartDate}
	`).Bind(dbx.Params{
		"user":          userID,
		"startDate":     startDate,
		"nextStartDate": nextStartDate,
	}).One(&row)
	if err != nil {
		return weeklyUsage{}, err
	}
	return weeklyUsage{
		WeekStart:        startDate,
		ResetAt:          resetAt,
		Requests:         max(row.Requests, 0),
		PromptTokens:     max(row.PromptTokens, 0),
		CompletionTokens: max(row.CompletionTokens, 0),
		TotalTokens:      max(row.TotalTokens, 0),
	}, nil
}

func checkWeeklyTokenQuota(app core.App, user *core.Record, now time.Time) (exceeded bool, retryAt time.Time, err error) {
	quota := user.GetInt("weekly_token_quota")
	if quota <= 0 {
		return false, time.Time{}, nil
	}

	usage, err := queryWeeklyUsage(app, user.Id, now)
	if err != nil {
		return false, time.Time{}, err
	}
	return usage.TotalTokens >= quota, usage.ResetAt, nil
}

func (s *API) handleAccountUsage(e *core.RequestEvent) error {
	if e.Auth == nil {
		return e.UnauthorizedError("Authentication is required.", nil)
	}

	usage, err := queryWeeklyUsage(e.App, e.Auth.Id, s.now())
	if err != nil {
		e.App.Logger().Warn("failed to query weekly usage", "user", e.Auth.Id, "error", err)
		return e.InternalServerError("Failed to query account usage.", nil)
	}

	quota := max(e.Auth.GetInt("weekly_token_quota"), 0)
	resp := accountUsageResponse{
		WeekStart:        usage.WeekStart,
		ResetAt:          usage.ResetAt.UTC().Format(time.RFC3339),
		Requests:         usage.Requests,
		PromptTokens:     usage.PromptTokens,
		CompletionTokens: usage.CompletionTokens,
		UsedTokens:       usage.TotalTokens,
		WeeklyTokenQuota: quota,
	}
	if quota > 0 {
		rem := max(quota-usage.TotalTokens, 0)
		resp.RemainingTokens = &rem
	}
	return e.JSON(http.StatusOK, resp)
}
