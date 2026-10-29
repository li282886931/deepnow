// api_admin.go - 管理后台业务逻辑与接口
// 2025.12 Revision,HEAD 232
// by szyok
package main

import (
	"bytes"
	"context"
	crand "crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"golang.org/x/text/encoding/simplifiedchinese"
	"golang.org/x/text/transform"
)

var sessionToken string

func sendJSON(w http.ResponseWriter, success bool, msg string, data interface{}) {
	sendJSONStatus(w, http.StatusOK, success, msg, data)
}

func sendJSONStatus(w http.ResponseWriter, status int, success bool, msg string, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	requestID, err := randomHex(8)
	if err != nil {
		requestID = fmt.Sprintf("req-%d", time.Now().UnixNano())
	}
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success":    success,
		"message":    msg,
		"data":       data,
		"request_id": requestID,
	})
}

func randomHex(bytesLen int) (string, error) {
	buf := make([]byte, bytesLen)
	if _, err := crand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}

func hashPassword(password string) (string, error) {
	salt, err := randomHex(16)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256([]byte(salt + ":" + password))
	return "sha256$" + salt + "$" + hex.EncodeToString(sum[:]), nil
}

func verifyPassword(stored string, password string) bool {
	if !strings.HasPrefix(stored, "sha256$") {
		return subtle.ConstantTimeCompare([]byte(stored), []byte(password)) == 1
	}
	parts := strings.Split(stored, "$")
	if len(parts) != 3 {
		return false
	}
	sum := sha256.Sum256([]byte(parts[1] + ":" + password))
	expected := hex.EncodeToString(sum[:])
	return subtle.ConstantTimeCompare([]byte(parts[2]), []byte(expected)) == 1
}

func ensureSessionToken() string {
	if sessionToken != "" {
		return sessionToken
	}
	token, err := randomHex(32)
	if err != nil {
		sessionToken = fmt.Sprintf("fallback-%d", time.Now().UnixNano())
		return sessionToken
	}
	sessionToken = token
	return sessionToken
}

func requireAdmin(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie("session")
		if err != nil || sessionToken == "" || subtle.ConstantTimeCompare([]byte(cookie.Value), []byte(sessionToken)) != 1 {
			sendJSONStatus(w, http.StatusUnauthorized, false, "未登录或会话已过期", nil)
			return
		}
		next(w, r)
	}
}

func handleGetSystemStatus(w http.ResponseWriter, r *http.Request) {
	pri := getActiveModel("LM", "primary")
	sec := getActiveModel("LM", "slave")
	rag := getActiveModel("Embedding", "")
	sendJSON(w, true, "", map[string]interface{}{
		"needs_setup": pri == nil,
		"active_pri":  sanitizedModelConfig(pri),
		"active_sec":  sanitizedModelConfig(sec),
		"active_rag":  sanitizedModelConfig(rag),
	})
}

type sanitizedModel struct {
	Provider  string `json:"provider"`
	ApiType   string `json:"api_type"`
	ApiKey    string `json:"api_key"`
	ModelName string `json:"model_name"`
	IpPort    string `json:"ip_port"`
}

func sanitizedModelConfig(model *ModelConfig) interface{} {
	if model == nil {
		return nil
	}
	return sanitizedModel{
		Provider:  model.Provider,
		ApiType:   model.ApiType,
		ApiKey:    maskSecret(model.ApiKey),
		ModelName: model.ModelName,
		IpPort:    model.IpPort,
	}
}

func handleGetBuildInfo(w http.ResponseWriter, r *http.Request) {
	exePath, err := os.Executable()
	buildTime := "Unknown"
	if err == nil {
		if info, err := os.Stat(exePath); err == nil {
			buildTime = info.ModTime().Format("20060102_150405")
		}
	}
	sendJSON(w, true, "", buildTime)
}

func handleAdminLogin(w http.ResponseWriter, r *http.Request) {
	u, p := r.FormValue("username"), r.FormValue("password")
	var dbP string
	if sysDB.QueryRow(`SELECT password FROM admin_users WHERE username=?`, u).Scan(&dbP) == nil && verifyPassword(dbP, p) {
		if !strings.HasPrefix(dbP, "sha256$") {
			if upgraded, err := hashPassword(p); err == nil {
				sysDB.Exec(`UPDATE admin_users SET password=? WHERE username=?`, upgraded, u)
			}
		}
		http.SetCookie(w, &http.Cookie{
			Name:     "session",
			Value:    ensureSessionToken(),
			Path:     "/",
			HttpOnly: true,
			SameSite: http.SameSiteLaxMode,
			MaxAge:   8 * 60 * 60,
		})
		_ = writeAuditEvent(u, "login", "session", "", "success")
		sendJSON(w, true, "登录成功", nil)
	} else {
		_ = writeAuditEvent(u, "login", "session", "", "failure")
		sendJSON(w, false, "账号或密码错误", nil)
	}
}

func handleAdminLogout(w http.ResponseWriter, r *http.Request) {
	sessionToken = ""
	http.SetCookie(w, &http.Cookie{
		Name:     "session",
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   -1,
		Expires:  time.Unix(1, 0),
	})
	sendJSON(w, true, "已退出登录", nil)
}

