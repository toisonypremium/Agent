package webconsole

import (
	"testing"
	"time"

	"btc-agent/internal/okxassets"
)

func TestOKXReconciliationNeverInfersThesisFromAssetSymbol(t *testing.T) {
	svc := testAPI(t).service
	now := time.Date(2026, 7, 24, 3, 0, 0, 0, time.UTC)
	svc.SetOKXAssetSource(fakeOKXAssetSource{artifact: okxassets.Artifact{SchemaVersion: 1, Source: okxassets.SourceOKXSpotReadOnly, ObservedAt: now.Format(time.RFC3339), State: okxassets.StateVerified, Assets: []okxassets.Asset{{Currency: "BTC", Available: "1", Frozen: "0", Total: "1", ThesisLink: okxassets.ThesisUnlinked}}}})
	out, err := svc.OKXReconciliation()
	if err != nil {
		t.Fatal(err)
	}
	if out.State != "can_ra_soat" || len(out.UnlinkedAssets) != 1 || out.UnlinkedAssets[0] != "BTC" {
		t.Fatalf("out=%+v", out)
	}
}
func TestOKXReconciliationFailsClosedWhenAssetArtifactUnavailable(t *testing.T) {
	out, err := testAPI(t).service.OKXReconciliation()
	if err != nil {
		t.Fatal(err)
	}
	if out.State != "khong_kha_dung" {
		t.Fatalf("out=%+v", out)
	}
}
