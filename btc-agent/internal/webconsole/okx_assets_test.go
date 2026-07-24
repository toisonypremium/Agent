package webconsole

import (
	"testing"
	"time"

	"btc-agent/internal/okxassets"
)

type fakeOKXAssetSource struct {
	artifact okxassets.Artifact
	err      error
}

func (f fakeOKXAssetSource) Load() (okxassets.Artifact, error) { return f.artifact, f.err }
func TestOKXAssetsFailsClosedAndKeepsAssetsSeparateFromCapital(t *testing.T) {
	now := time.Date(2026, 7, 24, 3, 0, 0, 0, time.UTC)
	svc := testAPI(t).service
	got, err := svc.OKXAssets()
	if err != nil {
		t.Fatal(err)
	}
	if got.State != "khong_kha_dung" {
		t.Fatalf("got=%+v", got)
	}
	svc.SetOKXAssetSource(fakeOKXAssetSource{artifact: okxassets.Artifact{SchemaVersion: 1, Source: okxassets.SourceOKXSpotReadOnly, ObservedAt: now.Format(time.RFC3339), State: okxassets.StateVerified, Assets: []okxassets.Asset{{Currency: "BTC", Available: "1", Frozen: "0", Total: "1", ThesisLink: okxassets.ThesisUnlinked}}}})
	got, err = svc.OKXAssets()
	if err != nil {
		t.Fatal(err)
	}
	if got.State != "da_xac_minh" || len(got.Assets) != 1 || got.Assets[0].ThesisLink != okxassets.ThesisUnlinked {
		t.Fatalf("got=%+v", got)
	}
}
