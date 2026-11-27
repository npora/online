package main

import (
	"context"
	"database/sql"
	"net/http"
	"strconv"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"
)

type userInput struct {
	Username     string    `json:"username"`
	Password     string    `json:"password"`
	DisplayName  string    `json:"displayName"`
	Avatar       *string   `json:"avatar"`
	Email        *string   `json:"email"`
	Mobile       *string   `json:"mobile"`
	DepartmentID *uint64   `json:"departmentId"`
	Status       uint8     `json:"status"`
	RoleIDs      *[]uint64 `json:"roleIds"`
}

func (a *app) userRoleDTO(ctx context.Context, userID uint64) ([]map[string]any, []uint64, error) {
	rows, err := a.db.QueryContext(ctx, `SELECT r.id,r.code,r.name,r.status FROM sys_user_role ur JOIN sys_role r ON r.id=ur.role_id WHERE ur.user_id=? AND r.deleted_at IS NULL ORDER BY r.sort_order,r.id`, userID)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()
	roles := []map[string]any{}
	ids := []uint64{}
	for rows.Next() {
		var id uint64
		var code, name string
		var status uint8
		if err := rows.Scan(&id, &code, &name, &status); err != nil {
			return nil, nil, err
		}
		ids = append(ids, id)
		roles = append(roles, map[string]any{"id": id, "code": code, "name": name, "status": status})
	}
	return roles, ids, rows.Err()
}

func (a *app) createUser(w http.ResponseWriter, r *http.Request) {
	actor, ok := a.guard(w, r, "system:user:create")
	if !ok {
		return
	}
	var in userInput
	if !decodeJSON(w, r, &in) || len(in.Password) < 6 || len(in.Password) > 128 || !validName(in.DisplayName) || in.Status > 1 || in.RoleIDs == nil {
		writeError(w, 400, 400, "Invalid user")
		return
	}
	if !actor.IsSuperAdmin && !a.hasPermission(r.Context(), actor.ID, "system:user:assign-roles") {
		writeError(w, 403, 403, "Role assignment permission required")
		return
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(in.Password), bcrypt.DefaultCost)
	if err != nil {
		writeError(w, 500, 500, "Unable to hash password")
		return
	}
	in.Username = strings.TrimSpace(in.Username)
	if len(in.Username) < 3 || len(in.Username) > 64 {
		writeError(w, 400, 400, "Invalid username")
		return
	}
	if in.DepartmentID != nil {
		var status uint8
		if err := a.db.QueryRowContext(r.Context(), `SELECT status FROM sys_department WHERE id=? AND deleted_at IS NULL`, *in.DepartmentID).Scan(&status); err != nil || status != 1 {
			writeError(w, 400, 400, "Invalid department")
			return
		}
	}
	tx, err := a.db.BeginTx(r.Context(), nil)
	if err != nil {
		writeError(w, 503, 503, "Unable to create user")
		return
	}
	defer tx.Rollback()
	now := time.Now().UnixMilli()
	res, err := tx.ExecContext(r.Context(), `INSERT INTO sys_user(username,password_hash,display_name,avatar,email,mobile,department_id,status,is_super_admin,created_at,updated_at) VALUES(?,?,?,?,?,?,?, ?,0,?,?)`, in.Username, string(hash), in.DisplayName, in.Avatar, in.Email, in.Mobile, in.DepartmentID, in.Status, now, now)
	if err != nil {
		writeError(w, 409, 409, "Username, email, or mobile is already in use")
		return
	}
	rawID, err := res.LastInsertId()
	if err != nil {
		writeError(w, 503, 503, "Unable to create user")
		return
	}
	id := uint64(rawID)
	if err := replaceUserRoles(r, tx, id, *in.RoleIDs); err != nil {
		writeError(w, 400, 400, "Invalid role assignment")
		return
	}
	if err := tx.Commit(); err != nil {
		writeError(w, 503, 503, "Unable to create user")
		return
	}
	roles, roleIDs, _ := a.userRoleDTO(r.Context(), id)
	writeJSON(w, 201, envelope{Code: 0, Message: "success", Data: map[string]any{"id": id, "username": in.Username, "displayName": in.DisplayName, "avatar": in.Avatar, "email": in.Email, "mobile": in.Mobile, "departmentId": in.DepartmentID, "status": in.Status, "isSuperAdmin": false, "authenticatorEnabled": false, "roleIds": roleIDs, "roles": roles, "createdAt": now, "updatedAt": now}})
}

