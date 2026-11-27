package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"
)

type roleInput struct {
	Code        string  `json:"code"`
	Name        string  `json:"name"`
	Description *string `json:"description"`
	SortOrder   uint32  `json:"sortOrder"`
	Status      uint8   `json:"status"`
}

type departmentInput struct {
	ParentID  *uint64 `json:"parentId"`
	Name      string  `json:"name"`
	Code      *string `json:"code"`
	Leader    *string `json:"leader"`
	Phone     *string `json:"phone"`
	Email     *string `json:"email"`
	SortOrder uint32  `json:"sortOrder"`
	Status    uint8   `json:"status"`
}

func (a *app) guard(w http.ResponseWriter, r *http.Request, permission string) (authContext, bool) {
	user, _, err := a.authenticate(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, 401, "Authentication required")
		return authContext{}, false
	}
	if !user.IsSuperAdmin && !a.hasPermission(r.Context(), user.ID, permission) {
		writeError(w, http.StatusForbidden, 403, "Permission denied")
		return authContext{}, false
	}
	return user, true
}

func (a *app) registerSystem(mux *http.ServeMux) {
	mux.HandleFunc("GET /system/menus", a.menuList)
	mux.HandleFunc("POST /system/menus", a.menuCreate)
	mux.HandleFunc("PUT /system/menus/{id}", a.menuUpdate)
	mux.HandleFunc("DELETE /system/menus/{id}", a.menuDelete)
	mux.HandleFunc("GET /system/roles", a.rolesList)
	mux.HandleFunc("POST /system/roles", a.roleCreate)
	mux.HandleFunc("PUT /system/roles/{id}", a.roleUpdate)
	mux.HandleFunc("DELETE /system/roles/{id}", a.roleDelete)
	mux.HandleFunc("GET /system/roles/options", a.roleOptions)
	mux.HandleFunc("GET /system/roles/menu-options", a.menuOptions)
	mux.HandleFunc("GET /system/roles/{id}/menus", a.roleMenuList)
	mux.HandleFunc("PUT /system/roles/{id}/menus", a.roleMenuReplace)
	mux.HandleFunc("GET /system/departments", a.departmentList)
	mux.HandleFunc("GET /system/departments/options", a.departmentOptions)
	mux.HandleFunc("POST /system/departments", a.departmentCreate)
	mux.HandleFunc("PUT /system/departments/{id}", a.departmentUpdate)
	mux.HandleFunc("DELETE /system/departments/{id}", a.departmentDelete)
	mux.HandleFunc("GET /system/users/{userId}/sessions", a.adminUserSessions)
	mux.HandleFunc("DELETE /system/users/{userId}/sessions/{sessionId}", a.adminRevokeSession)
	mux.HandleFunc("GET /system/dict-types", a.dictTypesList)
	mux.HandleFunc("POST /system/dict-types", a.dictTypeCreate)
	mux.HandleFunc("PUT /system/dict-types/{id}", a.dictTypeUpdate)
	mux.HandleFunc("DELETE /system/dict-types/{id}", a.dictTypeDelete)
	mux.HandleFunc("GET /system/dict-types/{id}/items", a.dictItemsList)
	mux.HandleFunc("POST /system/dict-types/{id}/items", a.dictItemCreate)
	mux.HandleFunc("PUT /system/dict-types/{id}/items/{itemId}", a.dictItemUpdate)
	mux.HandleFunc("DELETE /system/dict-types/{id}/items/{itemId}", a.dictItemDelete)
	mux.HandleFunc("GET /system/dictionaries/{code}", a.dictOptions)
	mux.HandleFunc("GET /system/users/{userId}/permission-preview", a.permissionPreview)
	mux.HandleFunc("GET /system/audit-logs", a.auditLogs)
}

