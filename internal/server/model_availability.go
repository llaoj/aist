package server

import (
	"time"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/types"
)

const (
	upstreamModelsCollection = "upstream_models"

	modelUnavailableReasonQuota     = "quota"
	modelUnavailableReasonRateLimit = "rate_limit"
	modelUnavailableReasonAuth      = "auth"

	defaultQuotaCooldown     = 6 * time.Hour
	backoffQuotaCooldown     = 24 * time.Hour
	defaultRateLimitCooldown = 30 * time.Second
	backoffRateLimitCooldown = 2 * time.Minute
	maxRateLimitCooldown     = time.Hour
)

func (s *API) bindModelHooks(app core.App) {
	app.OnRecordUpdate(upstreamModelsCollection).BindFunc(s.normalizeModelAvailabilityOnSave)
}

func (s *API) normalizeModelAvailabilityOnSave(e *core.RecordEvent) error {
	record := e.Record
	orig := record.Original()

	reason := record.GetString("unavailable_reason")
	origReason := orig.GetString("unavailable_reason")
	until := record.GetDateTime("unavailable_until")
	origUntil := orig.GetDateTime("unavailable_until")

	if origReason != "" && reason == "" {
		record.Set("unavailable_until", types.DateTime{})
		record.Set("last_error", "")
		return e.Next()
	}
	if reason != modelUnavailableReasonAuth && !origUntil.IsZero() && until.IsZero() && reason == origReason {
		record.Set("unavailable_reason", "")
		record.Set("last_error", "")
	}
	return e.Next()
}

func (c modelCandidate) isAvailableAt(now time.Time) bool {
	if c.UnavailableReason == modelUnavailableReasonAuth {
		return false
	}
	if !c.UnavailableUntil.IsZero() && c.UnavailableUntil.After(now) {
		return false
	}
	return true
}

func (c modelCandidate) hasAvailabilityState() bool {
	return c.UnavailableReason != "" || !c.UnavailableUntil.IsZero() || c.LastError != ""
}

func filterAvailableCandidates(candidates []modelCandidate, now time.Time) []modelCandidate {
	out := make([]modelCandidate, 0, len(candidates))
	for _, c := range candidates {
		if c.isAvailableAt(now) {
			out = append(out, c)
		}
	}
	return out
}

func recordModelOutcome(app core.App, candidate modelCandidate, classified classifiedUpstreamError, now time.Time) {
	var reason string
	var until types.DateTime

	switch classified.Kind {
	case upstreamErrorKindQuota:
		reason = modelUnavailableReasonQuota
		cooldown := defaultQuotaCooldown
		if candidate.UnavailableReason == modelUnavailableReasonQuota {
			cooldown = backoffQuotaCooldown
		}
		until, _ = types.ParseDateTime(now.Add(cooldown).UTC())

	case upstreamErrorKindRateLimit:
		reason = modelUnavailableReasonRateLimit
		cooldown := classified.RetryAfter
		switch {
		case cooldown > maxRateLimitCooldown:
			cooldown = maxRateLimitCooldown
		case cooldown > 0 && cooldown < time.Second:
			cooldown = time.Second
		case cooldown <= 0 && candidate.UnavailableReason == modelUnavailableReasonRateLimit:
			cooldown = backoffRateLimitCooldown
		case cooldown <= 0:
			cooldown = defaultRateLimitCooldown
		}
		until, _ = types.ParseDateTime(now.Add(cooldown).UTC())

	case upstreamErrorKindAuth:
		reason = modelUnavailableReasonAuth

	default:
		return
	}

	record, err := app.FindRecordById(upstreamModelsCollection, candidate.ModelID)
	if err != nil || record == nil {
		return
	}
	record.Set("unavailable_reason", reason)
	record.Set("unavailable_until", until)
	record.Set("last_error", classified.Summary)
	if err := app.Save(record); err != nil {
		app.Logger().Warn("failed to save model availability state",
			"model", candidate.Model,
			"error", err,
		)
	}
}

func clearModelAvailability(app core.App, modelID string) {
	record, err := app.FindRecordById(upstreamModelsCollection, modelID)
	if err != nil || record == nil {
		return
	}
	record.Set("unavailable_reason", "")
	record.Set("unavailable_until", types.DateTime{})
	record.Set("last_error", "")
	if err := app.Save(record); err != nil {
		app.Logger().Warn("failed to clear model availability state",
			"modelId", modelID,
			"error", err,
		)
	}
}

func clearProviderAuthUnavailable(app core.App, providerID string) {
	if providerID == "" {
		return
	}
	records, err := app.FindAllRecords(upstreamModelsCollection, dbx.HashExp{
		"provider":           providerID,
		"unavailable_reason": modelUnavailableReasonAuth,
	})
	if err != nil {
		return
	}
	for _, rec := range records {
		rec.Set("unavailable_reason", "")
		rec.Set("unavailable_until", types.DateTime{})
		rec.Set("last_error", "")
		_ = app.Save(rec)
	}
}
