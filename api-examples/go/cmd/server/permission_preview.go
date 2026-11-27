package main

import (
	"database/sql"
	"net/http"
)

func (a *app) permissionPreview(w http.ResponseWriter, r *http.Request) {
	if _, ok := a.guard(w, r, "system:user:permission-preview"); !ok {
		return
	}
	userID, ok := parsePathID(r, "userId")
	if !ok {
		writeError(w, 400, 400, "Invalid user id")
		return
	}
	var active, super bool
	if err := a.db.QueryRowContext(r.Context(), `SELECT status=1,is_super_admin=1 FROM sys_user WHERE id=? AND deleted_at IS NULL`, userID).Scan(&active, &super); err != nil {
		writeError(w, 404, 404, "User not found")
		return
	}
	roles := []map[string]any{}
	rows, err := a.db.QueryContext(r.Context(), `SELECT r.id,r.name FROM sys_user_role ur JOIN sys_role r ON r.id=ur.role_id WHERE ur.user_id=? AND r.status=1 AND r.deleted_at IS NULL ORDER BY r.sort_order,r.id`, userID)
	if err != nil {
		writeError(w, 503, 503, "Unable to query roles")
		return
	}
	for rows.Next() {
		var id uint64
		var name string
		if rows.Scan(&id, &name) != nil {
			rows.Close()
			writeError(w, 503, 503, "Unable to query roles")
			return
		}
		roles = append(roles, map[string]any{"id": id, "name": name})
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		writeError(w, 503, 503, "Unable to query roles")
		return
	}
	rows.Close()
	entries := []map[string]any{}
	if active {
		rows, err = a.db.QueryContext(r.Context(), `SELECT m.id,m.parent_id,m.type,m.title,m.permission,ur.role_id FROM sys_menu m LEFT JOIN sys_role_menu rm ON rm.menu_id=m.id LEFT JOIN sys_role r ON r.id=rm.role_id AND r.status=1 AND r.deleted_at IS NULL LEFT JOIN sys_user_role ur ON ur.role_id=r.id AND ur.user_id=? WHERE m.deleted_at IS NULL AND m.status=1 AND ( ? OR ur.role_id IS NOT NULL ) ORDER BY m.sort_order,m.id,ur.role_id`, userID, super && active)
		if err != nil {
			writeError(w, 503, 503, "Unable to query permissions")
			return
		}
		type entry struct {
			id         uint64
			parent     sql.NullInt64
			typ        uint8
			title      string
			permission sql.NullString
			roleIDs    []uint64
		}
		by := map[uint64]*entry{}
		order := []uint64{}
		for rows.Next() {
			var id uint64
			var parent sql.NullInt64
			var typ uint8
			var title string
			var permission sql.NullString
			var role sql.NullInt64
			if err := rows.Scan(&id, &parent, &typ, &title, &permission, &role); err != nil {
				rows.Close()
				writeError(w, 503, 503, "Unable to query permissions")
				return
			}
			e := by[id]
			if e == nil {
				e = &entry{id: id, parent: parent, typ: typ, title: title, permission: permission, roleIDs: []uint64{}}
				by[id] = e
				order = append(order, id)
			}
			if role.Valid {
				e.roleIDs = append(e.roleIDs, uint64(role.Int64))
			}
		}
		rows.Close()
		for _, id := range order {
			e := by[id]
			kind := "directory"
			if e.typ == 2 {
				kind = "menu"
			} else if e.typ == 3 {
				kind = "permission"
			}
			var parent any
			if e.parent.Valid {
				parent = e.parent.Int64
			}
			var permission any
			if e.permission.Valid {
				permission = e.permission.String
			}
			source := "role"
			if super {
				source = "superAdmin"
			}
			entries = append(entries, map[string]any{"id": e.id, "parentId": parent, "type": kind, "title": e.title, "permission": permission, "roleIds": e.roleIDs, "source": source})
		}
	}
	writeJSON(w, 200, envelope{Code: 0, Message: "success", Data: map[string]any{"userId": userID, "isSuperAdmin": super, "roles": roles, "entries": entries}})
}
