const fs = require("fs");
const vm = require("vm");

const source = fs.readFileSync(process.argv[2], "utf8");
const match = source.match(/const html=.*?;\n/);
if (!match) throw new Error("Không tìm thấy hàm html escape");

const context = {};
vm.createContext(context);
vm.runInContext(`${match[0]}globalThis.escapeHTML = html;`, context);

const payload = `<img src=x onerror=alert(1)> & " '`;
const expected = `&lt;img src=x onerror=alert(1)&gt; &amp; &quot; &#39;`;
const actual = context.escapeHTML(payload);
if (actual !== expected) {
  throw new Error(`Escape sai: ${actual}`);
}
console.log("XSS_ESCAPE=OK", actual);
