package main

import (
	"database/sql"
	"net/http"
	"strconv"
	"strings"
	"time"
)

type dictTypeInput struct {
	Code      string  `json:"code"`
	Name      string  `json:"name"`
	Remark    *string `json:"remark"`
	SortOrder uint32  `json:"sortOrder"`
	Status    uint8   `json:"status"`
}
type dictItemInput struct {
	Label     string  `json:"label"`
	Value     string  `json:"value"`
	TagType   string  `json:"tagType"`
	Remark    *string `json:"remark"`
	SortOrder uint32  `json:"sortOrder"`
	Status    uint8   `json:"status"`
}

func (a *app) dictTypesList(w http.ResponseWriter, r *http.Request) {
	if _, ok := a.guard(w, r, "system:dict:list"); !ok {
		return
	}
	p, s := pageQuery(r)
	where := []string{"deleted_at IS NULL"}
	args := []any{}
	kw := strings.TrimSpace(r.URL.Query().Get("keyword"))
	if kw != "" {
		where = append(where, "(code LIKE ? OR name LIKE ?)")
		args = append(args, "%"+kw+"%", "%"+kw+"%")
	}
	if st := r.URL.Query().Get("status"); st == "0" || st == "1" {
		where = append(where, "status=?")
		args = append(args, st)
	}
	clause := strings.Join(where, " AND ")
	var total int64
	if err := a.db.QueryRowContext(r.Context(), `SELECT COUNT(*) FROM sys_dict_type WHERE `+clause, args...).Scan(&total); err != nil {
		writeError(w, 503, 503, "Unable to query dictionary types")
		return
	}
	args = append(args, s, (p-1)*s)
	rows, err := a.db.QueryContext(r.Context(), `SELECT t.id,t.code,t.name,t.remark,t.sort_order,t.status,t.created_at,t.updated_at,(SELECT COUNT(*) FROM sys_dict_item i WHERE i.dict_type_id=t.id AND i.deleted_at IS NULL) FROM sys_dict_type t WHERE `+clause+` ORDER BY t.sort_order,t.id LIMIT ? OFFSET ?`, args...)
	if err != nil {
		writeError(w, 503, 503, "Unable to query dictionary types")
		return
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var id uint64
		var code, name string
		var remark sql.NullString
		var sort uint32
		var status uint8
		var created, updated int64
		var count int64
		if rows.Scan(&id, &code, &name, &remark, &sort, &status, &created, &updated, &count) != nil {
			writeError(w, 503, 503, "Unable to query dictionary types")
			return
		}
		out = append(out, map[string]any{"id": id, "code": code, "name": name, "remark": nullString(remark), "sortOrder": sort, "status": status, "createdAt": created, "updatedAt": updated, "itemCount": count})
	}
	writeJSON(w, 200, envelope{Code: 0, Message: "success", Data: map[string]any{"list": out, "total": total}})
}
func (a *app) dictTypeCreate(w http.ResponseWriter, r *http.Request) {
	if _, ok := a.guard(w, r, "system:dict:create"); !ok {
		return
	}
	var in dictTypeInput
	if !decodeJSON(w, r, &in) || !validDictCode(in.Code) || !validName(in.Name) || in.Status > 1 {
		writeError(w, 400, 400, "Invalid dictionary type")
		return
	}
	now := time.Now().UnixMilli()
	res, err := a.db.ExecContext(r.Context(), `INSERT INTO sys_dict_type(code,name,remark,sort_order,status,created_at,updated_at) VALUES(?,?,?,?,?,?,?)`, in.Code, in.Name, in.Remark, in.SortOrder, in.Status, now, now)
	if err != nil {
		writeError(w, 409, 409, "Dictionary code already exists")
		return
	}
	id, _ := res.LastInsertId()
	writeJSON(w, 201, envelope{Code: 0, Message: "success", Data: map[string]any{"id": id, "code": in.Code, "name": in.Name, "remark": in.Remark, "sortOrder": in.SortOrder, "status": in.Status, "createdAt": now, "updatedAt": now, "itemCount": 0}})
}
func (a *app) dictTypeUpdate(w http.ResponseWriter, r *http.Request) {
	if _, ok := a.guard(w, r, "system:dict:update"); !ok {
		return
	}
	id, ok := pathID(r)
	if !ok {
		writeError(w, 400, 400, "Invalid dictionary id")
		return
	}
	var in dictTypeInput
	if !decodeJSON(w, r, &in) || !validDictCode(in.Code) || !validName(in.Name) || in.Status > 1 {
		writeError(w, 400, 400, "Invalid dictionary type")
		return
	}
	var originalCode string
	if err := a.db.QueryRowContext(r.Context(), `SELECT code FROM sys_dict_type WHERE id=? AND deleted_at IS NULL`, id).Scan(&originalCode); err != nil {
		writeError(w, 404, 404, "Dictionary not found")
		return
	}
	if in.Code != originalCode {
		writeError(w, 400, 400, "Dictionary code cannot be changed")
		return
	}
	now := time.Now().UnixMilli()
	res, err := a.db.ExecContext(r.Context(), `UPDATE sys_dict_type SET code=?,name=?,remark=?,sort_order=?,status=?,updated_at=? WHERE id=? AND deleted_at IS NULL`, in.Code, in.Name, in.Remark, in.SortOrder, in.Status, now, id)
	if err != nil {
		writeError(w, 409, 409, "Dictionary code already exists")
		return
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		writeError(w, 404, 404, "Dictionary not found")
		return
	}
	writeJSON(w, 200, envelope{Code: 0, Message: "success", Data: map[string]any{"id": id, "code": in.Code, "name": in.Name, "remark": in.Remark, "sortOrder": in.SortOrder, "status": in.Status, "updatedAt": now}})
}
func (a *app) dictTypeDelete(w http.ResponseWriter, r *http.Request) {
	if _, ok := a.guard(w, r, "system:dict:delete"); !ok {
		return
	}
	id, ok := pathID(r)
	if !ok {
		writeError(w, 400, 400, "Invalid dictionary id")
		return
	}
	var n int
	if err := a.db.QueryRowContext(r.Context(), `SELECT COUNT(*) FROM sys_dict_item WHERE dict_type_id=? AND deleted_at IS NULL`, id).Scan(&n); err != nil {
		writeError(w, 503, 503, "Unable to delete dictionary")
		return
	}
	if n > 0 {
		writeError(w, 409, 409, "Dictionary still contains items")
		return
	}
	now := time.Now().UnixMilli()
	res, err := a.db.ExecContext(r.Context(), `UPDATE sys_dict_type SET deleted_at=?,updated_at=? WHERE id=? AND deleted_at IS NULL`, now, now, id)
	if err != nil {
		writeError(w, 503, 503, "Unable to delete dictionary")
		return
	}
	affected, _ := res.RowsAffected()
	if affected == 0 {
		writeError(w, 404, 404, "Dictionary not found")
		return
	}
	writeJSON(w, 200, envelope{Code: 0, Message: "success", Data: nil})
}
func (a *app) dictItemsList(w http.ResponseWriter, r *http.Request) {
	if _, ok := a.guard(w, r, "system:dict-item:list"); !ok {
		return
	}
	id, ok := pathID(r)
	if !ok {
		writeError(w, 400, 400, "Invalid dictionary id")
		return
	}
	p, s := pageQuery(r)
	where := []string{"dict_type_id=?", "deleted_at IS NULL"}
	args := []any{id}
	if kw := strings.TrimSpace(r.URL.Query().Get("keyword")); kw != "" {
		where = append(where, "(label LIKE ? OR value LIKE ?)")
		args = append(args, "%"+kw+"%", "%"+kw+"%")
	}
	if st := r.URL.Query().Get("status"); st == "0" || st == "1" {
		where = append(where, "status=?")
		args = append(args, st)
	}
	clause := strings.Join(where, " AND ")
	var total int64
	if err := a.db.QueryRowContext(r.Context(), `SELECT COUNT(*) FROM sys_dict_item WHERE `+clause, args...).Scan(&total); err != nil {
		writeError(w, 503, 503, "Unable to query dictionary items")
		return
	}
	args = append(args, s, (p-1)*s)
	rows, err := a.db.QueryContext(r.Context(), `SELECT id,dict_type_id,label,value,tag_type,remark,sort_order,status,created_at,updated_at FROM sys_dict_item WHERE `+clause+` ORDER BY sort_order,id LIMIT ? OFFSET ?`, args...)
	if err != nil {
		writeError(w, 503, 503, "Unable to query dictionary items")
		return
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var itemID, typeID uint64
		var label, value, tag string
		var remark sql.NullString
		var sort uint32
		var status uint8
		var created, updated int64
		if rows.Scan(&itemID, &typeID, &label, &value, &tag, &remark, &sort, &status, &created, &updated) != nil {
			writeError(w, 503, 503, "Unable to query dictionary items")
			return
		}
		out = append(out, dictItemObject(itemID, typeID, dictItemInput{label, value, tag, nullPtrString(remark), sort, status}, created, updated))
	}
	writeJSON(w, 200, envelope{Code: 0, Message: "success", Data: map[string]any{"list": out, "total": total}})
}
func (a *app) dictItemCreate(w http.ResponseWriter, r *http.Request) {
	if _, ok := a.guard(w, r, "system:dict-item:create"); !ok {
		return
	}
	id, ok := pathID(r)
	if !ok {
		writeError(w, 400, 400, "Invalid dictionary id")
		return
	}
	var in dictItemInput
	if !decodeJSON(w, r, &in) || !validateDictItem(in) {
		writeError(w, 400, 400, "Invalid dictionary item")
		return
	}
	var cnt int
	if err := a.db.QueryRowContext(r.Context(), `SELECT COUNT(*) FROM sys_dict_item WHERE dict_type_id=? AND deleted_at IS NULL`, id).Scan(&cnt); err != nil || cnt >= 500 {
		writeError(w, 409, 409, "Dictionary item limit reached")
		return
	}
	now := time.Now().UnixMilli()
	res, err := a.db.ExecContext(r.Context(), `INSERT INTO sys_dict_item(dict_type_id,label,value,tag_type,remark,sort_order,status,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?,?)`, id, in.Label, in.Value, in.TagType, in.Remark, in.SortOrder, in.Status, now, now)
	if err != nil {
		writeError(w, 409, 409, "Dictionary value already exists")
		return
	}
	itemID, _ := res.LastInsertId()
	writeJSON(w, 201, envelope{Code: 0, Message: "success", Data: dictItemObject(uint64(itemID), id, in, now, now)})
}
func (a *app) dictItemUpdate(w http.ResponseWriter, r *http.Request) {
	if _, ok := a.guard(w, r, "system:dict-item:update"); !ok {
		return
	}
	id, ok := parsePathID(r, "id")
	itemID, ok2 := parsePathID(r, "itemId")
	if !ok || !ok2 {
		writeError(w, 400, 400, "Invalid item id")
		return
	}
	var in dictItemInput
	if !decodeJSON(w, r, &in) || !validateDictItem(in) {
		writeError(w, 400, 400, "Invalid dictionary item")
		return
	}
	now := time.Now().UnixMilli()
	res, err := a.db.ExecContext(r.Context(), `UPDATE sys_dict_item SET label=?,value=?,tag_type=?,remark=?,sort_order=?,status=?,updated_at=? WHERE id=? AND dict_type_id=? AND deleted_at IS NULL`, in.Label, in.Value, in.TagType, in.Remark, in.SortOrder, in.Status, now, itemID, id)
	if err != nil {
		writeError(w, 409, 409, "Dictionary value already exists")
		return
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		writeError(w, 404, 404, "Dictionary item not found")
		return
	}
	writeJSON(w, 200, envelope{Code: 0, Message: "success", Data: dictItemObject(itemID, id, in, 0, now)})
}
func (a *app) dictItemDelete(w http.ResponseWriter, r *http.Request) {
	if _, ok := a.guard(w, r, "system:dict-item:delete"); !ok {
		return
	}
	id, ok := parsePathID(r, "id")
	itemID, ok2 := parsePathID(r, "itemId")
	if !ok || !ok2 {
		writeError(w, 400, 400, "Invalid item id")
		return
	}
	now := time.Now().UnixMilli()
	res, err := a.db.ExecContext(r.Context(), `UPDATE sys_dict_item SET deleted_at=?,updated_at=? WHERE id=? AND dict_type_id=? AND deleted_at IS NULL`, now, now, itemID, id)
	if err != nil {
		writeError(w, 503, 503, "Unable to delete item")
		return
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		writeError(w, 404, 404, "Dictionary item not found")
		return
	}
	writeJSON(w, 200, envelope{Code: 0, Message: "success", Data: nil})
}
func (a *app) dictOptions(w http.ResponseWriter, r *http.Request) {
	u, _, err := a.authenticate(r)
	if err != nil {
		writeError(w, 401, 401, "Authentication required")
		return
	}
	_ = u
	code := r.PathValue("code")
	var status uint8
	if err := a.db.QueryRowContext(r.Context(), `SELECT status FROM sys_dict_type WHERE code=? AND deleted_at IS NULL`, code).Scan(&status); err != nil {
		writeError(w, 404, 404, "Dictionary not found")
		return
	}
	out := []map[string]any{}
	if status == 1 {
		rows, err := a.db.QueryContext(r.Context(), `SELECT label,value,tag_type FROM sys_dict_item WHERE dict_type_id=(SELECT id FROM sys_dict_type WHERE code=? AND deleted_at IS NULL) AND status=1 AND deleted_at IS NULL ORDER BY sort_order,id`, code)
		if err != nil {
			writeError(w, 503, 503, "Unable to query options")
			return
		}
		defer rows.Close()
		for rows.Next() {
			var label, value, tag string
			if rows.Scan(&label, &value, &tag) != nil {
				writeError(w, 503, 503, "Unable to query options")
				return
			}
			out = append(out, map[string]any{"label": label, "value": value, "tagType": tag})
		}
	}
	writeJSON(w, 200, envelope{Code: 0, Message: "success", Data: out})
}

func validDictCode(s string) bool {
	if s == "" || len(s) > 64 || s[0] < 'a' || s[0] > 'z' {
		return false
	}
	for _, c := range s {
		if (c < 'a' || c > 'z') && (c < '0' || c > '9') && c != '_' {
			return false
		}
	}
	return true
}
func validateDictItem(x dictItemInput) bool {
	if !validName(x.Label) || x.Value == "" || len(x.Value) > 100 || x.Status > 1 {
		return false
	}
	switch x.TagType {
	case "primary", "success", "warning", "danger", "info":
		return true
	}
	return false
}
func dictItemObject(id, typeID uint64, x dictItemInput, created, updated int64) map[string]any {
	return map[string]any{"id": id, "dictTypeId": typeID, "label": x.Label, "value": x.Value, "tagType": x.TagType, "remark": x.Remark, "sortOrder": x.SortOrder, "status": x.Status, "createdAt": created, "updatedAt": updated}
}
func nullPtrString(v sql.NullString) *string {
	if !v.Valid {
		return nil
	}
	return &v.String
}
func dictFilterInt(s string) (string, bool) {
	if s == "" {
		return "", true
	}
	n, e := strconv.ParseInt(s, 10, 64)
	return strconv.FormatInt(n, 10), e == nil
}