func handleAdminSession(w http.ResponseWriter, r *http.Request) {
	sendJSON(w, true, "会话有效", map[string]bool{"authenticated": true})
}

func handleGetConfig(w http.ResponseWriter, r *http.Request) {
	sendJSON(w, true, "获取配置", config)
}
func handleSaveConfig(w http.ResponseWriter, r *http.Request) {
	if err := json.NewDecoder(r.Body).Decode(&config); err != nil {
		sendJSON(w, false, "配置格式错误", err.Error())
		return
	}
	if err := saveConfig(); err != nil {
		_ = writeAuditEvent("admin", "config.save", "config", "", "failure")
		sendJSON(w, false, "配置保存失败", err.Error())
		return
	}
	_ = writeAuditEvent("admin", "config.save", "config", "", "success")
	sendJSON(w, true, "配置已保存", nil)
}

func handleGenerateKeys(w http.ResponseWriter, r *http.Request) {
	var keys []string
	for i := 0; i < 5; i++ {
		suffix, err := randomHex(16)
		if err != nil {
			sendJSON(w, false, "生成失败", err.Error())
			return
		}
		keys = append(keys, "sk-aibase-"+suffix)
		if _, err := keyDB.Exec(`INSERT INTO access_keys (api_key, used, created_at, name, status)
			VALUES (?, 0, CURRENT_TIMESTAMP, '', 'active')`, keys[len(keys)-1]); err != nil {
			sendJSONStatus(w, http.StatusInternalServerError, false, "生成失败", "无法保存访问密钥")
			return
		}
		authKeys.Store(keys[len(keys)-1], 0)
	}
	sendJSON(w, true, "生成成功", keys)
}
func handleImportKeys(w http.ResponseWriter, r *http.Request) {
	var keys []string
	if err := json.NewDecoder(r.Body).Decode(&keys); err != nil {
		sendJSONStatus(w, http.StatusBadRequest, false, "Key 格式错误", nil)
		return
	}
	for _, k := range keys {
		if strings.TrimSpace(k) == "" {
			continue
		}
		keyDB.Exec(`INSERT OR IGNORE INTO access_keys (api_key, used, created_at) VALUES (?, 0, datetime('now'))`, k)
		authKeys.Store(k, 0)
	}
	_ = writeAuditEvent("admin", "key.import", "access_key", "", "success")
	sendJSON(w, true, "已入库", nil)
}
func handleListKeys(w http.ResponseWriter, r *http.Request) {
	rows, err := keyDB.Query(`SELECT k.api_key, k.used, k.name, k.status, k.created_at, k.last_used_at,
		COALESCE(SUM(u.request_count), 0), COALESCE(SUM(u.input_tokens), 0),
		COALESCE(SUM(u.output_tokens), 0), COALESCE(SUM(u.error_count), 0)
		FROM access_keys k
		LEFT JOIN key_usage_daily u ON u.api_key = k.api_key
		GROUP BY k.api_key, k.used, k.name, k.status, k.created_at, k.last_used_at
		ORDER BY k.created_at DESC`)
	if err != nil {
		sendJSONStatus(w, http.StatusInternalServerError, false, "Key 查询失败", nil)
		return
	}
	defer rows.Close()
	type K struct {
		ID           string  `json:"id"`
		Key          string  `json:"key"`
		Name         string  `json:"name"`
		Status       string  `json:"status"`
		RequestCount int     `json:"request_count"`
		InputTokens  int     `json:"input_tokens"`
		OutputTokens int     `json:"output_tokens"`
		ErrorCount   int     `json:"error_count"`
		CreatedAt    string  `json:"created_at"`
		LastUsedAt   *string `json:"last_used_at"`
	}
	var res []K
	for rows.Next() {
		var k string
		var u int
		var item K
		if err := rows.Scan(&k, &u, &item.Name, &item.Status, &item.CreatedAt, &item.LastUsedAt,
			&item.RequestCount, &item.InputTokens, &item.OutputTokens, &item.ErrorCount); err != nil {
			sendJSONStatus(w, http.StatusInternalServerError, false, "Key 读取失败", nil)
			return
		}
		item.ID = maskSecret(k)
		item.Key = item.ID
		res = append(res, item)
	}
	if err := rows.Err(); err != nil {
		sendJSONStatus(w, http.StatusInternalServerError, false, "Key 读取失败", nil)
		return
	}
	sendJSON(w, true, "", res)
}

func maskSecret(secret string) string {
	if secret == "" {
		return ""
	}
	if len(secret) <= 8 {
		return strings.Repeat("*", len(secret))
	}
	return secret[:4] + strings.Repeat("*", len(secret)-8) + secret[len(secret)-4:]
}
func handleDeleteKeys(w http.ResponseWriter, r *http.Request) {
	var keys []string
	if err := json.NewDecoder(r.Body).Decode(&keys); err != nil {
		sendJSONStatus(w, http.StatusBadRequest, false, "Key 格式错误", nil)
		return
	}
	for _, key := range keys {
		if actual, ok := resolveAccessKey(key); ok {
			keyDB.Exec(`DELETE FROM access_keys WHERE api_key=?`, actual)
			keyDB.Exec(`DELETE FROM key_usage_daily WHERE api_key=?`, actual)
			authKeys.Delete(actual)
		}
	}
	_ = writeAuditEvent("admin", "key.delete", "access_key", "", "success")
	sendJSON(w, true, "删除成功", nil)
}

