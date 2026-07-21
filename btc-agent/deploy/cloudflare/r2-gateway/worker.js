const MAX_BYTES = 64 * 1024;
const EXACT_KEYS = new Set(["heartbeat/latest.json"]);
const LLM_EVENT_KEY = /^llm-usage\/\d{4}\/\d{2}\/\d{2}\/events\/[A-Za-z0-9._-]+\.json$/;
const LLM_SUMMARY_KEY = /^llm-usage\/\d{4}\/\d{2}\/\d{2}\/summary-[A-Za-z0-9._-]+\.json$/;

function allowedKey(key) {
  if (!key || key.includes("..") || key.includes("//")) return false;
  return EXACT_KEYS.has(key) || LLM_EVENT_KEY.test(key) || LLM_SUMMARY_KEY.test(key);
}

export default {
  async fetch(request, env) {
    if (request.method !== "PUT") return new Response("method not allowed", { status: 405 });
    const url = new URL(request.url);
    const token = url.searchParams.get("token") || "";
    const key = url.searchParams.get("key") || "";
    if (!env.UPLOAD_TOKEN || token !== env.UPLOAD_TOKEN) return new Response("unauthorized", { status: 401 });
    if (!allowedKey(key)) return new Response("key not allowed", { status: 403 });
    const length = Number(request.headers.get("content-length") || "0");
    if (!Number.isFinite(length) || length < 1 || length > MAX_BYTES) return new Response("payload too large", { status: 413 });
    const body = await request.arrayBuffer();
    if (body.byteLength < 1 || body.byteLength > MAX_BYTES) return new Response("payload too large", { status: 413 });
    await env.ARTIFACTS.put(key, body, {
      httpMetadata: { contentType: "application/json" },
      customMetadata: { sha256: request.headers.get("x-amz-checksum-sha256") || "" },
    });
    return new Response(null, { status: 204 });
  },
};
