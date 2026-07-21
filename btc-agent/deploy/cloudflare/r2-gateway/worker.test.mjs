import assert from "node:assert/strict";
import worker from "./worker.js";

const writes = [];
const env = { UPLOAD_TOKEN: "secret", ARTIFACTS: { put: async (...args) => writes.push(args) } };
const put = (key, token = "secret", body = "{}") => worker.fetch(new Request(`https://example.test/?key=${encodeURIComponent(key)}&token=${token}`, { method: "PUT", body, headers: { "content-length": String(body.length), "x-amz-checksum-sha256": "abc" } }), env);

let response = await put("heartbeat/latest.json");
assert.equal(response.status, 204);
response = await put("llm-usage/2026/07/21/events/llm-abcd.json");
assert.equal(response.status, 204);
response = await put("llm-usage/2026/07/21/summary-abc123.json");
assert.equal(response.status, 204);
assert.deepEqual(writes.map(x => x[0]), ["heartbeat/latest.json", "llm-usage/2026/07/21/events/llm-abcd.json", "llm-usage/2026/07/21/summary-abc123.json"]);

for (const key of ["other.json", "llm-usage/2026/07/21/../../secret.json", "llm-usage/2026/07/21/events/x/y.json", "llm-usage/2026/7/21/events/x.json", "llm-usage/2026/07/21/events/.json", "llm-usage//2026/07/21/events/x.json"]) {
  response = await put(key);
  assert.equal(response.status, 403, key);
}
response = await put("heartbeat/latest.json", "wrong");
assert.equal(response.status, 401);
response = await worker.fetch(new Request("https://example.test/", { method: "GET" }), env);
assert.equal(response.status, 405);
response = await put("heartbeat/latest.json", "secret", "x".repeat(64 * 1024 + 1));
assert.equal(response.status, 413);
console.log("r2_gateway_tests=PASS");