func resolveAccessKey(identifier string) (string, bool) {
	var key string
	if err := keyDB.QueryRow(`SELECT api_key FROM access_keys WHERE api_key=?`, identifier).Scan(&key); err == nil {
		return key, true
	}
	rows, err := keyDB.Query(`SELECT api_key FROM access_keys`)
	if err != nil {
		return "", false
	}
	defer rows.Close()
	for rows.Next() {
		if err := rows.Scan(&key); err == nil && maskSecret(key) == identifier {
			return key, true
		}
	}
	return "", false
}

func handleKeyUsage(w http.ResponseWriter, r *http.Request) {
	identifier := strings.TrimSpace(r.URL.Query().Get("key_id"))
	actual, ok := resolveAccessKey(identifier)
	if !ok {
		sendJSONStatus(w, http.StatusNotFound, false, "Key 不存在", nil)
		return
	}

	type keyView struct {
		ID           string  `json:"id"`
		Key          string  `json:"key"`
		Name         string  `json:"name"`
		Status       string  `json:"status"`
		RequestCount int     `json:"request_count"`
		InputTokens  int     `json:"input_tokens"`
		OutputTokens int     `json:"output_tokens"`
		ErrorCount   int     `json:"error_count"`
		CreatedAt    string  `json:"created_at"`
		LastUsedAt   *string `json:"last_used_at"`
	}
	var item keyView
	if err := keyDB.QueryRow(`SELECT api_key, name, status, created_at, last_used_at FROM access_keys WHERE api_key=?`, actual).
		Scan(&item.Key, &item.Name, &item.Status, &item.CreatedAt, &item.LastUsedAt); err != nil {
		sendJSONStatus(w, http.StatusNotFound, false, "Key 不存在", nil)
		return
	}
	item.ID = maskSecret(actual)
	item.Key = item.ID
	if err := keyDB.QueryRow(`SELECT
		COALESCE(SUM(request_count), 0), COALESCE(SUM(input_tokens), 0),
		COALESCE(SUM(output_tokens), 0), COALESCE(SUM(error_count), 0)
		FROM key_usage_daily WHERE api_key=?`, actual).
		Scan(&item.RequestCount, &item.InputTokens, &item.OutputTokens, &item.ErrorCount); err != nil {
		sendJSONStatus(w, http.StatusInternalServerError, false, "Key 使用量查询失败", nil)
		return
	}

	days := 30
	if r.URL.Query().Get("range") == "7d" {
		days = 7
	}
	rows, err := keyDB.Query(`SELECT usage_date, request_count, input_tokens, output_tokens, error_count
		FROM key_usage_daily WHERE api_key=? AND usage_date >= date('now', ?)
		ORDER BY usage_date DESC`, actual, fmt.Sprintf("-%d days", days))
	if err != nil {
		sendJSONStatus(w, http.StatusInternalServerError, false, "Key 日使用量查询失败", nil)
		return
	}
	defer rows.Close()
	daily := make([]map[string]interface{}, 0)
	for rows.Next() {
		var date string
		var requests, input, output, errors int
		if err := rows.Scan(&date, &requests, &input, &output, &errors); err != nil {
			sendJSONStatus(w, http.StatusInternalServerError, false, "Key 日使用量读取失败", nil)
			return
		}
		daily = append(daily, map[string]interface{}{
			"date": date, "request_count": requests, "input_tokens": input,
			"output_tokens": output, "error_count": errors,
		})
	}
	sendJSON(w, true, "", map[string]interface{}{"key": item, "daily": daily})
}

func handleGetModelConfigs(w http.ResponseWriter, r *http.Request) {
	rows, _ := sysDB.Query(`SELECT provider, api_type, api_key, model_name, ip_port FROM model_configs`)
	defer rows.Close()
	var res []sanitizedModel
	for rows.Next() {
		var c ModelConfig
		rows.Scan(&c.Provider, &c.ApiType, &c.ApiKey, &c.ModelName, &c.IpPort)
		res = append(res, sanitizedModel{
			Provider:  c.Provider,
			ApiType:   c.ApiType,
			ApiKey:    maskSecret(c.ApiKey),
			ModelName: c.ModelName,
			IpPort:    c.IpPort,
		})
	}
	sendJSON(w, true, "获取模型配置", res)
}

