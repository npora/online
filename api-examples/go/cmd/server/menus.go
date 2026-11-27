package main

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"strings"
	"time"
)

type menuInput struct {
	ParentID   *uint64         `json:"parentId"`
	Type       uint8           `json:"type"`
	Title      string          `json:"title"`
	Name       *string         `json:"name"`
	Path       *string         `json:"path"`
	Component  *string         `json:"component"`
	Redirect   *string         `json:"redirect"`
	RouteQuery json.RawMessage `json:"routeQuery"`
	ActiveMenu *string         `json:"activeMenu"`
	Icon       *string         `json:"icon"`
	Permission *string         `json:"permission"`
	Visible    bool            `json:"visible"`
	AlwaysShow bool            `json:"alwaysShow"`
	KeepAlive  bool            `json:"keepAlive"`
	FixedTab   bool            `json:"fixedTab"`
	SortOrder  uint32          `json:"sortOrder"`
	Status     uint8           `json:"status"`
}

func (a *app) menuCreate(w http.ResponseWriter, r *http.Request) {
	if _, ok := a.guard(w, r, "system:menu:create"); !ok {
		return
	}
	var in menuInput
	if !decodeJSON(w, r, &in) || !validateMenu(r, &in, 0, a.db) {
		writeError(w, 400, 400, "Invalid menu")
		return
	}
	now := time.Now().UnixMilli()
	var query any
	if len(in.RouteQuery) > 0 && string(in.RouteQuery) != "null" {
		query = string(in.RouteQuery)
	}
	res, err := a.db.ExecContext(r.Context(), `INSERT INTO sys_menu(parent_id,type,title,name,path,component,redirect,route_query,active_menu,icon,permission,visible,always_show,keep_alive,fixed_tab,sort_order,status,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`, in.ParentID, in.Type, strings.TrimSpace(in.Title), in.Name, in.Path, in.Component, in.Redirect, query, in.ActiveMenu, in.Icon, in.Permission, in.Visible, in.AlwaysShow, in.KeepAlive, in.FixedTab, in.SortOrder, in.Status, now, now)
	if err != nil {
		writeError(w, 409, 409, "Menu route name, path, or permission conflicts")
		return
	}
	id, _ := res.LastInsertId()
	out := menuObject(uint64(id), in, now, now)
	writeJSON(w, 201, envelope{Code: 0, Message: "success", Data: out})
}

func (a *app) menuUpdate(w http.ResponseWriter, r *http.Request) {
	if _, ok := a.guard(w, r, "system:menu:update"); !ok {
		return
	}
	id, ok := pathID(r)
	if !ok {
		writeError(w, 400, 400, "Invalid menu id")
		return
	}
	var in menuInput
	if !decodeJSON(w, r, &in) || !validateMenu(r, &in, id, a.db) {
		writeError(w, 400, 400, "Invalid menu")
		return
	}
	now := time.Now().UnixMilli()
	var query any
	if len(in.RouteQuery) > 0 && string(in.RouteQuery) != "null" {
		query = string(in.RouteQuery)
	}
	res, err := a.db.ExecContext(r.Context(), `UPDATE sys_menu SET parent_id=?,type=?,title=?,name=?,path=?,component=?,redirect=?,route_query=?,active_menu=?,icon=?,permission=?,visible=?,always_show=?,keep_alive=?,fixed_tab=?,sort_order=?,status=?,updated_at=? WHERE id=? AND deleted_at IS NULL`, in.ParentID, in.Type, strings.TrimSpace(in.Title), in.Name, in.Path, in.Component, in.Redirect, query, in.ActiveMenu, in.Icon, in.Permission, in.Visible, in.AlwaysShow, in.KeepAlive, in.FixedTab, in.SortOrder, in.Status, now, id)
	if err != nil {
		writeError(w, 409, 409, "Menu route name, path, or permission conflicts")
		return
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		writeError(w, 404, 404, "Menu not found")
		return
	}
	writeJSON(w, 200, envelope{Code: 0, Message: "success", Data: menuObject(id, in, 0, now)})
}

