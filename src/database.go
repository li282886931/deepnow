// database.go - 数据库引擎与状态管理
// by szyok
package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"strings"
	"sync"
	"time"

	_ "modernc.org/sqlite"
)

var sysDB, keyDB, ragDB *sql.DB
var authKeys sync.Map
var keyUsageMu sync.Mutex

type ModelConfig struct {
	Provider  string
	ApiType   string
	ApiKey    string
	ModelName string
	IpPort    string
}

func initDatabases() error {
	var err error
	sysDB, err = sql.Open("sqlite", "system.db")
	if err != nil {
		return fmt.Errorf("打开 system.db 失败: %w", err)
	}
	if err := sysDB.Ping(); err != nil {
		return fmt.Errorf("连接 system.db 失败: %w", err)
	}
	if _, err := sysDB.Exec(`CREATE TABLE IF NOT EXISTS admin_users (username TEXT PRIMARY KEY, password TEXT)`); err != nil {
		return fmt.Errorf("创建 admin_users 失败: %w", err)
	}
	if _, err := sysDB.Exec(`CREATE TABLE IF NOT EXISTS model_configs (provider TEXT, api_type TEXT, api_key TEXT, model_name TEXT, ip_port TEXT, PRIMARY KEY(provider, api_type))`); err != nil {
		return fmt.Errorf("创建 model_configs 失败: %w", err)
	}
	if _, err := sysDB.Exec(`CREATE TABLE IF NOT EXISTS system_rules (key TEXT PRIMARY KEY, value TEXT)`); err != nil {
		return fmt.Errorf("创建 system_rules 失败: %w", err)
	}

	var c int
	if err := sysDB.QueryRow(`SELECT count(*) FROM admin_users`).Scan(&c); err != nil {
		return fmt.Errorf("读取管理员数量失败: %w", err)
	}
	if c == 0 {
		defaultPassword, err := hashPassword("12345")
		if err != nil {
			defaultPassword = "12345"
		}
		if _, err := sysDB.Exec(`INSERT INTO admin_users VALUES ('admin', ?)`, defaultPassword); err != nil {
			return fmt.Errorf("创建默认管理员失败: %w", err)
		}
	}

	keyDB, err = sql.Open("sqlite", "key.db")
	if err != nil {
		return fmt.Errorf("打开 key.db 失败: %w", err)
	}
	if err := keyDB.Ping(); err != nil {
		return fmt.Errorf("连接 key.db 失败: %w", err)
	}
	if err := migrateKeySchema(keyDB); err != nil {
		return fmt.Errorf("迁移 key.db 表失败: %w", err)
	}
	rows, err := keyDB.Query(`SELECT api_key, used FROM access_keys`)
	if err != nil {
		return fmt.Errorf("加载 access_keys 失败: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var k string
		var u int
		if err := rows.Scan(&k, &u); err != nil {
			return fmt.Errorf("读取 access_keys 失败: %w", err)
		}
		authKeys.Store(k, u)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("遍历 access_keys 失败: %w", err)
	}

	if err := os.MkdirAll("knowledge_base", 0755); err != nil {
		return fmt.Errorf("创建 knowledge_base 目录失败: %w", err)
	}
	ragDB, err = sql.Open("sqlite", "rag.db")
	if err != nil {
		return fmt.Errorf("打开 rag.db 失败: %w", err)
	}
	if err := ragDB.Ping(); err != nil {
		return fmt.Errorf("连接 rag.db 失败: %w", err)
	}
	if _, err := ragDB.Exec(`CREATE TABLE IF NOT EXISTS knowledge_index (id INTEGER PRIMARY KEY, chunk TEXT, source_file TEXT, embedding_json TEXT)`); err != nil {
		return fmt.Errorf("创建 knowledge_index 失败: %w", err)
	}
	if _, err := ragDB.Exec(`ALTER TABLE knowledge_index ADD COLUMN embedding_json TEXT`); err != nil && !strings.Contains(strings.ToLower(err.Error()), "duplicate column") {
		return fmt.Errorf("迁移 knowledge_index 失败: %w", err)
	}
	if err := migrateRAGSchema(ragDB); err != nil {
		return fmt.Errorf("迁移 rag.db 表失败: %w", err)
	}
	if err := migrateSystemSchema(sysDB); err != nil {
		return fmt.Errorf("迁移 system.db 表失败: %w", err)
	}

	autoActivateModels()
	return nil
}

func columnExists(db *sql.DB, table, column string) (bool, error) {
	var count int
	if err := db.QueryRow(`SELECT count(*) FROM pragma_table_info(?) WHERE name=?`, table, column).Scan(&count); err != nil {
		return false, err
	}
	return count > 0, nil
}

func addColumnIfMissing(db *sql.DB, table, column, definition string) error {
	exists, err := columnExists(db, table, column)
	if err != nil {
		return err
	}
	if exists {
		return nil
	}
	_, err = db.Exec(`ALTER TABLE ` + table + ` ADD COLUMN ` + column + ` ` + definition)
	return err
}

func migrateKeySchema(db *sql.DB) error {
	if db == nil {
		return fmt.Errorf("database is nil")
	}
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS access_keys (
		api_key TEXT PRIMARY KEY,
		used INTEGER NOT NULL DEFAULT 0,
		created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
	)`); err != nil {
		return err
	}
	for _, column := range []struct {
		name, definition string
	}{
		{"name", "TEXT NOT NULL DEFAULT ''"},
		{"status", "TEXT NOT NULL DEFAULT 'active'"},
		{"last_used_at", "DATETIME"},
	} {
		if err := addColumnIfMissing(db, "access_keys", column.name, column.definition); err != nil {
			return fmt.Errorf("add access_keys.%s: %w", column.name, err)
		}
	}
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS key_usage_daily (
		api_key TEXT NOT NULL,
		usage_date TEXT NOT NULL,
		request_count INTEGER NOT NULL DEFAULT 0,
		input_tokens INTEGER NOT NULL DEFAULT 0,
		output_tokens INTEGER NOT NULL DEFAULT 0,
		error_count INTEGER NOT NULL DEFAULT 0,
		PRIMARY KEY (api_key, usage_date)
	)`); err != nil {
		return err
	}
	return nil
}