func (a *app) adminUserSessions(w http.ResponseWriter, r *http.Request) {
	actor, ok := a.guard(w, r, "system:user-session:list")
	if !ok {
		return
	}
	id, ok := parsePathID(r, "userId")
	if !ok {
		writeError(w, 400, 400, "Invalid user id")
		return
	}
	var super bool
	if err := a.db.QueryRowContext(r.Context(), `SELECT is_super_admin FROM sys_user WHERE id=? AND deleted_at IS NULL`, id).Scan(&super); err != nil {
		writeError(w, 404, 404, "User not found")
		return
	}
	if super && !actor.IsSuperAdmin {
		writeError(w, 403, 403, "Protected user")
		return
	}
	out, err := a.sessionList(r.Context(), id, r)
	if err != nil {
		writeError(w, 503, 503, "Unable to query sessions")
		return
	}
	writeJSON(w, 200, envelope{Code: 0, Message: "success", Data: out})
}
func (a *app) adminRevokeSession(w http.ResponseWriter, r *http.Request) {
	actor, ok := a.guard(w, r, "system:user-session:list")
	if !ok {
		return
	}
	if !actor.IsSuperAdmin && !a.hasPermission(r.Context(), actor.ID, "system:user-session:revoke") {
		writeError(w, 403, 403, "Permission denied")
		return
	}
	userID, ok := parsePathID(r, "userId")
	if !ok {
		writeError(w, 400, 400, "Invalid user id")
		return
	}
	sessionID := r.PathValue("sessionId")
	if !validSessionID(sessionID) {
		writeError(w, 400, 400, "Invalid session id")
		return
	}
	var super bool
	if err := a.db.QueryRowContext(r.Context(), `SELECT is_super_admin FROM sys_user WHERE id=? AND deleted_at IS NULL`, userID).Scan(&super); err != nil {
		writeError(w, 404, 404, "User not found")
		return
	}
	if super && !actor.IsSuperAdmin {
		writeError(w, 403, 403, "Protected user")
		return
	}
	tx, err := a.db.BeginTx(r.Context(), nil)
	if err != nil {
		writeError(w, 503, 503, "Unable to revoke session")
		return
	}
	defer tx.Rollback()
	res, err := tx.ExecContext(r.Context(), `DELETE FROM sys_auth_session WHERE user_id=? AND public_id=? AND expires_at>?`, userID, sessionID, time.Now().UnixMilli())
	if err != nil {
		writeError(w, 503, 503, "Unable to revoke session")
		return
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		writeError(w, 404, 404, "Session not found")
		return
	}
	if err := insertAudit(r.Context(), tx, actor.ID, userID, "user", "session_revoke", 200, time.Now().UnixMilli()); err != nil {
		writeError(w, 503, 503, "Unable to write audit log")
		return
	}
	if err := tx.Commit(); err != nil {
		writeError(w, 503, 503, "Unable to revoke session")
		return
	}
	writeJSON(w, 200, envelope{Code: 0, Message: "success", Data: map[string]int64{"revoked": n}})
}

func (a *app) menuList(w http.ResponseWriter, r *http.Request) {
	if _, ok := a.guard(w, r, "system:menu:list"); !ok {
		return
	}
	menus, err := a.menuTree(r.Context(), true)
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, 503, "Unable to query menus")
		return
	}
	writeJSON(w, http.StatusOK, envelope{Code: 0, Message: "success", Data: menus})
}

func (a *app) rolesList(w http.ResponseWriter, r *http.Request) {
	if _, ok := a.guard(w, r, "system:role:list"); !ok {
		return
	}
	page, size := pageQuery(r)
	keyword := strings.TrimSpace(r.URL.Query().Get("keyword"))
	status := r.URL.Query().Get("status")
	where := []string{"r.deleted_at IS NULL"}
	args := []any{}
	if keyword != "" {
		where = append(where, "(r.code LIKE ? OR r.name LIKE ?)")
		args = append(args, "%"+keyword+"%", "%"+keyword+"%")
	}
	if status == "0" || status == "1" {
		where = append(where, "r.status=?")
		args = append(args, status)
	}
	clause := strings.Join(where, " AND ")
	var total int64
	if err := a.db.QueryRowContext(r.Context(), "SELECT COUNT(*) FROM sys_role r WHERE "+clause, args...).Scan(&total); err != nil {
		writeError(w, 503, 503, "Unable to query roles")
		return
	}
	args = append(args, size, (page-1)*size)
	rows, err := a.db.QueryContext(r.Context(), `SELECT r.id,r.code,r.name,r.description,r.sort_order,r.status,r.created_at,r.updated_at,
		(SELECT COUNT(DISTINCT ur.user_id) FROM sys_user_role ur JOIN sys_user u ON u.id=ur.user_id AND u.deleted_at IS NULL WHERE ur.role_id=r.id)
		FROM sys_role r WHERE `+clause+` ORDER BY r.sort_order,r.id LIMIT ? OFFSET ?`, args...)
	if err != nil {
		writeError(w, 503, 503, "Unable to query roles")
		return
	}
	defer rows.Close()
	list := []map[string]any{}
	for rows.Next() {
		var id uint64
		var code, name string
		var desc sql.NullString
		var sort uint32
		var status uint8
		var created, updated int64
		var count int64
		if err := rows.Scan(&id, &code, &name, &desc, &sort, &status, &created, &updated, &count); err != nil {
			writeError(w, 503, 503, "Unable to query roles")
			return
		}
		list = append(list, map[string]any{"id": id, "code": code, "name": name, "description": nullString(desc), "sortOrder": sort, "status": status, "createdAt": created, "updatedAt": updated, "userCount": count})
	}
	if rows.Err() != nil {
		writeError(w, 503, 503, "Unable to query roles")
		return
	}
	writeJSON(w, 200, envelope{Code: 0, Message: "success", Data: map[string]any{"list": list, "total": total}})
}

