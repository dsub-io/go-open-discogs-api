package catalog

import "testing"

func TestCursorPageConstruction(t *testing.T) {
	t.Parallel()
	request := PageRequest{AfterID: 40, Size: 20}
	if request.FetchSize() != 21 {
		t.Fatalf("fetch size=%d", request.FetchSize())
	}
	page := NewPage([]Artist{{ID: 41}, {ID: 42}}, 1)
	next := page.NextAfterID()
	if !page.HasMore || len(page.Items) != 1 || next == nil || *next != 41 {
		t.Fatalf("page=%+v next=%v", page, next)
	}
	final := NewPage([]Artist{{ID: 42}}, 1)
	if final.HasMore || final.NextAfterID() != nil {
		t.Fatalf("final page=%+v", final)
	}
	assertCursorItem(t, ArtistRelease{ID: 1}, 1)
	assertCursorItem(t, Label{ID: 2}, 2)
	assertCursorItem(t, LabelRelease{ID: 3}, 3)
	assertCursorItem(t, Master{ID: 4}, 4)
	assertCursorItem(t, MasterRelease{ID: 5}, 5)
	assertCursorItem(t, Release{ID: 6}, 6)
}

func assertCursorItem[T PageItem](t *testing.T, item T, expectedID int64) {
	t.Helper()
	page := NewPage([]T{item, item}, 1)
	next := page.NextAfterID()
	if next == nil || *next != expectedID {
		t.Fatalf("next=%v expected=%d", next, expectedID)
	}
}