func migrateRAGSchema(db *sql.DB) error {
	if db == nil {
		return fmt.Errorf("database is nil")
	}
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS knowledge_documents (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		filename TEXT NOT NULL,
		status TEXT NOT NULL,
		file_size INTEGER NOT NULL DEFAULT 0,
		chunk_count INTEGER NOT NULL DEFAULT 0,
		error_message TEXT NOT NULL DEFAULT '',
		created_at DATETIME NOT NULL,
		updated_at DATETIME NOT NULL
	)`); err != nil {
		return err
	}
	return nil
}

func migrateSystemSchema(db *sql.DB) error {
	if db == nil {
		return fmt.Errorf("database is nil")
	}
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS audit_events (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		actor TEXT NOT NULL,
		action TEXT NOT NULL,
		resource_type TEXT NOT NULL,
		resource_id TEXT NOT NULL DEFAULT '',
		result TEXT NOT NULL,
		created_at DATETIME NOT NULL
	)`); err != nil {
		return err
	}
	return nil
}

func recordKeyUsage(apiKey string, inputTokens, outputTokens int, failed bool) error {
	if keyDB == nil {
		return fmt.Errorf("key database is nil")
	}
	usageDate := time.Now().UTC().Format("2006-01-02")
	errorCount := 0
	if failed {
		errorCount = 1
	}
	_, err := keyDB.Exec(`INSERT INTO key_usage_daily
		(api_key, usage_date, request_count, input_tokens, output_tokens, error_count)
		VALUES (?, ?, 1, ?, ?, ?)
		ON CONFLICT(api_key, usage_date) DO UPDATE SET
			request_count = request_count + 1,
			input_tokens = input_tokens + excluded.input_tokens,
			output_tokens = output_tokens + excluded.output_tokens,
			error_count = error_count + excluded.error_count`,
		apiKey, usageDate, inputTokens, outputTokens, errorCount)
	return err
}