func (a *app) roleCreate(w http.ResponseWriter, r *http.Request) {
	if _, ok := a.guard(w, r, "system:role:create"); !ok {
		return
	}
	var in roleInput
	if !decodeJSON(w, r, &in) || !validCode(in.Code) || !validName(in.Name) || in.Status > 1 {
		writeError(w, 400, 400, "Invalid role")
		return
	}
	now := time.Now().UnixMilli()
	res, err := a.db.ExecContext(r.Context(), `INSERT INTO sys_role(code,name,description,sort_order,status,created_at,updated_at) VALUES(?,?,?,?,?,?,?)`, in.Code, in.Name, in.Description, in.SortOrder, in.Status, now, now)
	if err != nil {
		writeError(w, 409, 409, "Role code already exists or data is invalid")
		return
	}
	id, _ := res.LastInsertId()
	writeJSON(w, 201, envelope{Code: 0, Message: "success", Data: map[string]any{"id": id, "code": in.Code, "name": in.Name, "description": in.Description, "sortOrder": in.SortOrder, "status": in.Status, "createdAt": now, "updatedAt": now, "userCount": 0}})
}

func (a *app) roleUpdate(w http.ResponseWriter, r *http.Request) {
	if _, ok := a.guard(w, r, "system:role:update"); !ok {
		return
	}
	id, ok := pathID(r)
	if !ok {
		writeError(w, 400, 400, "Invalid role id")
		return
	}
	var in roleInput
	if !decodeJSON(w, r, &in) || !validCode(in.Code) || !validName(in.Name) || in.Status > 1 {
		writeError(w, 400, 400, "Invalid role")
		return
	}
	now := time.Now().UnixMilli()
	res, err := a.db.ExecContext(r.Context(), `UPDATE sys_role SET code=?,name=?,description=?,sort_order=?,status=?,updated_at=? WHERE id=? AND deleted_at IS NULL`, in.Code, in.Name, in.Description, in.SortOrder, in.Status, now, id)
	if err != nil {
		writeError(w, 409, 409, "Role code already exists or data is invalid")
		return
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		writeError(w, 404, 404, "Role not found")
		return
	}
	writeJSON(w, 200, envelope{Code: 0, Message: "success", Data: map[string]any{"id": id, "code": in.Code, "name": in.Name, "description": in.Description, "sortOrder": in.SortOrder, "status": in.Status, "updatedAt": now}})
}

func (a *app) roleDelete(w http.ResponseWriter, r *http.Request) {
	if _, ok := a.guard(w, r, "system:role:delete"); !ok {
		return
	}
	id, ok := pathID(r)
	if !ok {
		writeError(w, 400, 400, "Invalid role id")
		return
	}
	var count int
	if err := a.db.QueryRowContext(r.Context(), `SELECT COUNT(*) FROM sys_user_role ur JOIN sys_user u ON u.id=ur.user_id WHERE ur.role_id=? AND u.deleted_at IS NULL`, id).Scan(&count); err != nil {
		writeError(w, 503, 503, "Unable to delete role")
		return
	}
	if count > 0 {
		writeError(w, 409, 409, "Role is assigned to users")
		return
	}
	res, err := a.db.ExecContext(r.Context(), `UPDATE sys_role SET deleted_at=?,updated_at=? WHERE id=? AND deleted_at IS NULL`, time.Now().UnixMilli(), time.Now().UnixMilli(), id)
	if err != nil {
		writeError(w, 503, 503, "Unable to delete role")
		return
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		writeError(w, 404, 404, "Role not found")
		return
	}
	writeJSON(w, 200, envelope{Code: 0, Message: "success", Data: nil})
}

