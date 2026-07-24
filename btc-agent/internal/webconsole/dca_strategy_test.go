package webconsole

import "testing"

func TestDCAStrategyIsReadOnlyAndThesisFirst(t *testing.T) {
	out := testAPI(t).service.DCAStrategy()
	if out.Stage != "HALTED_SHADOW" || out.ExecutionAuthority || len(out.Candidates) != 3 {
		t.Fatalf("out=%+v", out)
	}
	for _, candidate := range out.Candidates {
		if candidate.ThesisID != "" || candidate.AllocatedUSDT != "0" || candidate.Status != "danh_sach_dca" {
			t.Fatalf("candidate=%+v", candidate)
		}
	}
	if out.Layers[0].Percent != 25 || out.Layers[1].Percent != 35 || out.Layers[2].Percent != 40 {
		t.Fatalf("layers=%+v", out.Layers)
	}
}
