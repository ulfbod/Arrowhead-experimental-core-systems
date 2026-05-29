package model_test

import (
	"testing"

	"arrowhead/core/internal/model"
)

func TestPaginateZeroRequestReturnsAll(t *testing.T) {
	items := []string{"c", "a", "b"}
	got, total := model.Paginate(items, model.PageRequest{}, func(s string) string { return s })
	if total != 3 {
		t.Errorf("total = %d, want 3", total)
	}
	if len(got) != 3 {
		t.Errorf("len = %d, want 3", len(got))
	}
}

func TestPaginateFirstPage(t *testing.T) {
	items := []string{"e", "d", "c", "b", "a"}
	got, total := model.Paginate(items, model.PageRequest{PageNumber: 0, PageSize: 2, PageDirection: "ASC"}, func(s string) string { return s })
	if total != 5 {
		t.Errorf("total = %d, want 5", total)
	}
	if len(got) != 2 {
		t.Errorf("page len = %d, want 2", len(got))
	}
	if got[0] != "a" || got[1] != "b" {
		t.Errorf("first page = %v, want [a b]", got)
	}
}

func TestPaginateLastPage(t *testing.T) {
	items := []string{"e", "d", "c", "b", "a"}
	got, total := model.Paginate(items, model.PageRequest{PageNumber: 2, PageSize: 2, PageDirection: "ASC"}, func(s string) string { return s })
	if total != 5 {
		t.Errorf("total = %d, want 5", total)
	}
	if len(got) != 1 {
		t.Errorf("last page len = %d, want 1", len(got))
	}
	if got[0] != "e" {
		t.Errorf("last page[0] = %q, want e", got[0])
	}
}

func TestPaginateBeyondEnd(t *testing.T) {
	items := []string{"a", "b"}
	got, total := model.Paginate(items, model.PageRequest{PageNumber: 5, PageSize: 2, PageDirection: "ASC"}, func(s string) string { return s })
	if total != 2 {
		t.Errorf("total = %d, want 2", total)
	}
	if len(got) != 0 {
		t.Errorf("beyond-end page len = %d, want 0", len(got))
	}
}

func TestPaginateSortDESC(t *testing.T) {
	items := []string{"a", "b", "c"}
	got, _ := model.Paginate(items, model.PageRequest{PageNumber: 0, PageSize: 2, PageDirection: "DESC"}, func(s string) string { return s })
	if got[0] != "c" || got[1] != "b" {
		t.Errorf("DESC page = %v, want [c b]", got)
	}
}
