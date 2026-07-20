import assert from "node:assert/strict";
import worker from "./worker.js";
const writes=[];const env={UPLOAD_TOKEN:"secret",ARTIFACTS:{put:async(...x)=>writes.push(x)}};
let r=await worker.fetch(new Request("https://example.test/?key=heartbeat/latest.json&token=secret",{method:"PUT",body:"{}",headers:{"content-length":"2","x-amz-checksum-sha256":"abc"}}),env);assert.equal(r.status,204);assert.equal(writes.length,1);assert.equal(writes[0][0],"heartbeat/latest.json");
r=await worker.fetch(new Request("https://example.test/?key=other.json&token=secret",{method:"PUT",body:"{}",headers:{"content-length":"2"}}),env);assert.equal(r.status,403);
r=await worker.fetch(new Request("https://example.test/?key=heartbeat/latest.json&token=wrong",{method:"PUT",body:"{}",headers:{"content-length":"2"}}),env);assert.equal(r.status,401);
console.log("r2_gateway_tests=PASS");