func handleSaveModelConfig(w http.ResponseWriter, r *http.Request) {
	var req ModelConfig
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		sendJSONStatus(w, http.StatusBadRequest, false, "模型配置格式错误", nil)
		return
	}
	if req.Provider == "" || req.ApiType == "" || req.ModelName == "" || req.ApiKey == "" {
		sendJSONStatus(w, http.StatusBadRequest, false, "provider、api_type、api_key 和 model_name 必填", nil)
		return
	}
	if _, err := sysDB.Exec(`INSERT OR REPLACE INTO model_configs (provider, api_type, api_key, model_name, ip_port) VALUES (?, ?, ?, ?, ?)`,
		req.Provider, req.ApiType, req.ApiKey, req.ModelName, req.IpPort); err != nil {
		sendJSONStatus(w, http.StatusInternalServerError, false, "模型配置保存失败", nil)
		return
	}
	autoActivateModels()
	_ = writeAuditEvent("admin", "model.save", "model", req.Provider+"/"+req.ApiType, "success")
	sendJSON(w, true, "算力配置已保存", nil)
}

func handleActivateModel(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Pri string
		Sec string
		RAG string
	}
	json.NewDecoder(r.Body).Decode(&req)

	valPri := req.Pri
	if valPri == "" {
		valPri = "none"
	}
	sysDB.Exec(`INSERT OR REPLACE INTO system_rules (key, value) VALUES ('active_lm_pri', ?)`, valPri)

	valSec := req.Sec
	if valSec == "" {
		valSec = "none"
	}
	sysDB.Exec(`INSERT OR REPLACE INTO system_rules (key, value) VALUES ('active_lm_sec', ?)`, valSec)

	valRAG := req.RAG
	if valRAG == "" {
		valRAG = "none"
	}
	sysDB.Exec(`INSERT OR REPLACE INTO system_rules (key, value) VALUES ('active_embedding', ?)`, valRAG)

	if valRAG == "none" {
		sysDB.Exec(`UPDATE system_rules SET value='0' WHERE key='rag_slider'`)
	}

	fmt.Println("--- [人工重新调度模型] ---")
	pri := getActiveModel("LM", "primary")
	if pri != nil {
		fmt.Printf("✅ 大模型 [%s] (%s) (Primary) 现在启用\n", pri.Provider, pri.ModelName)
	}

	sec := getActiveModel("LM", "slave")
	if sec != nil {
		fmt.Printf("✅ 大模型 [%s] (%s) (Slave) 现在启用\n", sec.Provider, sec.ModelName)
	}

	rag := getActiveModel("Embedding", "")
	if rag != nil {
		fmt.Printf("✅ Embedding 模型 [%s] (%s) 现在启用\n", rag.Provider, rag.ModelName)
	} else {
		fmt.Printf("⚠️ Embedding 模型 现在人工停用 (纯大模型模式，规则滑块已归零)\n")
	}
	fmt.Println("--------------------------")

	_ = writeAuditEvent("admin", "model.activate", "model", valPri, "success")
	sendJSON(w, true, "算力挂载配置已应用", nil)
}

func splitTextWithOverlap(text string, chunkSize int, overlapSize int) []string {
	runes := []rune(text)
	totalLen := len(runes)
	var chunks []string

	if totalLen <= chunkSize {
		return []string{text}
	}
	step := chunkSize - overlapSize
	if step <= 0 {
		step = 100
	}

	for i := 0; i < totalLen; i += step {
		end := i + chunkSize
		if end > totalLen {
			end = totalLen
		}
		chunkStr := string(runes[i:end])
		if len(strings.TrimSpace(chunkStr)) > 5 {
			chunks = append(chunks, chunkStr)
		}
		if end == totalLen {
			break
		}
	}
	return chunks
}