func (a *app) menuDelete(w http.ResponseWriter, r *http.Request) {
	if _, ok := a.guard(w, r, "system:menu:delete"); !ok {
		return
	}
	id, ok := pathID(r)
	if !ok {
		writeError(w, 400, 400, "Invalid menu id")
		return
	}
	var children int
	if err := a.db.QueryRowContext(r.Context(), `SELECT COUNT(*) FROM sys_menu WHERE parent_id=? AND deleted_at IS NULL`, id).Scan(&children); err != nil {
		writeError(w, 503, 503, "Unable to check menu children")
		return
	}
	if children > 0 {
		writeError(w, 409, 409, "Delete child nodes first")
		return
	}
	now := time.Now().UnixMilli()
	tx, err := a.db.BeginTx(r.Context(), nil)
	if err != nil {
		writeError(w, 503, 503, "Unable to delete menu")
		return
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(r.Context(), `DELETE FROM sys_role_menu WHERE menu_id=?`, id); err != nil {
		writeError(w, 503, 503, "Unable to delete menu")
		return
	}
	res, err := tx.ExecContext(r.Context(), `UPDATE sys_menu SET deleted_at=?,updated_at=? WHERE id=? AND deleted_at IS NULL`, now, now, id)
	if err != nil {
		writeError(w, 503, 503, "Unable to delete menu")
		return
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		writeError(w, 404, 404, "Menu not found")
		return
	}
	if err := tx.Commit(); err != nil {
		writeError(w, 503, 503, "Unable to delete menu")
		return
	}
	writeJSON(w, 200, envelope{Code: 0, Message: "success", Data: nil})
}

func validateMenu(r *http.Request, in *menuInput, id uint64, db *sql.DB) bool {
	if !validName(in.Title) || in.Type < 1 || in.Type > 3 || in.Status > 1 {
		return false
	}
	if len(in.RouteQuery) > 0 && string(in.RouteQuery) != "null" && !json.Valid(in.RouteQuery) {
		return false
	}
	if in.Type == 3 {
		if in.ParentID == nil || in.Permission == nil || !validCode(*in.Permission) || in.Name != nil || in.Path != nil || in.Component != nil || in.Redirect != nil {
			return false
		}
	} else {
		if in.Permission != nil {
			return false
		}
		if in.Type == 2 && (in.Name == nil || !validCode(*in.Name) || in.Path == nil || *in.Path == "" || in.Component == nil || !validComponent(*in.Component)) {
			return false
		}
		if in.Type == 1 && in.Component != nil {
			return false
		}
	}
	if in.ParentID != nil {
		if *in.ParentID == id {
			return false
		}
		var typ uint8
		if err := db.QueryRowContext(r.Context(), `SELECT type FROM sys_menu WHERE id=? AND deleted_at IS NULL`, *in.ParentID).Scan(&typ); err != nil {
			return false
		}
		if (in.Type == 1 || in.Type == 2) && typ != 1 {
			return false
		}
		if in.Type == 3 && typ != 2 {
			return false
		}
		if id > 0 {
			var count int
			if err := db.QueryRowContext(r.Context(), `WITH RECURSIVE descendants AS (SELECT id FROM sys_menu WHERE parent_id=? AND deleted_at IS NULL UNION ALL SELECT m.id FROM sys_menu m JOIN descendants d ON m.parent_id=d.id WHERE m.deleted_at IS NULL) SELECT COUNT(*) FROM descendants WHERE id=?`, id, *in.ParentID).Scan(&count); err != nil || count > 0 {
				return false
			}
		}
	}
	if in.Type == 3 && (in.Visible || in.AlwaysShow || in.KeepAlive || in.FixedTab) {
		return false
	}
	return true
}

func validComponent(s string) bool {
	return s != "" && !strings.Contains(s, "..") && !strings.HasPrefix(s, "/") && !strings.ContainsAny(s, "\\\x00") && len(s) <= 255
}
func menuObject(id uint64, in menuInput, created, updated int64) map[string]any {
	return map[string]any{"id": id, "parentId": in.ParentID, "type": in.Type, "title": strings.TrimSpace(in.Title), "name": in.Name, "path": in.Path, "component": in.Component, "redirect": in.Redirect, "routeQuery": jsonValue(in.RouteQuery), "activeMenu": in.ActiveMenu, "icon": in.Icon, "permission": in.Permission, "visible": in.Visible, "alwaysShow": in.AlwaysShow, "keepAlive": in.KeepAlive, "fixedTab": in.FixedTab, "sortOrder": in.SortOrder, "status": in.Status, "createdAt": created, "updatedAt": updated, "children": []any{}}
}
func jsonValue(v json.RawMessage) any {
	if len(v) == 0 || string(v) == "null" {
		return nil
	}
	var x any
	if json.Unmarshal(v, &x) != nil {
		return nil
	}
	return x
}
