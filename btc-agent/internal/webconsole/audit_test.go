package webconsole

import (
	"btc-agent/internal/storage"
	"testing"
)

func TestAuditMasksViewerIdentityAndExcludesPayload(t *testing.T) {
	api := testAPI(t)
	db := api.service.db
	if _, e := db.Exec(`INSERT INTO operator_audit_events(timestamp,identity,action,result,request_id,payload_json) VALUES(1,'person@example.test','HALT','APPLIED','r1','{"secret":"no"}')`); e != nil {
		t.Fatal(e)
	}
	out, e := api.service.Audit(5, RoleViewer)
	if e != nil || len(out.Events) != 1 {
		t.Fatalf("%+v %v", out, e)
	}
	if out.Events[0].Actor == "person@example.test" {
		t.Fatal("viewer identity leaked")
	}
	_ = storage.RuntimeEvent{}
}