func (a *app) roleOptions(w http.ResponseWriter, r *http.Request) {
	user, _, err := a.authenticate(r)
	if err != nil {
		writeError(w, 401, 401, "Authentication required")
		return
	}
	if !user.IsSuperAdmin && !a.anyPermission(r.Context(), user.ID, "system:user:list", "system:user:create", "system:user:update") {
		writeError(w, 403, 403, "Permission denied")
		return
	}
	rows, err := a.db.QueryContext(r.Context(), `SELECT id,code,name,status FROM sys_role WHERE deleted_at IS NULL ORDER BY sort_order,id`)
	if err != nil {
		writeError(w, 503, 503, "Unable to query roles")
		return
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var id uint64
		var code, name string
		var status uint8
		if rows.Scan(&id, &code, &name, &status) != nil {
			writeError(w, 503, 503, "Unable to query roles")
			return
		}
		out = append(out, map[string]any{"id": id, "code": code, "name": name, "status": status})
	}
	writeJSON(w, 200, envelope{Code: 0, Message: "success", Data: out})
}

func (a *app) roleMenuList(w http.ResponseWriter, r *http.Request) {
	_, ok := a.guard(w, r, "system:role:grant")
	if !ok {
		return
	}
	id, ok := pathID(r)
	if !ok {
		writeError(w, 400, 400, "Invalid role id")
		return
	}
	rows, err := a.db.QueryContext(r.Context(), `SELECT menu_id FROM sys_role_menu WHERE role_id=? ORDER BY menu_id`, id)
	if err != nil {
		writeError(w, 503, 503, "Unable to query grants")
		return
	}
	defer rows.Close()
	out := []uint64{}
	for rows.Next() {
		var x uint64
		if rows.Scan(&x) != nil {
			writeError(w, 503, 503, "Unable to query grants")
			return
		}
		out = append(out, x)
	}
	writeJSON(w, 200, envelope{Code: 0, Message: "success", Data: out})
}

func (a *app) roleMenuReplace(w http.ResponseWriter, r *http.Request) {
	actor, ok := a.guard(w, r, "system:role:grant")
	if !ok {
		return
	}
	id, ok := pathID(r)
	if !ok {
		writeError(w, 400, 400, "Invalid role id")
		return
	}
	var in struct {
		MenuIDs []uint64 `json:"menuIds"`
	}
	if !decodeJSON(w, r, &in) {
		writeError(w, 400, 400, "Invalid grants")
		return
	}
	tx, err := a.db.BeginTx(r.Context(), nil)
	if err != nil {
		writeError(w, 503, 503, "Unable to save grants")
		return
	}
	defer tx.Rollback()
	var exists int
	if tx.QueryRowContext(r.Context(), `SELECT COUNT(*) FROM sys_role WHERE id=? AND deleted_at IS NULL`, id).Scan(&exists) != nil || exists == 0 {
		writeError(w, 404, 404, "Role not found")
		return
	}
	set := map[uint64]struct{}{}
	for _, mid := range in.MenuIDs {
		for current := mid; current > 0; {
			var parent sql.NullInt64
			var typ int
			err := tx.QueryRowContext(r.Context(), `SELECT parent_id,type FROM sys_menu WHERE id=? AND status=1 AND deleted_at IS NULL`, current).Scan(&parent, &typ)
			if err != nil {
				writeError(w, 400, 400, "Invalid menu id")
				return
			}
			set[current] = struct{}{}
			if !parent.Valid {
				break
			}
			current = uint64(parent.Int64)
		}
	}
	if _, err := tx.ExecContext(r.Context(), `DELETE FROM sys_role_menu WHERE role_id=?`, id); err != nil {
		writeError(w, 503, 503, "Unable to save grants")
		return
	}
	for mid := range set {
		if _, err := tx.ExecContext(r.Context(), `INSERT INTO sys_role_menu(role_id,menu_id) VALUES(?,?)`, id, mid); err != nil {
			writeError(w, 503, 503, "Unable to save grants")
			return
		}
	}
	if err := insertAudit(r.Context(), tx, actor.ID, id, "role", "role_grant", 200, time.Now().UnixMilli()); err != nil {
		writeError(w, 503, 503, "Unable to write audit log")
		return
	}
	if err := tx.Commit(); err != nil {
		writeError(w, 503, 503, "Unable to save grants")
		return
	}
	out := make([]uint64, 0, len(set))
	for x := range set {
		out = append(out, x)
	}
	writeJSON(w, 200, envelope{Code: 0, Message: "success", Data: out})
}

