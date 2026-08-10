package catalog

import "testing"

func TestPageRequestOffset(t *testing.T) {
	t.Parallel()
	request := PageRequest{Page: 3, Size: 20}
	if request.Offset() != 40 {
		t.Fatalf("offset=%d", request.Offset())
	}
}
