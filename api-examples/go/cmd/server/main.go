package main

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	_ "github.com/go-sql-driver/mysql"
	"golang.org/x/crypto/bcrypt"
)

type envelope struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data"`
}

type app struct {
	db      *sql.DB
	origins map[string]struct{}
	pepper  []byte
}

type loginInput struct {
	Username   string `json:"username"`
	Password   string `json:"password"`
	RememberMe bool   `json:"rememberMe"`
}

type userInfo struct {
	ID           uint64   `json:"id"`
	Username     string   `json:"username"`
	DisplayName  string   `json:"displayName"`
	Avatar       *string  `json:"avatar,omitempty"`
	DepartmentID *uint64  `json:"departmentId"`
	IsSuperAdmin bool     `json:"isSuperAdmin"`
	Roles        []string `json:"roles"`
	Permissions  []string `json:"permissions"`
}

type authContext struct {
	ID                    uint64
	Username              string
	DisplayName           string
	Avatar                sql.NullString
	DepartmentID          sql.NullInt64
	IsSuperAdmin          bool
	PasswordHash          string
	CredentialFingerprint string
}

func main() {
	loadDotEnv(".env")
	dsn := os.Getenv("MYSQL_DSN")
	if dsn == "" {
		log.Fatal("MYSQL_DSN is required")
	}
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		log.Fatalf("open MySQL connection: %v", err)
	}
	db.SetMaxOpenConns(20)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(5 * time.Minute)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		log.Fatalf("connect to MySQL: %v", err)
	}
	if os.Getenv("APP_ENV") == "development" || (os.Getenv("APP_ENV") != "production" && os.Getenv("DEMO_BOOTSTRAP") == "true") {
		if err := ensureDefaultAdmin(ctx, db); err != nil {
			log.Fatalf("initialize default demo administrator: %v", err)
		}
	}
	defer db.Close()

	origins := map[string]struct{}{}
	for _, origin := range strings.Split(os.Getenv("APP_ORIGINS"), ",") {
		if origin = strings.TrimSpace(origin); origin != "" {
			origins[origin] = struct{}{}
		}
	}
	pepperHex := strings.TrimSpace(os.Getenv("TOKEN_PEPPER"))
	pepper, err := hex.DecodeString(pepperHex)
	if err != nil || len(pepper) != 32 || pepperHex == "replace-with-at-least-32-random-bytes" {
		log.Fatal("TOKEN_PEPPER must be replaced with exactly 64 hexadecimal characters (32 random bytes)")
	}
	a := &app{db: db, origins: origins, pepper: pepper}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", a.health)
	mux.HandleFunc("GET /api/health", a.health)
	mux.HandleFunc("POST /auth/login", a.login)
	mux.HandleFunc("POST /auth/logout", a.logout)
	mux.HandleFunc("POST /auth/password/change", a.changePassword)
	mux.HandleFunc("POST /auth/password/reset", a.resetPassword)
	mux.HandleFunc("GET /auth/totp", a.totpStatus)
	mux.HandleFunc("POST /auth/totp/setup", a.totpNotImplemented)
	mux.HandleFunc("POST /auth/totp/enable", a.totpNotImplemented)
	mux.HandleFunc("POST /auth/totp/disable", a.totpNotImplemented)
	mux.HandleFunc("GET /auth/sessions", a.ownSessions)
	mux.HandleFunc("DELETE /auth/sessions/{sessionId}", a.revokeOwnSession)
	mux.HandleFunc("DELETE /auth/sessions/others", a.revokeOtherSessions)
	mux.HandleFunc("GET /auth/user", a.getUser)
	mux.HandleFunc("GET /auth/session", a.getSession)
	mux.HandleFunc("GET /auth/routes", a.getRoutes)
	mux.HandleFunc("GET /system/users", a.listUsers)
	mux.HandleFunc("POST /system/users", a.createUser)
	mux.HandleFunc("PUT /system/users/{id}", a.updateUser)
	mux.HandleFunc("DELETE /system/users/{id}", a.deleteUser)
	a.registerSystem(mux)
	addr := os.Getenv("APP_ADDR")
	if addr == "" {
		addr = ":8080"
	}
	log.Printf("Npora Go API listening on %s", addr)
	log.Fatal(http.ListenAndServe(addr, a.cors(mux)))
}

