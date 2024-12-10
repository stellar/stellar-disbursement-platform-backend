package utils

import (
	"database/sql"
	"time"

	"github.com/lib/pq"
)

func SQLNullString(s string) sql.NullString {
	return sql.NullString{
		String: s,
		Valid:  s != "",
	}
}

func SQLNullTime(t time.Time) pq.NullTime {
	return pq.NullTime{
		Time:  t,
		Valid: !t.IsZero(),
	}
}
