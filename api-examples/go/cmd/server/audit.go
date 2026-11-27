package main

import (
	"context"
	"database/sql"
	"net/http"
	"strconv"
	"strings"
)

func (a *app) auditLogs(w http.ResponseWriter, r *http.Request) {
	if _, ok := a.guard(w, r, "system:audit-log:list"); !ok {
		return
	}
	page, size := pageQuery(r)
	where := []string{"1=1"}
	args := []any{}
	q := r.URL.Query()
	for key, col := range map[string]string{"action": "action", "result": "result"} {
		if value := q.Get(key); value != "" {
			if key == "action" && !validAuditAction(value) || key == "result" && value != "success" && value != "failure" {
				writeError(w, 400, 400, "Invalid audit filter")
				return
			}
			where = append(where, col+"=?")
			args = append(args, value)
		}
	}
	for key, col := range map[string]string{"actorId": "actor_id", "targetId": "target_id", "createdFrom": "created_at >=", "createdTo": "created_at <"} {
		value := q.Get(key)
		if value == "" {
			continue
		}
		n, err := strconv.ParseInt(value, 10, 64)
		if err != nil || n < 0 || n > 9007199254740991 || (key != "createdFrom" && key != "createdTo" && n == 0) {
			writeError(w, 400, 400, "Invalid audit filter")
			return
		}
		where = append(where, col+"?")
		args = append(args, n)
	}
	if from, to := q.Get("createdFrom"), q.Get("createdTo"); from != "" && to != "" {
		f, _ := strconv.ParseInt(from, 10, 64)
		t, _ := strconv.ParseInt(to, 10, 64)
		if f >= t {
			writeError(w, 400, 400, "createdFrom must precede createdTo")
			return
		}
	}
	clause := strings.Join(where, " AND ")
	var total int64
	if err := a.db.QueryRowContext(r.Context(), `SELECT COUNT(*) FROM sys_audit_log WHERE `+clause, args...).Scan(&total); err != nil {
		writeError(w, 503, 503, "Audit log unavailable")
		return
	}
	args = append(args, size, (page-1)*size)
	rows, err := a.db.QueryContext(r.Context(), `SELECT id,actor_id,target_type,target_id,action,result,status_code,created_at FROM sys_audit_log WHERE `+clause+` ORDER BY created_at DESC,id DESC LIMIT ? OFFSET ?`, args...)
	if err != nil {
		writeError(w, 503, 503, "Audit log unavailable")
		return
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var id, actor, target uint64
		var targetType, action, result string
		var code uint16
		var created uint64
		if err := rows.Scan(&id, &actor, &targetType, &target, &action, &result, &code, &created); err != nil {
			writeError(w, 503, 503, "Audit log unavailable")
			return
		}
		out = append(out, map[string]any{"id": id, "actorId": actor, "targetType": targetType, "targetId": target, "action": action, "result": result, "statusCode": code, "createdAt": created})
	}
	writeJSON(w, 200, envelope{Code: 0, Message: "success", Data: map[string]any{"list": out, "total": total}})
}
func validAuditAction(s string) bool {
	switch s {
	case "password_reset", "totp_enable", "totp_disable", "role_grant", "session_revoke":
		return true
	}
	return false
}

func insertAudit(ctx context.Context, tx *sql.Tx, actorID, targetID uint64, targetType, action string, status int, createdAt int64) error {
	result := "success"
	if status < 200 || status >= 300 {
		result = "failure"
	}
	_, err := tx.ExecContext(ctx, `INSERT INTO sys_audit_log(actor_id,target_type,target_id,action,result,status_code,created_at) VALUES(?,?,?,?,?,?,?)`, actorID, targetType, targetID, action, result, status, createdAt)
	return err
}
