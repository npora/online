package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"
)

func (a *app) changePassword(w http.ResponseWriter, r *http.Request) {
	user, _, err := a.authenticate(r)
	if err != nil {
		writeError(w, 401, 401, "Authentication required")
		return
	}
	var in struct {
		CurrentPassword string `json:"currentPassword"`
		NewPassword     string `json:"newPassword"`
		TOTPCode        string `json:"totpCode"`
	}
	if !decodeJSON(w, r, &in) || len(in.NewPassword) < 6 || len(in.NewPassword) > 128 || bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(in.CurrentPassword)) != nil {
		writeError(w, 400, 400, "Invalid password")
		return
	}
	if a.totpEnabled(r.Context(), user.ID) {
		writeError(w, 501, 501, "Authenticator password changes are not implemented in this example")
		return
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(in.NewPassword), bcrypt.DefaultCost)
	if err != nil {
		writeError(w, 500, 500, "Unable to update password")
		return
	}
	tx, err := a.db.BeginTx(r.Context(), nil)
	if err != nil {
		writeError(w, 503, 503, "Unable to update password")
		return
	}
	defer tx.Rollback()
	now := time.Now().UnixMilli()
	if _, err := tx.ExecContext(r.Context(), `UPDATE sys_user SET password_hash=?,updated_at=? WHERE id=?`, string(hash), now, user.ID); err != nil {
		writeError(w, 503, 503, "Unable to update password")
		return
	}
	if _, err := tx.ExecContext(r.Context(), `DELETE FROM sys_auth_session WHERE user_id=?`, user.ID); err != nil {
		writeError(w, 503, 503, "Unable to revoke sessions")
		return
	}
	if err := tx.Commit(); err != nil {
		writeError(w, 503, 503, "Unable to update password")
		return
	}
	writeJSON(w, 200, envelope{Code: 0, Message: "success", Data: nil})
}

func (a *app) resetPassword(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Username    string `json:"username"`
		TOTPCode    string `json:"totpCode"`
		NewPassword string `json:"newPassword"`
	}
	if !decodeJSON(w, r, &in) || len(in.NewPassword) < 6 || len(in.NewPassword) > 128 {
		writeError(w, 400, 400, "Invalid request")
		return
	}
	// Deliberately do not look up the username until reset is implemented so this endpoint
	// cannot be used to enumerate accounts.
	writeError(w, 501, 501, "Password reset is not implemented in this example")
}

func (a *app) totpStatus(w http.ResponseWriter, r *http.Request) {
	u, _, err := a.authenticate(r)
	if err != nil {
		writeError(w, 401, 401, "Authentication required")
		return
	}
	enabled := a.totpEnabled(r.Context(), u.ID)
	writeJSON(w, 200, envelope{Code: 0, Message: "success", Data: map[string]any{"available": false, "enabled": enabled}})
}
func (a *app) totpNotImplemented(w http.ResponseWriter, r *http.Request) {
	if _, _, err := a.authenticate(r); err != nil {
		writeError(w, 401, 401, "Authentication required")
		return
	}
	writeError(w, 501, 501, "Authenticator operations are not implemented in this example")
}
func (a *app) totpEnabled(ctx context.Context, id uint64) bool {
	var enabled int
	err := a.db.QueryRowContext(ctx, `SELECT COALESCE((SELECT enabled FROM sys_user_totp WHERE user_id=?),0)`, id).Scan(&enabled)
	return err == nil && enabled == 1
}

func currentSessionHash(a *app, r *http.Request) (string, error) {
	token := strings.TrimSpace(strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "))
	if token == "" {
		return "", errors.New("missing token")
	}
	sum := sha256.Sum256(append(a.pepper, []byte(token)...))
	return hex.EncodeToString(sum[:]), nil
}
func (a *app) ownSessions(w http.ResponseWriter, r *http.Request) {
	u, _, err := a.authenticate(r)
	if err != nil {
		writeError(w, 401, 401, "Authentication required")
		return
	}
	list, err := a.sessionList(r.Context(), u.ID, r)
	if err != nil {
		writeError(w, 503, 503, "Unable to query sessions")
		return
	}
	writeJSON(w, 200, envelope{Code: 0, Message: "success", Data: list})
}
func (a *app) sessionList(ctx context.Context, userID uint64, r *http.Request) ([]map[string]any, error) {
	current, err := currentSessionHash(a, r)
	if err != nil {
		return nil, err
	}
	rows, err := a.db.QueryContext(ctx, `SELECT public_id,created_at,expires_at,token_hash FROM sys_auth_session WHERE user_id=? AND expires_at>? ORDER BY created_at DESC`, userID, time.Now().UnixMilli())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var id, hash string
		var created, expires int64
		if err := rows.Scan(&id, &created, &expires, &hash); err != nil {
			return nil, err
		}
		out = append(out, map[string]any{"id": id, "createdAt": created, "expiresAt": expires, "current": hash == current})
	}
	return out, rows.Err()
}
func (a *app) revokeOwnSession(w http.ResponseWriter, r *http.Request) {
	u, _, err := a.authenticate(r)
	if err != nil {
		writeError(w, 401, 401, "Authentication required")
		return
	}
	id := r.PathValue("sessionId")
	if !validSessionID(id) {
		writeError(w, 400, 400, "Invalid session id")
		return
	}
	current, err := currentSessionHash(a, r)
	if err != nil {
		writeError(w, 401, 401, "Authentication required")
		return
	}
	var currentID string
	if err := a.db.QueryRowContext(r.Context(), `SELECT public_id FROM sys_auth_session WHERE token_hash=?`, current).Scan(&currentID); err != nil {
		writeError(w, 401, 401, "Authentication required")
		return
	}
	if id == currentID {
		writeError(w, 409, 409, "Cannot revoke current session")
		return
	}
	n, err := a.deleteSession(r.Context(), u.ID, id)
	if err != nil {
		writeError(w, 503, 503, "Unable to revoke session")
		return
	}
	if n == 0 {
		writeError(w, 404, 404, "Session not found")
		return
	}
	writeJSON(w, 200, envelope{Code: 0, Message: "success", Data: map[string]int64{"revoked": n}})
}
func (a *app) revokeOtherSessions(w http.ResponseWriter, r *http.Request) {
	u, _, err := a.authenticate(r)
	if err != nil {
		writeError(w, 401, 401, "Authentication required")
		return
	}
	current, err := currentSessionHash(a, r)
	if err != nil {
		writeError(w, 401, 401, "Authentication required")
		return
	}
	res, err := a.db.ExecContext(r.Context(), `DELETE FROM sys_auth_session WHERE user_id=? AND token_hash<>?`, u.ID, current)
	if err != nil {
		writeError(w, 503, 503, "Unable to revoke sessions")
		return
	}
	n, _ := res.RowsAffected()
	writeJSON(w, 200, envelope{Code: 0, Message: "success", Data: map[string]int64{"revoked": n}})
}
func (a *app) deleteSession(ctx context.Context, userID uint64, id string) (int64, error) {
	res, err := a.db.ExecContext(ctx, `DELETE FROM sys_auth_session WHERE user_id=? AND public_id=? AND expires_at>?`, userID, id, time.Now().UnixMilli())
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}
func validSessionID(id string) bool {
	if len(id) != 32 {
		return false
	}
	_, err := hex.DecodeString(id)
	return err == nil
}
func parsePathID(r *http.Request, key string) (uint64, bool) {
	id, err := strconv.ParseUint(r.PathValue(key), 10, 64)
	return id, err == nil && id > 0
}