// ensureDefaultAdmin initializes only an empty database. The demo password is intentionally
// documented for local examples; production deployments must replace it before exposure.
func ensureDefaultAdmin(ctx context.Context, db *sql.DB) error {
	conn, err := db.Conn(ctx)
	if err != nil {
		return err
	}
	defer conn.Close()
	var lock int
	if err := conn.QueryRowContext(ctx, `SELECT GET_LOCK('npora_go_default_admin',10)`).Scan(&lock); err != nil || lock != 1 {
		return errors.New("unable to acquire administrator bootstrap lock")
	}
	defer conn.ExecContext(context.Background(), `SELECT RELEASE_LOCK('npora_go_default_admin')`)
	tx, err := conn.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var count int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM sys_user`).Scan(&count); err != nil {
		return err
	}
	if count > 0 {
		return tx.Commit()
	}
	hash, err := bcrypt.GenerateFromPassword([]byte("123456"), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	now := time.Now().UnixMilli()
	res, err := tx.ExecContext(ctx, `INSERT INTO sys_user(username,password_hash,display_name,status,is_super_admin,created_at,updated_at) VALUES('admin',?,'系统管理员',1,1,?,?)`, string(hash), now, now)
	if err != nil {
		return err
	}
	userID, err := res.LastInsertId()
	if err != nil {
		return err
	}
	var roleID uint64
	if err := tx.QueryRowContext(ctx, `SELECT id FROM sys_role WHERE code='admin' AND status=1 AND deleted_at IS NULL`).Scan(&roleID); err != nil {
		return fmt.Errorf("admin role missing; run template seed SQL first: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO sys_user_role(user_id,role_id,created_at) VALUES(?,?,?)`, userID, roleID, now); err != nil {
		return err
	}
	return tx.Commit()
}

func (a *app) health(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()
	if err := a.db.PingContext(ctx); err != nil {
		writeJSON(w, http.StatusServiceUnavailable, envelope{Code: 503, Message: "Service unavailable", Data: nil})
		return
	}
	writeJSON(w, http.StatusOK, envelope{Code: 0, Message: "success", Data: map[string]string{"status": "ok"}})
}

func (a *app) login(w http.ResponseWriter, r *http.Request) {
	var input loginInput
	if !decodeJSON(w, r, &input) || len(input.Username) < 3 || len(input.Username) > 64 || len(input.Password) < 1 || len(input.Password) > 128 {
		writeError(w, http.StatusBadRequest, 400, "Invalid request")
		return
	}
	var userID uint64
	var passwordHash string
	var displayName string
	var status uint8
	var superAdmin bool
	var departmentID sql.NullInt64
	err := a.db.QueryRowContext(r.Context(), `SELECT id, password_hash, display_name, status, is_super_admin, department_id
		FROM sys_user WHERE username = ? AND deleted_at IS NULL`, input.Username).Scan(&userID, &passwordHash, &displayName, &status, &superAdmin, &departmentID)
	if err != nil || status != 1 || bcrypt.CompareHashAndPassword([]byte(passwordHash), []byte(input.Password)) != nil {
		writeError(w, http.StatusUnauthorized, 401, "Invalid username or password")
		return
	}
	// Fail closed while the optional TOTP flow is not implemented in this example.
	// Never issue a session that bypasses an enabled second factor.
	var totpEnabled int
	if err := a.db.QueryRowContext(r.Context(), `SELECT COALESCE((SELECT enabled FROM sys_user_totp WHERE user_id=?),0)`, userID).Scan(&totpEnabled); err != nil {
		writeError(w, http.StatusServiceUnavailable, 503, "Unable to verify authenticator status")
		return
	}
	if totpEnabled == 1 {
		writeError(w, http.StatusNotImplemented, 501, "Authenticator login is not implemented in this example")
		return
	}
	tokenBytes := make([]byte, 32)
	if _, err := rand.Read(tokenBytes); err != nil {
		writeError(w, http.StatusInternalServerError, 500, "Authentication failed")
		return
	}
	token := hex.EncodeToString(tokenBytes)
	now := time.Now().UnixMilli()
	ttl := envInt("SESSION_TTL_HOURS", 1)
	if input.RememberMe {
		ttl = envInt("REMEMBER_SESSION_TTL_HOURS", 720)
	}
	expiresAt := now + int64(ttl)*int64(time.Hour/time.Millisecond)
	tokenHash := sha256.Sum256(append(a.pepper, []byte(token)...))
	fingerprint := credentialFingerprint(passwordHash)
	publicIDBytes := make([]byte, 16)
	if _, err := rand.Read(publicIDBytes); err != nil {
		writeError(w, http.StatusInternalServerError, 500, "Authentication failed")
		return
	}
	_, err = a.db.ExecContext(r.Context(), `INSERT INTO sys_auth_session(token_hash, public_id, user_id, credential_fingerprint, created_at, expires_at)
		VALUES (?, ?, ?, ?, ?, ?)`, hex.EncodeToString(tokenHash[:]), hex.EncodeToString(publicIDBytes), userID, fingerprint, now, expiresAt)
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, 503, "Authentication service unavailable")
		return
	}
	_, _ = a.db.ExecContext(r.Context(), `UPDATE sys_user SET last_login_at = ?, updated_at = ? WHERE id = ?`, now, now, userID)
	writeJSON(w, http.StatusOK, envelope{Code: 0, Message: "success", Data: map[string]any{"accessToken": token, "expiresAt": expiresAt}})
}

