package webconsole

import "testing"

func TestReportsDefaultDenyIsEmpty(t *testing.T) {
	api := testAPI(t)
	got := api.service.Reports()
	if got.Reports == nil || len(got.Reports) != 0 {
		t.Fatalf("reports=%+v", got)
	}
}
