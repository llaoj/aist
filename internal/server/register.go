package server

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"strings"
	"time"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/mails"
	"github.com/pocketbase/pocketbase/tools/types"
)

const (
	inviteCodesCollection = "invite_codes"

	maxRegisterBodyBytes int64 = 16 << 10
	maxInviteCodeLength        = 64

	registerErrorCodeInvalidInviteCode = "invalid_invite_code"
	registerErrorCodeInvalidEmail      = "invalid_email"
	registerErrorCodeEmailTaken        = "email_taken"
	registerErrorCodeInvalidPassword   = "invalid_password"
	registerErrorCodeInternal          = "internal_error"
)

var (
	errInvalidInviteCode = errors.New("invalid or exhausted invite code")
	errEmailTaken        = errors.New("email already registered")
)

type registerRequest struct {
	InviteCode string `json:"invite_code"`
	Email      string `json:"email"`
	Password   string `json:"password"`
}

type registerResponse struct {
	Email            string `json:"email"`
	VerificationSent bool   `json:"verification_sent"`
}

func (s *API) handleRegister(e *core.RequestEvent) error {
	if !hasJSONContentType(e.Request) {
		return respondProxyError(e, http.StatusUnsupportedMediaType, proxyErrorCodeInvalidRequest, "Content-Type must be application/json", time.Time{})
	}

	var req registerRequest
	if err := decodeJSON(e.Request.Body, &req); err != nil {
		if errors.Is(err, apis.ErrRequestEntityTooLarge) {
			return err
		}
		return respondProxyError(e, http.StatusBadRequest, proxyErrorCodeInvalidRequest, "invalid JSON payload", time.Time{})
	}

	req.InviteCode = strings.TrimSpace(req.InviteCode)
	req.Email = strings.ToLower(strings.TrimSpace(req.Email))

	if req.InviteCode == "" || len(req.InviteCode) > maxInviteCodeLength {
		return respondProxyError(e, http.StatusBadRequest, registerErrorCodeInvalidInviteCode, "invalid or expired invite code", time.Time{})
	}

	usersCol, err := e.App.FindCachedCollectionByNameOrId(usersCollection)
	if err != nil {
		return respondProxyError(e, http.StatusInternalServerError, registerErrorCodeInternal, "registration failed", time.Time{})
	}
	if code, msg := validateCandidateUser(e.Request.Context(), e.App, usersCol, req); code != "" {
		return respondProxyError(e, http.StatusBadRequest, code, msg, time.Time{})
	}

	now := s.now().UTC()
	user, err := redeemInviteAndCreateUser(e.App, now, req)
	if err != nil {
		switch {
		case errors.Is(err, errInvalidInviteCode):
			return respondProxyError(e, http.StatusBadRequest, registerErrorCodeInvalidInviteCode, "invalid or expired invite code", time.Time{})
		case errors.Is(err, errEmailTaken):
			return respondProxyError(e, http.StatusConflict, registerErrorCodeEmailTaken, "email is already registered", time.Time{})
		default:
			e.App.Logger().Error("failed to register user", "email", req.Email, "error", err)
			return respondProxyError(e, http.StatusInternalServerError, registerErrorCodeInternal, "registration failed", time.Time{})
		}
	}

	verificationSent := false
	if e.App.Settings().SMTP.Enabled {
		if err := mails.SendRecordVerification(e.App, user); err != nil {
			e.App.Logger().Error("failed to send verification email", "email", user.Email(), "error", err)
		} else {
			verificationSent = true
		}
	}

	return e.JSON(http.StatusCreated, registerResponse{
		Email:            user.Email(),
		VerificationSent: verificationSent,
	})
}