func (a *app) authenticate(r *http.Request) (authContext, string, error) {
	header := r.Header.Get("Authorization")
	if !strings.HasPrefix(header, "Bearer ") || len(header) < 8 {
		return authContext{}, "", errors.New("unauthorized")
	}
	token := strings.TrimSpace(strings.TrimPrefix(header, "Bearer "))
	sum := sha256.Sum256(append(a.pepper, []byte(token)...))
	var user authContext
	var sessionHash string
	var expiresAt int64
	err := a.db.QueryRowContext(r.Context(), `SELECT u.id, u.username, u.display_name, u.avatar, u.department_id,
		u.is_super_admin, u.password_hash, s.credential_fingerprint, s.token_hash, s.expires_at
		FROM sys_auth_session s JOIN sys_user u ON u.id = s.user_id
		WHERE s.token_hash = ? AND s.expires_at > ? AND u.status = 1 AND u.deleted_at IS NULL`, hex.EncodeToString(sum[:]), time.Now().UnixMilli()).
		Scan(&user.ID, &user.Username, &user.DisplayName, &user.Avatar, &user.DepartmentID, &user.IsSuperAdmin,
			&user.PasswordHash, &user.CredentialFingerprint, &sessionHash, &expiresAt)
	if err != nil || user.CredentialFingerprint != credentialFingerprint(user.PasswordHash) {
		return authContext{}, "", errors.New("unauthorized")
	}
	return user, sessionHash, nil
}

func (a *app) logout(w http.ResponseWriter, r *http.Request) {
	_, sessionHash, err := a.authenticate(r)
	if err != nil {
		writeJSON(w, http.StatusOK, envelope{Code: 0, Message: "success", Data: nil})
		return
	}
	_, _ = a.db.ExecContext(r.Context(), `DELETE FROM sys_auth_session WHERE token_hash = ?`, sessionHash)
	writeJSON(w, http.StatusOK, envelope{Code: 0, Message: "success", Data: nil})
}

func (a *app) getUser(w http.ResponseWriter, r *http.Request) {
	user, _, err := a.authenticate(r)
	if err != nil {
		writeError(w, 401, 401, "Authentication required")
		return
	}
	info, err := a.userInfo(r.Context(), user)
	if err != nil {
		writeError(w, 503, 503, "Unable to load user")
		return
	}
	writeJSON(w, 200, envelope{Code: 0, Message: "success", Data: info})
}

func (a *app) getSession(w http.ResponseWriter, r *http.Request) {
	user, _, err := a.authenticate(r)
	if err != nil {
		writeError(w, 401, 401, "Authentication required")
		return
	}
	info, err := a.userInfo(r.Context(), user)
	if err != nil {
		writeError(w, 503, 503, "Unable to load user")
		return
	}
	routes, err := a.routes(r.Context(), info)
	if err != nil {
		writeError(w, 503, 503, "Unable to load routes")
		return
	}
	writeJSON(w, 200, envelope{Code: 0, Message: "success", Data: map[string]any{"user": info, "routes": routes}})
}