func writeAuditEvent(actor, action, resourceType, resourceID, result string) error {
	if sysDB == nil {
		return fmt.Errorf("system database is nil")
	}
	_, err := sysDB.Exec(`INSERT INTO audit_events
		(actor, action, resource_type, resource_id, result, created_at)
		VALUES (?, ?, ?, ?, ?, ?)`,
		actor, action, resourceType, resourceID, result, time.Now().UTC())
	return err
}

func createKnowledgeDocument(filename string) (int64, error) {
	now := time.Now().UTC()
	result, err := ragDB.Exec(`INSERT INTO knowledge_documents
		(filename, status, created_at, updated_at) VALUES (?, 'processing', ?, ?)`,
		filename, now, now)
	if err != nil {
		return 0, err
	}
	return result.LastInsertId()
}

func updateKnowledgeDocument(id int64, status string, fileSize, chunkCount int, errorMessage string) error {
	if len(errorMessage) > 500 {
		errorMessage = errorMessage[:500]
	}
	_, err := ragDB.Exec(`UPDATE knowledge_documents
		SET status=?, file_size=?, chunk_count=?, error_message=?, updated_at=?
		WHERE id=?`, status, fileSize, chunkCount, errorMessage, time.Now().UTC(), id)
	return err
}

func checkModelValid(apiType, provider string) bool {
	if provider == "" || provider == "none" {
		return false
	}
	var k, m string
	err := sysDB.QueryRow(`SELECT api_key, model_name FROM model_configs WHERE api_type=? AND provider=?`, apiType, provider).Scan(&k, &m)
	return err == nil && k != "" && m != ""
}

func autoActivateModels() {
	fmt.Println("--- [算力挂载状态] ---")

	var activePri string
	errPri := sysDB.QueryRow(`SELECT value FROM system_rules WHERE key='active_lm_pri'`).Scan(&activePri)

	if errPri != nil || activePri == "" {
		sysDB.QueryRow(`SELECT provider FROM model_configs WHERE api_type='LM' AND api_key!='' AND model_name!='' LIMIT 1`).Scan(&activePri)
		if activePri != "" {
			sysDB.Exec(`INSERT INTO system_rules (key, value) VALUES ('active_lm_pri', ?)`, activePri)
		}
	} else if activePri != "none" && !checkModelValid("LM", activePri) {
		activePri = ""
		sysDB.QueryRow(`SELECT provider FROM model_configs WHERE api_type='LM' AND api_key!='' AND model_name!='' LIMIT 1`).Scan(&activePri)
		if activePri != "" {
			sysDB.Exec(`UPDATE system_rules SET value=? WHERE key='active_lm_pri'`, activePri)
		} else {
			sysDB.Exec(`UPDATE system_rules SET value='none' WHERE key='active_lm_pri'`)
		}
	}

	if activePri != "" && activePri != "none" {
		var mName string
		sysDB.QueryRow(`SELECT model_name FROM model_configs WHERE api_type='LM' AND provider=?`, activePri).Scan(&mName)
		fmt.Printf("✅ 大模型 [%s] (%s) (Primary) 现在启用\n", activePri, mName)
	} else {
		fmt.Println("❌ 当前无可用的大模型 (Primary LM)！引擎无法工作。")
	}

	var activeSec string
	errSec := sysDB.QueryRow(`SELECT value FROM system_rules WHERE key='active_lm_sec'`).Scan(&activeSec)
	if errSec == nil && activeSec != "none" && !checkModelValid("LM", activeSec) {
		sysDB.Exec(`UPDATE system_rules SET value='none' WHERE key='active_lm_sec'`)
		activeSec = "none"
	}
	if activeSec != "" && activeSec != "none" {
		var mName string
		sysDB.QueryRow(`SELECT model_name FROM model_configs WHERE api_type='LM' AND provider=?`, activeSec).Scan(&mName)
		fmt.Printf("✅ 大模型 [%s] (%s) (Slave) 现在启用\n", activeSec, mName)
	}

	var activeRAG string
	errRag := sysDB.QueryRow(`SELECT value FROM system_rules WHERE key='active_embedding'`).Scan(&activeRAG)

	if errRag != nil || activeRAG == "" {
		sysDB.QueryRow(`SELECT provider FROM model_configs WHERE api_type='Embedding' AND api_key!='' AND model_name!='' LIMIT 1`).Scan(&activeRAG)
		if activeRAG != "" {
			sysDB.Exec(`INSERT INTO system_rules (key, value) VALUES ('active_embedding', ?)`, activeRAG)
		}
	} else if activeRAG != "none" && !checkModelValid("Embedding", activeRAG) {
		activeRAG = ""
		sysDB.QueryRow(`SELECT provider FROM model_configs WHERE api_type='Embedding' AND api_key!='' AND model_name!='' LIMIT 1`).Scan(&activeRAG)
		if activeRAG != "" {
			sysDB.Exec(`UPDATE system_rules SET value=? WHERE key='active_embedding'`, activeRAG)
		} else {
			sysDB.Exec(`UPDATE system_rules SET value='none' WHERE key='active_embedding'`)
		}
	}

	if activeRAG != "" && activeRAG != "none" {
		var mName string
		sysDB.QueryRow(`SELECT model_name FROM model_configs WHERE api_type='Embedding' AND provider=?`, activeRAG).Scan(&mName)
		fmt.Printf("✅ Embedding 模型 [%s] (%s) 现在启用\n", activeRAG, mName)
	} else {
		sysDB.Exec(`UPDATE system_rules SET value='0' WHERE key='rag_slider'`)
		fmt.Println("⚠️ Embedding 模型现在停用 (纯大模型模式，底层规则已强制归零)")
	}
	fmt.Println("--------------------------")
}