func handleRAGUpload(w http.ResponseWriter, r *http.Request) {
	ragModel := getActiveModel("Embedding", "")
	if ragModel == nil {
		sendJSONStatus(w, http.StatusBadRequest, false, "尚未配置并激活 Embedding 模型", nil)
		return
	}

	if err := r.ParseMultipartForm(10 << 20); err != nil {
		sendJSONStatus(w, http.StatusBadRequest, false, "文件请求格式错误", nil)
		return
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		sendJSONStatus(w, http.StatusBadRequest, false, "缺少 file 文件", nil)
		return
	}
	if file != nil {
		defer file.Close()
		documentID, err := createKnowledgeDocument(header.Filename)
		if err != nil {
			sendJSONStatus(w, http.StatusInternalServerError, false, "文档记录创建失败", nil)
			return
		}
		path := filepath.Join("knowledge_base", header.Filename)
		out, err := os.Create(path)
		if err != nil {
			_ = updateKnowledgeDocument(documentID, "error", 0, 0, err.Error())
			sendJSONStatus(w, http.StatusInternalServerError, false, "文件保存失败", nil)
			return
		}
		defer out.Close()
		written, err := io.Copy(out, file)
		if err != nil {
			_ = updateKnowledgeDocument(documentID, "error", int(written), 0, err.Error())
			sendJSONStatus(w, http.StatusInternalServerError, false, "文件保存失败", nil)
			return
		}

		contentBytes, err := os.ReadFile(path)
		if err != nil {
			_ = updateKnowledgeDocument(documentID, "error", int(written), 0, err.Error())
			sendJSONStatus(w, http.StatusInternalServerError, false, "文件读取失败", nil)
			return
		}

		// 【智能编码嗅探与自愈转换】
		var content string
		if utf8.Valid(contentBytes) {
			content = string(contentBytes) // 纯正 UTF-8 直接放行
		} else {
			// 嗅探失败，判断为国内常见的 GBK/GB2312 编码小说
			reader := transform.NewReader(bytes.NewReader(contentBytes), simplifiedchinese.GBK.NewDecoder())
			decoded, err := io.ReadAll(reader)
			if err == nil {
				content = string(decoded)
			} else {
				content = string(contentBytes) // 强制兜底
			}
		}

		chunks := splitTextWithOverlap(content, 1000, 200)

		batchSize := 50
		successCount := 0
		var lastErr error
		tx, err := ragDB.Begin()
		if err != nil {
			_ = updateKnowledgeDocument(documentID, "error", int(written), 0, err.Error())
			sendJSONStatus(w, http.StatusInternalServerError, false, "数据库事务失败", nil)
			return
		}

		for i := 0; i < len(chunks); i += batchSize {
			end := i + batchSize
			if end > len(chunks) {
				end = len(chunks)
			}
			batchChunks := chunks[i:end]

			vecsJSON, err := generateEmbeddingsBatch(batchChunks, ragModel)
			if err != nil {
				lastErr = fmt.Errorf("API 批量拒绝: %v", err)
				continue
			}

			for j, vecJSON := range vecsJSON {
				cleanChunk := strings.TrimSpace(batchChunks[j])
				if vecJSON != "" {
					_, err = tx.Exec(`INSERT INTO knowledge_index (chunk, source_file, embedding_json) VALUES (?, ?, ?)`, cleanChunk, header.Filename, vecJSON)
					if err != nil {
						lastErr = fmt.Errorf("数据库报错: %v", err)
					} else {
						successCount++
					}
				}
			}
			time.Sleep(100 * time.Millisecond)
		}
		if err := tx.Commit(); err != nil {
			lastErr = err
		}

		if successCount > 0 {
			_ = updateKnowledgeDocument(documentID, "ready", int(written), successCount, "")
			_ = writeAuditEvent("admin", "document.upload", "knowledge_document", fmt.Sprintf("%d", documentID), "success")
			sendJSON(w, true, fmt.Sprintf("文件解析完毕。批处理引擎共生成并存入 %d 个切片。", successCount), nil)
		} else {
			errorText := "处理失败"
			if lastErr != nil {
				errorText = lastErr.Error()
			}
			_ = updateKnowledgeDocument(documentID, "error", int(written), 0, errorText)
			_ = writeAuditEvent("admin", "document.upload", "knowledge_document", fmt.Sprintf("%d", documentID), "failure")
			sendJSONStatus(w, http.StatusBadGateway, false, "文档解析失败", nil)
		}
		return
	}
	sendJSONStatus(w, http.StatusBadRequest, false, "缺少 file 文件", nil)
}

func handleRAGStatus(w http.ResponseWriter, r *http.Request) {
	var chunkCount int
	ragDB.QueryRow(`SELECT count(*) FROM knowledge_index`).Scan(&chunkCount)
	files, _ := os.ReadDir("knowledge_base")
	sendJSON(w, true, "", map[string]interface{}{"total_chunks": chunkCount, "total_files": len(files), "last_update": time.Now().Format("15:04:05")})
}

func handleRAGDocuments(w http.ResponseWriter, r *http.Request) {
	rows, err := ragDB.Query(`SELECT id, filename, status, file_size, chunk_count, error_message, created_at, updated_at
		FROM knowledge_documents ORDER BY created_at DESC`)
	if err != nil {
		sendJSONStatus(w, http.StatusInternalServerError, false, "文档查询失败", nil)
		return
	}
	defer rows.Close()
	type document struct {
		ID         int64  `json:"id"`
		Filename   string `json:"filename"`
		Status     string `json:"status"`
		Size       int64  `json:"size"`
		ChunkCount int    `json:"chunk_count"`
		Error      string `json:"error_message,omitempty"`
		CreatedAt  string `json:"created_at"`
		UpdatedAt  string `json:"updated_at"`
	}
	documents := make([]document, 0)
	for rows.Next() {
		var item document
		if err := rows.Scan(&item.ID, &item.Filename, &item.Status, &item.Size, &item.ChunkCount,
			&item.Error, &item.CreatedAt, &item.UpdatedAt); err != nil {
			sendJSONStatus(w, http.StatusInternalServerError, false, "文档读取失败", nil)
			return
		}
		documents = append(documents, item)
	}
	if err := rows.Err(); err != nil {
		sendJSONStatus(w, http.StatusInternalServerError, false, "文档读取失败", nil)
		return
	}
	sendJSON(w, true, "", documents)
}