func (a *app) getRoutes(w http.ResponseWriter, r *http.Request) {
	user, _, err := a.authenticate(r)
	if err != nil {
		writeError(w, 401, 401, "Authentication required")
		return
	}
	info, err := a.userInfo(r.Context(), user)
	if err != nil {
		writeError(w, 503, 503, "Unable to load user")
		return
	}
	routes, err := a.routes(r.Context(), info)
	if err != nil {
		writeError(w, 503, 503, "Unable to load routes")
		return
	}
	writeJSON(w, 200, envelope{Code: 0, Message: "success", Data: routes})
}

func (a *app) userInfo(ctx context.Context, user authContext) (userInfo, error) {
	info := userInfo{ID: user.ID, Username: user.Username, DisplayName: user.DisplayName,
		IsSuperAdmin: user.IsSuperAdmin, Roles: []string{}, Permissions: []string{}}
	if user.Avatar.Valid {
		info.Avatar = &user.Avatar.String
	}
	if user.DepartmentID.Valid {
		id := uint64(user.DepartmentID.Int64)
		info.DepartmentID = &id
	}
	rows, err := a.db.QueryContext(ctx, `SELECT r.name FROM sys_user_role ur JOIN sys_role r ON r.id = ur.role_id
		WHERE ur.user_id = ? AND r.status = 1 AND r.deleted_at IS NULL ORDER BY r.sort_order, r.id`, user.ID)
	if err != nil {
		return info, err
	}
	defer rows.Close()
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return info, err
		}
		info.Roles = append(info.Roles, name)
	}
	if err := rows.Err(); err != nil {
		return info, err
	}
	rows, err = a.db.QueryContext(ctx, `SELECT DISTINCT m.permission FROM sys_user_role ur
		JOIN sys_role r ON r.id = ur.role_id AND r.status = 1 AND r.deleted_at IS NULL
		JOIN sys_role_menu rm ON rm.role_id = r.id JOIN sys_menu m ON m.id = rm.menu_id
		WHERE ur.user_id = ? AND m.type = 3 AND m.permission IS NOT NULL AND m.status = 1 AND m.deleted_at IS NULL ORDER BY m.permission`, user.ID)
	if err != nil {
		return info, err
	}
	defer rows.Close()
	for rows.Next() {
		var permission string
		if err := rows.Scan(&permission); err != nil {
			return info, err
		}
		info.Permissions = append(info.Permissions, permission)
	}
	return info, rows.Err()
}