func getActiveModel(apiType string, role string) *ModelConfig {
	var provider string
	keyStr := "active_lm_pri"
	if apiType == "LM" && role == "slave" {
		keyStr = "active_lm_sec"
	}
	if apiType == "Embedding" {
		keyStr = "active_embedding"
	}

	err := sysDB.QueryRow(`SELECT value FROM system_rules WHERE key=?`, keyStr).Scan(&provider)
	if err != nil || provider == "" || provider == "none" {
		return nil
	}

	var conf ModelConfig
	err = sysDB.QueryRow(`SELECT provider, api_type, api_key, model_name, ip_port FROM model_configs WHERE provider=? AND api_type=?`, provider, apiType).Scan(
		&conf.Provider, &conf.ApiType, &conf.ApiKey, &conf.ModelName, &conf.IpPort)
	if err != nil || conf.ApiKey == "" || conf.ModelName == "" {
		return nil
	}
	return &conf
}

func cosineSimilarity(a, b []float64) float64 {
	var dotProduct, normA, normB float64
	for i := 0; i < len(a) && i < len(b); i++ {
		dotProduct += a[i] * b[i]
		normA += a[i] * a[i]
		normB += b[i] * b[i]
	}
	if normA == 0 || normB == 0 {
		return 0
	}
	return dotProduct / (math.Sqrt(normA) * math.Sqrt(normB))
}

func searchKnowledge(query string) (string, float64) {
	activeRAG := getActiveModel("Embedding", "")
	if activeRAG == nil {
		return "", 0
	}

	queryVecJSON, err := generateEmbeddingForSearch(query, activeRAG)
	if err != nil {
		return "", 0
	}

	var queryVec []float64
	json.Unmarshal([]byte(queryVecJSON), &queryVec)

	rows, err := ragDB.Query(`SELECT chunk, embedding_json FROM knowledge_index WHERE embedding_json IS NOT NULL`)
	if err != nil {
		return "", 0
	}
	defer rows.Close()

	var bestChunk string
	var maxSim float64 = -1.0

	for rows.Next() {
		var chunk, embJSON string
		rows.Scan(&chunk, &embJSON)

		var chunkVec []float64
		if json.Unmarshal([]byte(embJSON), &chunkVec) == nil {
			sim := cosineSimilarity(queryVec, chunkVec)
			if sim > maxSim {
				maxSim = sim
				bestChunk = chunk
			}
		}
	}
	return bestChunk, maxSim
}
