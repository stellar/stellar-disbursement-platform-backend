package utils

type ResultWithTotal struct {
	Total  int
	Result interface{}
}

func NewResultWithTotal(total int, result interface{}) *ResultWithTotal {
	return &ResultWithTotal{
		Total:  total,
		Result: result,
	}
}