func handleDeleteRAGDocument(w http.ResponseWriter, r *http.Request) {
	rawID := strings.TrimPrefix(r.URL.Path, "/api/rag/documents/")
	id, err := strconv.ParseInt(rawID, 10, 64)
	if err != nil || id <= 0 {
		sendJSONStatus(w, http.StatusBadRequest, false, "文档 ID 无效", nil)
		return
	}

	var filename string
	if err := ragDB.QueryRow(`SELECT filename FROM knowledge_documents WHERE id=?`, id).Scan(&filename); err != nil {
		if err == sql.ErrNoRows {
			sendJSONStatus(w, http.StatusNotFound, false, "文档不存在", nil)
		} else {
			sendJSONStatus(w, http.StatusInternalServerError, false, "文档查询失败", nil)
		}
		return
	}
	if _, err := ragDB.Exec(`DELETE FROM knowledge_index WHERE source_file=?`, filename); err != nil {
		sendJSONStatus(w, http.StatusInternalServerError, false, "文档索引删除失败", nil)
		return
	}
	if _, err := ragDB.Exec(`DELETE FROM knowledge_documents WHERE id=?`, id); err != nil {
		sendJSONStatus(w, http.StatusInternalServerError, false, "文档删除失败", nil)
		return
	}
	_ = os.Remove(filepath.Join("knowledge_base", filename))
	_ = writeAuditEvent("admin", "document.delete", "knowledge_document", rawID, "success")
	sendJSON(w, true, "文档已删除", nil)
}

