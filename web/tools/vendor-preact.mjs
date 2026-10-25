// Копирует исходники Preact (ES-модули) в web/static/vendor/preact и
// дописывает расширения .js к относительным импортам — тогда браузер грузит
// их напрямую, без сборщика и без import map (CSP остаётся script-src 'self').
// Запуск: node web/tools/vendor-preact.mjs <путь к клону preactjs/preact>
import fs from "node:fs";
import path from "node:path";

const src = process.argv[2];
if (!src) throw new Error("usage: vendor-preact.mjs <preact checkout>");
const out = path.join(path.dirname(new URL(import.meta.url).pathname), "../static/vendor/preact");
fs.rmSync(out, { recursive: true, force: true });

const fix = (code, file) =>
  code
    .replace(/(from\s+|import\s*\(\s*|import\s+)(['"])(\.{1,2}\/[^'"]+?)\2/g, (m, pre, q, spec) =>
      spec.endsWith(".js") || spec.endsWith(".d.ts") ? m : `${pre}${q}${spec}${fs.existsSync(path.join(path.dirname(file), spec)) && fs.statSync(path.join(path.dirname(file), spec)).isDirectory() ? "/index" : ""}.js${q}`)
    .replace(/(['"])preact\1/g, "$1./index.js$1");

function copy(dir, dest, filter) {
  fs.mkdirSync(dest, { recursive: true });
  for (const name of fs.readdirSync(dir)) {
    const p = path.join(dir, name);
    if (fs.statSync(p).isDirectory()) { copy(p, path.join(dest, name), filter); continue; }
    if (!filter(name)) continue;
    fs.writeFileSync(path.join(dest, name), fix(fs.readFileSync(p, "utf8"), p));
  }
}
copy(path.join(src, "src"), out, (n) => (n.endsWith(".js") && n !== "cjs.js") || (n.endsWith(".d.ts") && n !== "index-5.d.ts"));
// хуки лежат рядом: ./hooks.js импортирует ./index.js
for (const [from, to] of [["hooks/src/index.js", "hooks.js"], ["hooks/src/index.d.ts", "hooks.d.ts"]]) {
  fs.writeFileSync(path.join(out, to), fix(fs.readFileSync(path.join(src, from), "utf8"), path.join(src, "hooks/src", "x")));
}
fs.copyFileSync(path.join(src, "LICENSE"), path.join(out, "LICENSE"));
const pkg = JSON.parse(fs.readFileSync(path.join(src, "package.json"), "utf8"));
fs.writeFileSync(path.join(out, "VERSION"), `preact ${pkg.version}\n`);
console.log("vendored preact", pkg.version, "→", out);
