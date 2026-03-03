package pagination

import (
	"net/http"
	"strconv"
)

type Pagination struct {
	Page  int `json:"page"`
	Limit int `json:"limit"`
}

func Parse(r *http.Request) Pagination {
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))

	if page <= 0 {
		page = 1
	}
	if limit <= 0 {
		limit = 10
	}

	return Pagination{page, limit}
}