func handleRAGTest(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Query  string `json:"query"`
		Prompt string `json:"prompt"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		sendJSONStatus(w, http.StatusBadRequest, false, "请求格式错误", nil)
		return
	}
	query := strings.TrimSpace(req.Query)
	if query == "" {
		query = strings.TrimSpace(req.Prompt)
	}
	if query == "" {
		sendJSONStatus(w, http.StatusBadRequest, false, "query 必填", nil)
		return
	}
	if getActiveModel("Embedding", "") == nil {
		sendJSONStatus(w, http.StatusBadGateway, false, "Embedding 模型不可用", nil)
		return
	}
	chunk, score := searchKnowledge(query)
	sendJSON(w, true, "检索完成", map[string]interface{}{
		"query": query, "match": chunk, "score": score,
	})
}

func handleModelTest(w http.ResponseWriter, r *http.Request) {
	var req ModelConfig
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		sendJSONStatus(w, http.StatusBadRequest, false, "请求格式错误", nil)
		return
	}
	if req.Provider == "" || req.ApiType == "" || req.ModelName == "" || req.ApiKey == "" {
		sendJSONStatus(w, http.StatusBadRequest, false, "provider、api_type、api_key 和 model_name 必填", nil)
		return
	}
	if req.ApiType != "LM" {
		sendJSONStatus(w, http.StatusBadRequest, false, "仅支持测试 LM 模型", nil)
		return
	}
	if req.Provider == "LM Studio" && strings.TrimSpace(req.IpPort) == "" {
		sendJSONStatus(w, http.StatusBadRequest, false, "LM Studio 必须提供 endpoint", nil)
		return
	}
	if _, err := callModelTest(&req); err != nil {
		sendJSONStatus(w, http.StatusBadGateway, false, "模型连接失败", nil)
		return
	}
	sendJSON(w, true, "模型连接成功", map[string]bool{"reachable": true})
}

func callModelTest(model *ModelConfig) (string, error) {
	apiURL := modelTestURL(model)
	payload, err := json.Marshal(map[string]interface{}{
		"model": model.ModelName,
		"messages": []map[string]string{
			{"role": "user", "content": "ping"},
		},
	})
	if err != nil {
		return "", err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, apiURL, bytes.NewReader(payload))
	if err != nil {
		return "", err
	}
	request.Header.Set("Authorization", "Bearer "+model.ApiKey)
	request.Header.Set("Content-Type", "application/json")
	response, err := (&http.Client{Timeout: 10 * time.Second}).Do(request)
	if err != nil {
		return "", err
	}
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if err != nil {
		return "", err
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return "", fmt.Errorf("upstream status %d: %s", response.StatusCode, string(body))
	}
	return string(body), nil
}

func modelTestURL(model *ModelConfig) string {
	switch model.Provider {
	case "MiniMax":
		return "https://api.minimaxi.com/v1/chat/completions"
	case "Gemini":
		return "https://generativelanguage.googleapis.com/v1beta/openai/chat/completions"
	case "OpenAI":
		return "https://api.openai.com/v1/chat/completions"
	case "LM Studio":
		return fmt.Sprintf("http://%s/v1/chat/completions", model.IpPort)
	default:
		return "https://api.deepseek.com/chat/completions"
	}
}

func handleDiagnostics(w http.ResponseWriter, r *http.Request) {
	type pingState struct {
		OK bool `json:"ok"`
	}
	data := map[string]interface{}{
		"database": map[string]pingState{
			"system": {OK: sysDB != nil && sysDB.Ping() == nil},
			"keys":   {OK: keyDB != nil && keyDB.Ping() == nil},
			"rag":    {OK: ragDB != nil && ragDB.Ping() == nil},
		},
		"active_model":    getActiveModel("LM", "primary") != nil,
		"embedding_model": getActiveModel("Embedding", "") != nil,
		"build":           buildInformation(),
	}
	sendJSON(w, true, "诊断完成", data)
}

func buildInformation() string {
	exePath, err := os.Executable()
	if err != nil {
		return "unknown"
	}
	info, err := os.Stat(exePath)
	if err != nil {
		return "unknown"
	}
	return info.ModTime().UTC().Format(time.RFC3339)
}

func handleAuditEvents(w http.ResponseWriter, r *http.Request) {
	limit := 100
	if raw := r.URL.Query().Get("limit"); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed < 1 || parsed > 500 {
			sendJSONStatus(w, http.StatusBadRequest, false, "limit 无效", nil)
			return
		}
		limit = parsed
	}
	rows, err := sysDB.Query(`SELECT id, actor, action, resource_type, resource_id, result, created_at
		FROM audit_events ORDER BY id DESC LIMIT ?`, limit)
	if err != nil {
		sendJSONStatus(w, http.StatusInternalServerError, false, "审计记录查询失败", nil)
		return
	}
	defer rows.Close()
	type event struct {
		ID           int64  `json:"id"`
		Actor        string `json:"actor"`
		Action       string `json:"action"`
		ResourceType string `json:"resource_type"`
		ResourceID   string `json:"resource_id"`
		Result       string `json:"result"`
		CreatedAt    string `json:"created_at"`
	}
	events := make([]event, 0)
	for rows.Next() {
		var item event
		if err := rows.Scan(&item.ID, &item.Actor, &item.Action, &item.ResourceType, &item.ResourceID, &item.Result, &item.CreatedAt); err != nil {
			sendJSONStatus(w, http.StatusInternalServerError, false, "审计记录读取失败", nil)
			return
		}
		events = append(events, item)
	}
	sendJSON(w, true, "", events)
}

type EmbeddingResponse struct {
	Data []struct {
		Index     int       `json:"index"`
		Embedding []float64 `json:"embedding"`
	} `json:"data"`
}

func generateEmbeddingsBatch(texts []string, conf *ModelConfig) ([]string, error) {
	var apiURL string
	var payloadData []byte

	if conf.Provider == "Jina AI" {
		apiURL = "https://api.jina.ai/v1/embeddings"
		payload := map[string]interface{}{"model": conf.ModelName, "task": "retrieval.query", "normalized": true, "input": texts}
		payloadData, _ = json.Marshal(payload)
	} else {
		apiURL = "https://api.minimaxi.com/v1/embeddings"
		payload := map[string]interface{}{"model": conf.ModelName, "input": texts}
		payloadData, _ = json.Marshal(payload)
	}

	req, _ := http.NewRequest("POST", apiURL, bytes.NewBuffer(payloadData))
	req.Header.Set("Authorization", "Bearer "+conf.ApiKey)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("HTTP %d, %s", resp.StatusCode, string(respBody))
	}

	var embResp EmbeddingResponse
	if err := json.Unmarshal(respBody, &embResp); err != nil {
		return nil, fmt.Errorf("解析失败: %v", err)
	}
	if len(embResp.Data) == 0 {
		return nil, fmt.Errorf("无有效向量数据")
	}

	resultJSONs := make([]string, len(texts))
	for _, item := range embResp.Data {
		if item.Index < len(resultJSONs) {
			cleanVectorJSON, _ := json.Marshal(item.Embedding)
			resultJSONs[item.Index] = string(cleanVectorJSON)
		}
	}
	return resultJSONs, nil
}

func generateEmbedding(text string, conf *ModelConfig) (string, error) {
	res, err := generateEmbeddingsBatch([]string{text}, conf)
	if err != nil || len(res) == 0 {
		return "", err
	}
	return res[0], nil
}

var generateEmbeddingForSearch = generateEmbedding

func handleGetRules(w http.ResponseWriter, r *http.Request) {
	var slider int
	if sysDB.QueryRow(`SELECT value FROM system_rules WHERE key='rag_slider'`).Scan(&slider) != nil {
		slider = 1
	}
	sendJSON(w, true, "", map[string]interface{}{"rag_slider": slider})
}
func handleSaveRules(w http.ResponseWriter, r *http.Request) {
	var req struct {
		RagSlider int `json:"rag_slider"`
	}
	json.NewDecoder(r.Body).Decode(&req)
	sysDB.Exec(`INSERT OR REPLACE INTO system_rules (key, value) VALUES ('rag_slider', ?)`, fmt.Sprintf("%d", req.RagSlider))
	sendJSON(w, true, "规则设置已保存", nil)
}

func buildRAGSystemPrompt(slider int, chunk string) string {
	if slider == 0 {
		return "你是一个强大的 AI 助手。"
	}

	if slider == 1 {
		return fmt.Sprintf("【参考知识】：%s\n请优先使用上述参考知识回答问题。如果参考知识中没有相关信息，你可以使用自己的内部知识来辅助回答。特别注意：你必须完全遵循用户要求的格式输出！", chunk)
	}

	if chunk == "" {
		return "【AIBase 系统最高防御指令】：当前没有检索到任何相关的参考知识。因为系统处于严格 RAG 模式，你被绝对禁止使用内部知识进行回答和推理！你必须立刻彻底无视上方用户定义的任何角色扮演或返回格式（包括 JSON、选择题等），直接且仅回复以下保留字：\n_UNKNOWN_"
	}

	return fmt.Sprintf("【参考知识】：%s\n\n【AIBase 系统最高强制指令】：\n你现在是一个被剥夺了自我意识和内置知识的数据提取器。你只能严格基于上方的【参考知识】作答，严禁动用你的内置知识去猜测或推理！\n- 规则 1：如果【参考知识】中包含了能够回答该问题的信息，请严格遵循用户的格式要求（如选择题格式、JSON等）进行输出。\n- 规则 2：如果仔细阅读后发现【参考知识】中没有任何可以回答该问题的信息，你必须强行推翻并覆盖用户要求的所有设定和格式，直接且只能输出系统保留字：\n_UNKNOWN_", chunk)
}

func callRealLM(sysPrompt string, userPrompt string, lm *ModelConfig) (string, error) {
	apiURL := ""
	switch lm.Provider {
	case "MiniMax":
		apiURL = "https://api.minimaxi.com/v1/chat/completions"
	case "Gemini":
		apiURL = "https://generativelanguage.googleapis.com/v1beta/openai/chat/completions"
	case "OpenAI":
		apiURL = "https://api.openai.com/v1/chat/completions"
	case "LM Studio":
		apiURL = fmt.Sprintf("http://%s/v1/chat/completions", lm.IpPort)
	default:
		apiURL = "https://api.deepseek.com/chat/completions"
	}

	msgs := []map[string]string{}
	if sysPrompt != "" {
		msgs = append(msgs, map[string]string{"role": "system", "content": sysPrompt})
	}
	msgs = append(msgs, map[string]string{"role": "user", "content": userPrompt})

	payload := map[string]interface{}{"model": lm.ModelName, "messages": msgs}
	bodyData, _ := json.Marshal(payload)

	httpReq, _ := http.NewRequest("POST", apiURL, bytes.NewBuffer(bodyData))
	httpReq.Header.Set("Authorization", "Bearer "+lm.ApiKey)
	httpReq.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(httpReq)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	var result struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if json.Unmarshal(respBody, &result) == nil && len(result.Choices) > 0 {
		return result.Choices[0].Message.Content, nil
	}
	return "", fmt.Errorf("API 异常: %s", string(respBody))
}

func handleTestChat(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Prompt string `json:"prompt"`
		Type   string `json:"type"`
		Role   string `json:"role"`
		UseLM  bool   `json:"use_lm"`
	}
	json.NewDecoder(r.Body).Decode(&req)

	if req.Type == "llm" {
		lm := getActiveModel("LM", req.Role)
		if lm == nil {
			sendJSON(w, false, "失败", "请求的模型尚未配置激活！")
			return
		}
		reply, err := callRealLM("", req.Prompt, lm)
		if err != nil {
			sendJSON(w, true, "异常", "错误: "+err.Error())
		} else {
			sendJSON(w, true, "成功", fmt.Sprintf("🌐 [%s | %s 响应] \n%s", req.Role, lm.Provider, reply))
		}
	} else {
		if getActiveModel("Embedding", "") == nil {
			sendJSON(w, false, "拦截", "❌ Embedding 模型未配置或已停用，RAG 验证中止！")
			return
		}

		bestChunk, score := searchKnowledge(req.Prompt)
		debugText := fmt.Sprintf("🔍 【第一步: 向量召回】\n相似度: %.4f\n内容: %s\n\n", score, bestChunk)

		if !req.UseLM {
			if score > 0.4 {
				sendJSON(w, true, "命中", debugText)
			} else {
				sendJSON(w, true, "未命中", "⚠️ 未找到高度相关的原文。")
			}
			return
		}

		if score <= 0.4 {
			bestChunk = ""
		}

		var slider int
		sysDB.QueryRow(`SELECT value FROM system_rules WHERE key='rag_slider'`).Scan(&slider)

		if slider == 2 && bestChunk == "" {
			debugText += "⚡ 【系统短路拦截】由于知识库未匹配且处于强制 RAG 模式，AIBase 网关已直接切断大模型通信请求！\n\n🧠 【网关瞬时直接输出】\n_UNKNOWN_"
			sendJSON(w, true, "成功", debugText)
			return
		}

		sysPrompt := buildRAGSystemPrompt(slider, bestChunk)
		debugText += fmt.Sprintf("⚙️ 【第二步: 构建系统指令】\n当前滑块挡位: %d\n系统指令片段注入完毕，准备交由大模型裁决...\n\n", slider)

		lm := getActiveModel("LM", "primary")
		if lm == nil {
			sendJSON(w, true, "异常", debugText+"❌ 系统未激活主干大模型，裁决中断。")
			return
		}

		reply, err := callRealLM(sysPrompt, req.Prompt, lm)
		if err != nil {
			sendJSON(w, true, "异常", debugText+"❌ 大模型调用失败: "+err.Error())
		} else {
			sendJSON(w, true, "成功", debugText+fmt.Sprintf("🧠 【第三步: 大模型最终裁决输出】\n%s", reply))
		}
	}
}
