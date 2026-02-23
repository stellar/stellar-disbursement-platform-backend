package utils

import (
	"database/sql"
	"time"

	"github.com/shopspring/decimal"
)

func SQLNullString(s string) sql.NullString {
	return sql.NullString{
		String: s,
		Valid:  s != "",
	}
}

func SQLNullNumeric(d decimal.Decimal) sql.NullString {
	return sql.NullString{
		String: d.String(),
		Valid:  d.Sign() != 0,
	}
}

func SQLNullTime(t time.Time) sql.NullTime {
	return sql.NullTime{
		Time:  t,
		Valid: !t.IsZero(),
	}
}