func (a *app) menuOptions(w http.ResponseWriter, r *http.Request) {
	if _, ok := a.guard(w, r, "system:role:grant"); !ok {
		return
	}
	out, err := a.menuTree(r.Context(), false)
	if err != nil {
		writeError(w, 503, 503, "Unable to query menus")
		return
	}
	writeJSON(w, 200, envelope{Code: 0, Message: "success", Data: out})
}

func (a *app) menuTree(ctx context.Context, includeButtons bool) ([]map[string]any, error) {
	filter := "m.type IN (1,2)"
	if includeButtons {
		filter = "m.type IN (1,2,3)"
	}
	rows, err := a.db.QueryContext(ctx, `SELECT m.id,m.parent_id,m.type,m.title,m.name,m.path,m.component,m.redirect,m.route_query,m.active_menu,m.icon,m.permission,m.visible,m.always_show,m.keep_alive,m.fixed_tab,m.sort_order,m.status,m.created_at,m.updated_at FROM sys_menu m WHERE m.deleted_at IS NULL AND `+filter+` ORDER BY m.sort_order,m.id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	type node struct {
		id     uint64
		parent sql.NullInt64
		obj    map[string]any
	}
	byID := map[uint64]*node{}
	list := []*node{}
	for rows.Next() {
		var id uint64
		var parent sql.NullInt64
		var typ int
		var title string
		var name, path, component, redirect, active, icon, permission sql.NullString
		var query []byte
		var visible, always, keep, fixed bool
		var sort uint32
		var status uint8
		var created, updated int64
		if err := rows.Scan(&id, &parent, &typ, &title, &name, &path, &component, &redirect, &query, &active, &icon, &permission, &visible, &always, &keep, &fixed, &sort, &status, &created, &updated); err != nil {
			return nil, err
		}
		meta := map[string]any{"title": title, "hidden": !visible, "alwaysShow": always, "keepAlive": keep, "fixedTab": fixed}
		for k, v := range map[string]sql.NullString{"icon": icon, "activeMenu": active} {
			if v.Valid {
				meta[k] = v.String
			}
		}
		obj := map[string]any{"id": id, "type": typ, "title": title, "visible": visible, "alwaysShow": always, "keepAlive": keep, "fixedTab": fixed, "sortOrder": sort, "status": status, "createdAt": created, "updatedAt": updated, "meta": meta, "children": []map[string]any{}}
		for k, v := range map[string]sql.NullString{"name": name, "path": path, "component": component, "redirect": redirect, "activeMenu": active, "icon": icon, "permission": permission} {
			if v.Valid {
				obj[k] = v.String
			}
		}
		if len(query) > 0 {
			var q any
			if json.Unmarshal(query, &q) == nil {
				obj["routeQuery"] = q
			}
		} else {
			obj["routeQuery"] = nil
		}
		byID[id] = &node{id, parent, obj}
		list = append(list, byID[id])
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	roots := []map[string]any{}
	for _, n := range list {
		if n.parent.Valid && byID[uint64(n.parent.Int64)] != nil {
			p := byID[uint64(n.parent.Int64)].obj
			p["children"] = append(p["children"].([]map[string]any), n.obj)
		} else {
			roots = append(roots, n.obj)
		}
	}
	return roots, nil
}

func (a *app) departmentList(w http.ResponseWriter, r *http.Request) {
	if _, ok := a.guard(w, r, "system:department:list"); !ok {
		return
	}
	out, err := a.departmentTree(r.Context(), true)
	if err != nil {
		writeError(w, 503, 503, "Unable to query departments")
		return
	}
	writeJSON(w, 200, envelope{Code: 0, Message: "success", Data: out})
}
func (a *app) departmentOptions(w http.ResponseWriter, r *http.Request) {
	user, _, err := a.authenticate(r)
	if err != nil {
		writeError(w, 401, 401, "Authentication required")
		return
	}
	if !user.IsSuperAdmin && !a.anyPermission(r.Context(), user.ID, "system:user:list", "system:user:create", "system:user:update", "system:department:list", "system:department:create", "system:department:update") {
		writeError(w, 403, 403, "Permission denied")
		return
	}
	out, err := a.departmentTree(r.Context(), false)
	if err != nil {
		writeError(w, 503, 503, "Unable to query departments")
		return
	}
	writeJSON(w, 200, envelope{Code: 0, Message: "success", Data: out})
}
func (a *app) departmentTree(ctx context.Context, withCount bool) ([]map[string]any, error) {
	rows, err := a.db.QueryContext(ctx, `SELECT d.id,d.parent_id,d.name,d.code,d.leader,d.phone,d.email,d.sort_order,d.status,d.created_at,d.updated_at,`+func() string {
		if withCount {
			return `(SELECT COUNT(*) FROM sys_user u WHERE u.department_id=d.id AND u.deleted_at IS NULL)`
		}
		return `0`
	}()+` FROM sys_department d WHERE d.deleted_at IS NULL ORDER BY d.sort_order,d.id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	type node struct {
		id     uint64
		parent sql.NullInt64
		obj    map[string]any
	}
	by := map[uint64]*node{}
	list := []*node{}
	for rows.Next() {
		var id uint64
		var parent sql.NullInt64
		var name string
		var code, leader, phone, email sql.NullString
		var sort uint32
		var status uint8
		var created, updated int64
		var count int64
		if err := rows.Scan(&id, &parent, &name, &code, &leader, &phone, &email, &sort, &status, &created, &updated, &count); err != nil {
			return nil, err
		}
		obj := map[string]any{"id": id, "parentId": nullInt(parent), "name": name, "code": nullString(code), "leader": nullString(leader), "phone": nullString(phone), "email": nullString(email), "sortOrder": sort, "status": status, "createdAt": created, "updatedAt": updated, "children": []map[string]any{}}
		if withCount {
			obj["userCount"] = count
		}
		n := &node{id, parent, obj}
		by[id] = n
		list = append(list, n)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	roots := []map[string]any{}
	for _, n := range list {
		if n.parent.Valid && by[uint64(n.parent.Int64)] != nil {
			p := by[uint64(n.parent.Int64)].obj
			p["children"] = append(p["children"].([]map[string]any), n.obj)
		} else {
			roots = append(roots, n.obj)
		}
	}
	return roots, nil
}

func (a *app) departmentCreate(w http.ResponseWriter, r *http.Request) {
	if _, ok := a.guard(w, r, "system:department:create"); !ok {
		return
	}
	var in departmentInput
	if !decodeJSON(w, r, &in) || !validName(in.Name) || in.Status > 1 {
		writeError(w, 400, 400, "Invalid department")
		return
	}
	if in.ParentID != nil {
		var status uint8
		if err := a.db.QueryRowContext(r.Context(), `SELECT status FROM sys_department WHERE id=? AND deleted_at IS NULL`, *in.ParentID).Scan(&status); err != nil || status != 1 {
			writeError(w, 400, 400, "Invalid parent department")
			return
		}
	}
	now := time.Now().UnixMilli()
	res, err := a.db.ExecContext(r.Context(), `INSERT INTO sys_department(parent_id,name,code,leader,phone,email,sort_order,status,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?,?,?)`, in.ParentID, in.Name, in.Code, in.Leader, in.Phone, in.Email, in.SortOrder, in.Status, now, now)
	if err != nil {
		writeError(w, 409, 409, "Department code already exists or data is invalid")
		return
	}
	id, _ := res.LastInsertId()
	out := map[string]any{"id": id, "parentId": in.ParentID, "name": in.Name, "code": in.Code, "leader": in.Leader, "phone": in.Phone, "email": in.Email, "sortOrder": in.SortOrder, "status": in.Status, "createdAt": now, "updatedAt": now, "userCount": 0, "children": []any{}}
	writeJSON(w, 201, envelope{Code: 0, Message: "success", Data: out})
}
func (a *app) departmentUpdate(w http.ResponseWriter, r *http.Request) {
	if _, ok := a.guard(w, r, "system:department:update"); !ok {
		return
	}
	id, ok := pathID(r)
	if !ok {
		writeError(w, 400, 400, "Invalid department id")
		return
	}
	var in departmentInput
	if !decodeJSON(w, r, &in) || !validName(in.Name) || in.Status > 1 {
		writeError(w, 400, 400, "Invalid department")
		return
	}
	if in.ParentID != nil {
		if *in.ParentID == id {
			writeError(w, 400, 400, "Invalid parent department")
			return
		}
		var count int
		if err := a.db.QueryRowContext(r.Context(), `WITH RECURSIVE descendants AS (SELECT id FROM sys_department WHERE parent_id=? AND deleted_at IS NULL UNION ALL SELECT d.id FROM sys_department d JOIN descendants x ON d.parent_id=x.id WHERE d.deleted_at IS NULL) SELECT COUNT(*) FROM descendants WHERE id=?`, id, *in.ParentID).Scan(&count); err != nil || count > 0 {
			writeError(w, 400, 400, "Parent cannot be a descendant")
			return
		}
	}
	now := time.Now().UnixMilli()
	res, err := a.db.ExecContext(r.Context(), `UPDATE sys_department SET parent_id=?,name=?,code=?,leader=?,phone=?,email=?,sort_order=?,status=?,updated_at=? WHERE id=? AND deleted_at IS NULL`, in.ParentID, in.Name, in.Code, in.Leader, in.Phone, in.Email, in.SortOrder, in.Status, now, id)
	if err != nil {
		writeError(w, 409, 409, "Department code already exists or data is invalid")
		return
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		writeError(w, 404, 404, "Department not found")
		return
	}
	writeJSON(w, 200, envelope{Code: 0, Message: "success", Data: map[string]any{"id": id, "parentId": in.ParentID, "name": in.Name, "code": in.Code, "leader": in.Leader, "phone": in.Phone, "email": in.Email, "sortOrder": in.SortOrder, "status": in.Status, "updatedAt": now}})
}
func (a *app) departmentDelete(w http.ResponseWriter, r *http.Request) {
	if _, ok := a.guard(w, r, "system:department:delete"); !ok {
		return
	}
	id, ok := pathID(r)
	if !ok {
		writeError(w, 400, 400, "Invalid department id")
		return
	}
	var count int
	if err := a.db.QueryRowContext(r.Context(), `SELECT (SELECT COUNT(*) FROM sys_department WHERE parent_id=? AND deleted_at IS NULL)+(SELECT COUNT(*) FROM sys_user WHERE department_id=? AND deleted_at IS NULL)`, id, id).Scan(&count); err != nil {
		writeError(w, 503, 503, "Unable to delete department")
		return
	}
	if count > 0 {
		writeError(w, 409, 409, "Department has children or users")
		return
	}
	now := time.Now().UnixMilli()
	res, err := a.db.ExecContext(r.Context(), `UPDATE sys_department SET deleted_at=?,updated_at=? WHERE id=? AND deleted_at IS NULL`, now, now, id)
	if err != nil {
		writeError(w, 503, 503, "Unable to delete department")
		return
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		writeError(w, 404, 404, "Department not found")
		return
	}
	writeJSON(w, 200, envelope{Code: 0, Message: "success", Data: nil})
}

func (a *app) anyPermission(ctx context.Context, id uint64, permissions ...string) bool {
	for _, p := range permissions {
		if a.hasPermission(ctx, id, p) {
			return true
		}
	}
	return false
}
func pageQuery(r *http.Request) (int64, int64) {
	page, size := int64(1), int64(20)
	if x, e := strconv.ParseInt(r.URL.Query().Get("page"), 10, 64); e == nil && x > 0 {
		page = x
	}
	if x, e := strconv.ParseInt(r.URL.Query().Get("pageSize"), 10, 64); e == nil && x > 0 {
		size = x
	}
	if size > 100 {
		size = 100
	}
	return page, size
}
func pathID(r *http.Request) (uint64, bool) {
	id, err := strconv.ParseUint(r.PathValue("id"), 10, 64)
	return id, err == nil && id > 0
}
func validName(s string) bool { return len(strings.TrimSpace(s)) > 0 && len([]rune(s)) <= 100 }
func validCode(s string) bool {
	if len(s) == 0 || len(s) > 64 {
		return false
	}
	for i, c := range s {
		if (i == 0 && ((c < 'a' || c > 'z') && (c < 'A' || c > 'Z'))) || (i > 0 && ((c < 'a' || c > 'z') && (c < 'A' || c > 'Z') && (c < '0' || c > '9') && c != '_' && c != '-' && c != '.')) {
			return false
		}
	}
	return true
}
