package mysql

import (
	"database/sql"

	_ "github.com/go-sql-driver/mysql"
)

func Open(dsn string) (*sql.DB, error) {
	return sql.Open("mysql", dsn)
}