func validateCandidateUser(ctx context.Context, app core.App, usersCol *core.Collection, req registerRequest) (string, string) {
	candidate := core.NewRecord(usersCol)
	candidate.SetEmail(req.Email)
	candidate.SetPassword(req.Password)

	if emailField := usersCol.Fields.GetByName(core.FieldNameEmail); emailField != nil {
		if err := emailField.ValidateValue(ctx, app, candidate); err != nil {
			return registerErrorCodeInvalidEmail, "invalid email address"
		}
	}
	if passwordField := usersCol.Fields.GetByName(core.FieldNamePassword); passwordField != nil {
		if err := passwordField.ValidateValue(ctx, app, candidate); err != nil {
			return registerErrorCodeInvalidPassword, "invalid password"
		}
	}
	if err := app.ValidateWithContext(ctx, candidate); err != nil {
		return proxyErrorCodeInvalidRequest, "invalid registration data"
	}
	return "", ""
}

func lookupValidInviteCode(app core.App, code string, now time.Time) (*core.Record, error) {
	record, err := app.FindFirstRecordByFilter(inviteCodesCollection, "code = {:code}", dbx.Params{"code": code})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, errInvalidInviteCode
		}
		return nil, err
	}
	if !isInviteCodeUsable(record, now) {
		return nil, errInvalidInviteCode
	}
	return record, nil
}

func isInviteCodeUsable(record *core.Record, now time.Time) bool {
	if record == nil || !record.GetBool("enabled") {
		return false
	}
	if expiresAt := record.GetDateTime("expires_at").Time(); !expiresAt.IsZero() && !expiresAt.After(now) {
		return false
	}
	maxUses := record.GetInt("max_uses")
	return maxUses == 0 || record.GetInt("used_count") < maxUses
}

func redeemInviteAndCreateUser(app core.App, now time.Time, req registerRequest) (*core.Record, error) {
	ts, _ := types.ParseDateTime(now.UTC())
	var created *core.Record

	err := app.RunInTransaction(func(txApp core.App) error {
		invite, err := lookupValidInviteCode(txApp, req.InviteCode, now)
		if err != nil {
			return err
		}

		usersCol, err := txApp.FindCollectionByNameOrId(usersCollection)
		if err != nil {
			return err
		}

		if existing, err := txApp.FindAuthRecordByEmail(usersCol, req.Email); err != nil && !errors.Is(err, sql.ErrNoRows) {
			return err
		} else if existing != nil {
			return errEmailTaken
		}

		res, err := txApp.DB().NewQuery(`
			UPDATE invite_codes
			SET used_count = used_count + 1,
				updated = {:now}
			WHERE id = {:id}
				AND enabled = 1
				AND (max_uses = 0 OR used_count < max_uses)
				AND (expires_at = '' OR expires_at > {:now})
		`).Bind(dbx.Params{
			"id":  invite.Id,
			"now": ts.String(),
		}).Execute()
		if err != nil {
			return err
		}
		affected, err := res.RowsAffected()
		if err != nil {
			return err
		}
		if affected != 1 {
			return errInvalidInviteCode
		}

		user := core.NewRecord(usersCol)
		user.SetEmail(req.Email)
		user.SetPassword(req.Password)
		if models := invite.GetStringSlice("allowed_models"); len(models) > 0 {
			user.Set("allowed_models", models)
		}
		if quota := invite.GetInt("weekly_token_quota"); quota > 0 {
			user.Set("weekly_token_quota", quota)
		}
		if rawSettings, err := toRawJSON(invite.Get("settings")); err == nil && len(rawSettings) > 0 && string(rawSettings) != "null" {
			user.Set("settings", json.RawMessage(rawSettings))
		}

		if err := txApp.Save(user); err != nil {
			return err
		}
		created = user
		return nil
	})
	if err != nil {
		return nil, err
	}
	return created, nil
}

func hasJSONContentType(r *http.Request) bool {
	mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	return err == nil && strings.EqualFold(mediaType, "application/json")
}

func decodeJSON(r io.Reader, dst any) error {
	dec := json.NewDecoder(r)
	if err := dec.Decode(dst); err != nil {
		return err
	}
	if err := dec.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return errors.New("unexpected trailing JSON data")
	}
	return nil
}