func (a *app) routes(ctx context.Context, info userInfo) ([]map[string]any, error) {
	rows, err := a.db.QueryContext(ctx, `SELECT DISTINCT m.id, m.parent_id, m.type, m.title, m.name, m.path,
		m.component, m.redirect, m.route_query, m.active_menu, m.icon, m.visible, m.always_show, m.keep_alive, m.fixed_tab
		FROM sys_menu m LEFT JOIN sys_role_menu rm ON rm.menu_id = m.id
		LEFT JOIN sys_user_role ur ON ur.role_id = rm.role_id
		LEFT JOIN sys_role r ON r.id = ur.role_id AND r.status = 1 AND r.deleted_at IS NULL
		WHERE m.type IN (1,2) AND m.status = 1 AND m.deleted_at IS NULL AND ( ? OR ur.user_id = ? )
		ORDER BY m.sort_order, m.id`, info.IsSuperAdmin, info.ID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	type routeRow struct {
		id                                            uint64
		parent                                        sql.NullInt64
		kind                                          int
		title                                         string
		name, path, component, redirect, active, icon sql.NullString
		query                                         []byte
		visible, always, keep, fixed                  bool
	}
	var list []routeRow
	for rows.Next() {
		var x routeRow
		if err := rows.Scan(&x.id, &x.parent, &x.kind, &x.title, &x.name, &x.path, &x.component, &x.redirect, &x.query, &x.active, &x.icon, &x.visible, &x.always, &x.keep, &x.fixed); err != nil {
			return nil, err
		}
		list = append(list, x)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	byID := map[uint64]map[string]any{}
	roots := []map[string]any{}
	for _, x := range list {
		meta := map[string]any{"title": x.title, "hidden": !x.visible, "alwaysShow": x.always, "keepAlive": x.keep, "fixedTab": x.fixed}
		if x.icon.Valid {
			meta["icon"] = x.icon.String
		}
		route := map[string]any{"meta": meta, "children": []map[string]any{}}
		if x.name.Valid {
			route["name"] = x.name.String
		}
		if x.path.Valid {
			route["path"] = x.path.String
		}
		if x.component.Valid {
			route["component"] = x.component.String
		}
		if x.redirect.Valid {
			route["redirect"] = x.redirect.String
		}
		if x.active.Valid {
			meta["activeMenu"] = x.active.String
		}
		if len(x.query) > 0 {
			var q any
			if json.Unmarshal(x.query, &q) == nil {
				route["query"] = q
			}
		}
		byID[x.id] = route
		if x.parent.Valid && byID[uint64(x.parent.Int64)] != nil {
			p := byID[uint64(x.parent.Int64)]
			p["children"] = append(p["children"].([]map[string]any), route)
		} else {
			roots = append(roots, route)
		}
	}
	return roots, nil
}

func (a *app) listUsers(w http.ResponseWriter, r *http.Request) {
	user, _, err := a.authenticate(r)
	if err != nil {
		writeError(w, 401, 401, "Authentication required")
		return
	}
	if !user.IsSuperAdmin && !a.hasPermission(r.Context(), user.ID, "system:user:list") {
		writeError(w, 403, 403, "Permission denied")
		return
	}
	page, size := pageQuery(r)
	where := []string{"u.deleted_at IS NULL"}
	args := []any{}
	if keyword := strings.TrimSpace(r.URL.Query().Get("keyword")); keyword != "" {
		where = append(where, "(u.username LIKE ? OR u.display_name LIKE ? OR u.email LIKE ? OR u.mobile LIKE ?)")
		for i := 0; i < 4; i++ {
			args = append(args, "%"+keyword+"%")
		}
	}
	if raw := r.URL.Query().Get("status"); raw != "" {
		status, err := strconv.ParseUint(raw, 10, 8)
		if err != nil || status > 1 {
			writeError(w, 400, 400, "Invalid status filter")
			return
		}
		where = append(where, "u.status=?")
		args = append(args, status)
	}
	if raw := r.URL.Query().Get("roleId"); raw != "" {
		roleID, err := strconv.ParseUint(raw, 10, 64)
		if err != nil || roleID == 0 {
			writeError(w, 400, 400, "Invalid role filter")
			return
		}
		where = append(where, "EXISTS (SELECT 1 FROM sys_user_role ur WHERE ur.user_id=u.id AND ur.role_id=?)")
		args = append(args, roleID)
	}
	if raw := r.URL.Query().Get("departmentId"); raw != "" {
		deptID, err := strconv.ParseUint(raw, 10, 64)
		if err != nil || deptID == 0 {
			writeError(w, 400, 400, "Invalid department filter")
			return
		}
		rows, err := a.db.QueryContext(r.Context(), `WITH RECURSIVE tree AS (SELECT id FROM sys_department WHERE id=? AND deleted_at IS NULL UNION ALL SELECT d.id FROM sys_department d JOIN tree t ON d.parent_id=t.id WHERE d.deleted_at IS NULL) SELECT id FROM tree`, deptID)
		if err != nil {
			writeError(w, 503, 503, "Unable to query departments")
			return
		}
		ids := []any{}
		for rows.Next() {
			var child uint64
			if rows.Scan(&child) == nil {
				ids = append(ids, child)
			}
		}
		rows.Close()
		if len(ids) == 0 {
			writeError(w, 400, 400, "Department not found")
			return
		}
		marks := strings.TrimRight(strings.Repeat("?,", len(ids)), ",")
		where = append(where, "u.department_id IN ("+marks+")")
		args = append(args, ids...)
	}
	clause := strings.Join(where, " AND ")
	var total int64
	if err := a.db.QueryRowContext(r.Context(), `SELECT COUNT(*) FROM sys_user u WHERE `+clause, args...).Scan(&total); err != nil {
		writeError(w, 503, 503, "Unable to query users")
		return
	}
	queryArgs := append(append([]any{}, args...), size, (page-1)*size)
	rows, err := a.db.QueryContext(r.Context(), `SELECT u.id,u.username,u.display_name,u.avatar,u.department_id,u.status,u.is_super_admin,u.last_login_at,u.created_at,u.updated_at,d.name,u.email,u.mobile
		FROM sys_user u LEFT JOIN sys_department d ON d.id=u.department_id AND d.deleted_at IS NULL
		WHERE `+clause+` ORDER BY u.created_at DESC,u.id DESC LIMIT ? OFFSET ?`, queryArgs...)
	if err != nil {
		writeError(w, 503, 503, "Unable to query users")
		return
	}
	defer rows.Close()
	list := []map[string]any{}
	for rows.Next() {
		var id uint64
		var username, display string
		var avatar, deptName, email, mobile sql.NullString
		var deptID, last, created, updated sql.NullInt64
		var status uint8
		var super bool
		if err := rows.Scan(&id, &username, &display, &avatar, &deptID, &status, &super, &last, &created, &updated, &deptName, &email, &mobile); err != nil {
			writeError(w, 503, 503, "Unable to query users")
			return
		}
		roles, roleIDs, roleErr := a.userRoleDTO(r.Context(), id)
		if roleErr != nil {
			writeError(w, 503, 503, "Unable to query user roles")
			return
		}
		var totpEnabled int
		_ = a.db.QueryRowContext(r.Context(), `SELECT COALESCE((SELECT enabled FROM sys_user_totp WHERE user_id=?),0)`, id).Scan(&totpEnabled)
		obj := map[string]any{"id": id, "username": username, "displayName": display, "avatar": nullString(avatar), "departmentId": nullInt(deptID), "departmentName": nullString(deptName), "status": status, "isSuperAdmin": super, "authenticatorEnabled": totpEnabled == 1, "lastLoginAt": nullInt(last), "createdAt": created.Int64, "updatedAt": updated.Int64, "email": nullString(email), "mobile": nullString(mobile), "roleIds": roleIDs, "roles": roles}
		list = append(list, obj)
	}
	if err := rows.Err(); err != nil {
		writeError(w, 503, 503, "Unable to query users")
		return
	}
	writeJSON(w, 200, envelope{Code: 0, Message: "success", Data: map[string]any{"list": list, "total": total}})
}

func (a *app) hasPermission(ctx context.Context, userID uint64, permission string) bool {
	var found int
	err := a.db.QueryRowContext(ctx, `SELECT 1 FROM sys_user_role ur JOIN sys_role r ON r.id=ur.role_id AND r.status=1 AND r.deleted_at IS NULL
		JOIN sys_role_menu rm ON rm.role_id=r.id JOIN sys_menu m ON m.id=rm.menu_id
		WHERE ur.user_id=? AND m.permission=? AND m.type=3 AND m.status=1 AND m.deleted_at IS NULL LIMIT 1`, userID, permission).Scan(&found)
	return err == nil
}

func nullString(v sql.NullString) any {
	if v.Valid {
		return v.String
	}
	return nil
}
func nullInt(v sql.NullInt64) any {
	if v.Valid {
		return v.Int64
	}
	return nil
}
func credentialFingerprint(passwordHash string) string {
	sum := sha256.Sum256([]byte(passwordHash))
	return hex.EncodeToString(sum[:])
}
func decodeJSON(w http.ResponseWriter, r *http.Request, out any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	defer r.Body.Close()
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	return dec.Decode(out) == nil
}
func writeError(w http.ResponseWriter, status, code int, message string) {
	writeJSON(w, status, envelope{Code: code, Message: message, Data: nil})
}
func envInt(name string, fallback int) int {
	v, err := strconv.Atoi(os.Getenv(name))
	if err != nil || v < 1 {
		return fallback
	}
	return v
}
func (a *app) cors(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if origin != "" {
			if _, ok := a.origins[origin]; !ok {
				writeJSON(w, http.StatusForbidden, envelope{Code: 403, Message: "Origin not allowed", Data: nil})
				return
			}
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Vary", "Origin")
			w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
		}
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func writeJSON(w http.ResponseWriter, status int, body envelope) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func loadDotEnv(path string) {
	content, err := os.ReadFile(path)
	if err != nil {
		return
	}
	for _, line := range strings.Split(string(content), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		name, value, ok := strings.Cut(line, "=")
		if !ok || strings.TrimSpace(name) == "" {
			continue
		}
		name = strings.TrimSpace(name)
		value = strings.Trim(strings.TrimSpace(value), "\"'")
		if _, exists := os.LookupEnv(name); !exists {
			_ = os.Setenv(name, value)
		}
	}
}
