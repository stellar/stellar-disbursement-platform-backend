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

func SQLNullFloat64(f float64) sql.NullFloat64 {
	return sql.NullFloat64{
		Float64: f,
		Valid:   f != 0,
	}
}

func SQLNullTime(t time.Time) pq.NullTime {
	return pq.NullTime{
		Time:  t,
		Valid: !t.IsZero(),
	}
}
