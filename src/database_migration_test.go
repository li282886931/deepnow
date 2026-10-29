package main

import (
	"database/sql"
	"testing"

	_ "modernc.org/sqlite"
)

func openTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func createCurrentSchema(t *testing.T, db *sql.DB) {
	t.Helper()
	mustExec(t, db, `CREATE TABLE access_keys (
		api_key TEXT PRIMARY KEY,
		used INTEGER,
		created_at DATETIME
	)`)
}

func assertColumnExists(t *testing.T, db *sql.DB, table, column string) {
	t.Helper()
	var count int
	if err := db.QueryRow(`SELECT count(*) FROM pragma_table_info(?) WHERE name=?`, table, column).Scan(&count); err != nil {
		t.Fatalf("inspect %s.%s: %v", table, column, err)
	}
	if count != 1 {
		t.Fatalf("column %s.%s does not exist", table, column)
	}
}

func assertTableExists(t *testing.T, db *sql.DB, table string) {
	t.Helper()
	var count int
	if err := db.QueryRow(`SELECT count(*) FROM sqlite_master WHERE type='table' AND name=?`, table).Scan(&count); err != nil {
		t.Fatalf("inspect table %s: %v", table, err)
	}
	if count != 1 {
		t.Fatalf("table %s does not exist", table)
	}
}

func assertTableAbsent(t *testing.T, db *sql.DB, table string) {
	t.Helper()
	var count int
	if err := db.QueryRow(`SELECT count(*) FROM sqlite_master WHERE type='table' AND name=?`, table).Scan(&count); err != nil {
		t.Fatalf("inspect table %s: %v", table, err)
	}
	if count != 0 {
		t.Fatalf("table %s should not exist", table)
	}
}

func TestMigrationHelpersOwnOnlyTheirDatabaseSchemas(t *testing.T) {
	keyDB := openTestDB(t)
	if err := migrateKeySchema(keyDB); err != nil {
		t.Fatal(err)
	}
	assertTableExists(t, keyDB, "access_keys")
	assertTableExists(t, keyDB, "key_usage_daily")
	assertTableAbsent(t, keyDB, "knowledge_documents")
	assertTableAbsent(t, keyDB, "audit_events")

	ragDB := openTestDB(t)
	if err := migrateRAGSchema(ragDB); err != nil {
		t.Fatal(err)
	}
	assertTableExists(t, ragDB, "knowledge_documents")
	assertTableAbsent(t, ragDB, "access_keys")
	assertTableAbsent(t, ragDB, "key_usage_daily")
	assertTableAbsent(t, ragDB, "audit_events")

	sysDB := openTestDB(t)
	if err := migrateSystemSchema(sysDB); err != nil {
		t.Fatal(err)
	}
	assertTableExists(t, sysDB, "audit_events")
	assertTableAbsent(t, sysDB, "access_keys")
	assertTableAbsent(t, sysDB, "key_usage_daily")
	assertTableAbsent(t, sysDB, "knowledge_documents")
}

func TestMigrateKeySchemaOnFreshDatabase(t *testing.T) {
	db := openTestDB(t)

	if err := migrateKeySchema(db); err != nil {
		t.Fatal(err)
	}

	assertTableExists(t, db, "access_keys")
	assertTableExists(t, db, "key_usage_daily")
	assertColumnExists(t, db, "access_keys", "name")
	assertColumnExists(t, db, "access_keys", "status")
	assertColumnExists(t, db, "access_keys", "last_used_at")
}

func TestMigrateSchemasAreIdempotent(t *testing.T) {
	db := openTestDB(t)
	createCurrentSchema(t, db)

	if err := migrateKeySchema(db); err != nil {
		t.Fatal(err)
	}
	if err := migrateKeySchema(db); err != nil {
		t.Fatal(err)
	}

	assertColumnExists(t, db, "access_keys", "name")
	assertColumnExists(t, db, "access_keys", "status")
	assertTableExists(t, db, "key_usage_daily")

	ragDB := openTestDB(t)
	if err := migrateRAGSchema(ragDB); err != nil {
		t.Fatal(err)
	}
	if err := migrateRAGSchema(ragDB); err != nil {
		t.Fatal(err)
	}
	assertTableExists(t, ragDB, "knowledge_documents")

	sysDB := openTestDB(t)
	if err := migrateSystemSchema(sysDB); err != nil {
		t.Fatal(err)
	}
	if err := migrateSystemSchema(sysDB); err != nil {
		t.Fatal(err)
	}
	assertTableExists(t, sysDB, "audit_events")
}

func TestRecordKeyUsageUpdatesDailyCounters(t *testing.T) {
	db := openTestDB(t)
	if _, err := db.Exec(`CREATE TABLE access_keys (api_key TEXT PRIMARY KEY, used INTEGER, created_at DATETIME)`); err != nil {
		t.Fatal(err)
	}
	if err := migrateKeySchema(db); err != nil {
		t.Fatal(err)
	}

	original := keyDB
	keyDB = db
	t.Cleanup(func() { keyDB = original })

	if err := recordKeyUsage("sk-test", 12, 8, false); err != nil {
		t.Fatal(err)
	}
	if err := recordKeyUsage("sk-test", 3, 2, true); err != nil {
		t.Fatal(err)
	}

	var requests, input, output, errors int
	err := db.QueryRow(`SELECT request_count, input_tokens, output_tokens, error_count
		FROM key_usage_daily WHERE api_key=?`, "sk-test").
		Scan(&requests, &input, &output, &errors)
	if err != nil {
		t.Fatal(err)
	}
	if requests != 2 || input != 15 || output != 10 || errors != 1 {
		t.Fatalf("usage = %d/%d/%d/%d", requests, input, output, errors)
	}
}

func TestWriteAuditEventPersistsRecord(t *testing.T) {
	db := openTestDB(t)
	if err := migrateSystemSchema(db); err != nil {
		t.Fatal(err)
	}

	original := sysDB
	sysDB = db
	t.Cleanup(func() { sysDB = original })

	if err := writeAuditEvent("admin", "model.save", "model", "openai/LM", "success"); err != nil {
		t.Fatal(err)
	}

	var actor, action, resourceType, resourceID, result string
	if err := db.QueryRow(`SELECT actor, action, resource_type, resource_id, result
		FROM audit_events`).Scan(&actor, &action, &resourceType, &resourceID, &result); err != nil {
		t.Fatal(err)
	}
	if actor != "admin" || action != "model.save" || resourceType != "model" ||
		resourceID != "openai/LM" || result != "success" {
		t.Fatalf("audit = %q/%q/%q/%q/%q", actor, action, resourceType, resourceID, result)
	}
}