func (a *app) updateUser(w http.ResponseWriter, r *http.Request) {
	actor, ok := a.guard(w, r, "system:user:update")
	if !ok {
		return
	}
	id, ok := pathID(r)
	if !ok {
		writeError(w, 400, 400, "Invalid user id")
		return
	}
	var in userInput
	if !decodeJSON(w, r, &in) || in.Username != "" || in.Password != "" || !validName(in.DisplayName) || in.Status > 1 {
		writeError(w, 400, 400, "Invalid user")
		return
	}
	var currentSuper bool
	if err := a.db.QueryRowContext(r.Context(), `SELECT is_super_admin FROM sys_user WHERE id=? AND deleted_at IS NULL`, id).Scan(&currentSuper); err != nil {
		writeError(w, 404, 404, "User not found")
		return
	}
	if currentSuper && in.Status == 0 {
		writeError(w, 403, 403, "Cannot disable a super administrator")
		return
	}
	if in.DepartmentID != nil {
		var status uint8
		if err := a.db.QueryRowContext(r.Context(), `SELECT status FROM sys_department WHERE id=? AND deleted_at IS NULL`, *in.DepartmentID).Scan(&status); err != nil || status != 1 {
			writeError(w, 400, 400, "Invalid department")
			return
		}
	}
	tx, err := a.db.BeginTx(r.Context(), nil)
	if err != nil {
		writeError(w, 503, 503, "Unable to update user")
		return
	}
	defer tx.Rollback()
	now := time.Now().UnixMilli()
	res, err := tx.ExecContext(r.Context(), `UPDATE sys_user SET display_name=?,avatar=?,email=?,mobile=?,department_id=?,status=?,updated_at=? WHERE id=? AND deleted_at IS NULL`, in.DisplayName, in.Avatar, in.Email, in.Mobile, in.DepartmentID, in.Status, now, id)
	if err != nil {
		writeError(w, 409, 409, "User data conflicts with another account")
		return
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		writeError(w, 404, 404, "User not found")
		return
	}
	canAssign := actor.IsSuperAdmin || a.hasPermission(r.Context(), actor.ID, "system:user:assign-roles")
	if canAssign && in.RoleIDs != nil {
		if err := replaceUserRoles(r, tx, id, *in.RoleIDs); err != nil {
			writeError(w, 400, 400, "Invalid role assignment")
			return
		}
	}
	if err := tx.Commit(); err != nil {
		writeError(w, 503, 503, "Unable to update user")
		return
	}
	roles, roleIDs, _ := a.userRoleDTO(r.Context(), id)
	writeJSON(w, 200, envelope{Code: 0, Message: "success", Data: map[string]any{"id": id, "displayName": in.DisplayName, "avatar": in.Avatar, "email": in.Email, "mobile": in.Mobile, "departmentId": in.DepartmentID, "status": in.Status, "roleIds": roleIDs, "roles": roles, "updatedAt": now}})
}

func (a *app) deleteUser(w http.ResponseWriter, r *http.Request) {
	if _, ok := a.guard(w, r, "system:user:delete"); !ok {
		return
	}
	id, ok := pathID(r)
	if !ok {
		writeError(w, 400, 400, "Invalid user id")
		return
	}
	var super bool
	if err := a.db.QueryRowContext(r.Context(), `SELECT is_super_admin FROM sys_user WHERE id=? AND deleted_at IS NULL`, id).Scan(&super); err != nil {
		writeError(w, 404, 404, "User not found")
		return
	}
	if super {
		writeError(w, 403, 403, "Cannot delete a super administrator")
		return
	}
	now := time.Now().UnixMilli()
	tx, err := a.db.BeginTx(r.Context(), nil)
	if err != nil {
		writeError(w, 503, 503, "Unable to delete user")
		return
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(r.Context(), `UPDATE sys_user SET deleted_at=?,updated_at=? WHERE id=?`, now, now, id); err != nil {
		writeError(w, 503, 503, "Unable to delete user")
		return
	}
	if _, err = tx.ExecContext(r.Context(), `DELETE FROM sys_auth_session WHERE user_id=?`, id); err != nil {
		writeError(w, 503, 503, "Unable to revoke user sessions")
		return
	}
	if err = tx.Commit(); err != nil {
		writeError(w, 503, 503, "Unable to delete user")
		return
	}
	writeJSON(w, 200, envelope{Code: 0, Message: "success", Data: nil})
}

func replaceUserRoles(r *http.Request, tx *sql.Tx, userID uint64, roleIDs []uint64) error {
	if len(roleIDs) > 100 {
		return sql.ErrNoRows
	}
	if _, err := tx.ExecContext(r.Context(), `DELETE FROM sys_user_role WHERE user_id=?`, userID); err != nil {
		return err
	}
	seen := map[uint64]bool{}
	for _, id := range roleIDs {
		if seen[id] {
			continue
		}
		seen[id] = true
		var status uint8
		if err := tx.QueryRowContext(r.Context(), `SELECT status FROM sys_role WHERE id=? AND deleted_at IS NULL`, id).Scan(&status); err != nil || status != 1 {
			return sql.ErrNoRows
		}
		if _, err := tx.ExecContext(r.Context(), `INSERT INTO sys_user_role(user_id,role_id,created_at) VALUES(?,?,?)`, userID, id, time.Now().UnixMilli()); err != nil {
			return err
		}
	}
	return nil
}

func parseFilterID(q string) (*uint64, bool) {
	if q == "" {
		return nil, true
	}
	v, e := strconv.ParseUint(q, 10, 64)
	if e != nil || v == 0 {
		return nil, false
	}
	return &v, true
}
