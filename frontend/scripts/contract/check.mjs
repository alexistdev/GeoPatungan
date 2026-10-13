// Memeriksa kontrak frontend-backend. Jalankan: npm run contract
//
// Untuk setiap panggilan this.http.<metode>(url, body?) di src/app/core:
//  1. rute (metode + URL) harus ada di contract/routes.json (daftar rute router asli backend);
//  2. tipe respons yang dideklarasikan (generik) harus cocok dengan bentuk respons nyata yang direkam
//     backend di contract/samples.json (field, tipe, null/opsional);
//  3. field pada body yang dikirim harus dikenal backend (ada di permintaan yang direkam).
// Sumber kontrak diperbarui dari backend: UPDATE_CONTRACT=1 go test ./internal/app/
import fs from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
import ts from 'typescript';

import { checkRequestBody, compat, normalizeRoute } from './lib.mjs';

const here = path.dirname(fileURLToPath(import.meta.url));
const root = path.resolve(here, '../..'); // frontend/
const contractDir = path.resolve(root, '../contract');
const coreDir = path.join(root, 'src/app/core');

const routes = JSON.parse(fs.readFileSync(path.join(contractDir, 'routes.json'), 'utf8'));
const samples = JSON.parse(fs.readFileSync(path.join(contractDir, 'samples.json'), 'utf8'));

const routeSet = new Set(routes.map((r) => { const [m, ...p] = r.split(' '); return `${m} ${normalizeRoute(p.join(' '))}`; }));

// indeks sampel menurut "METODE rute-ternormalisasi"
const respIndex = new Map(); // -> [{status, shape}]
for (const [key, shape] of Object.entries(samples.responses)) {
  const i = key.lastIndexOf(' ');
  const route = key.slice(0, i);
  const status = Number(key.slice(i + 1));
  const [m, ...p] = route.split(' ');
  const k = `${m} ${normalizeRoute(p.join(' '))}`;
  if (!respIndex.has(k)) respIndex.set(k, []);
  respIndex.get(k).push({ status, shape });
}
const reqIndex = new Map();
for (const [key, shape] of Object.entries(samples.requests)) {
  const [m, ...p] = key.split(' ');
  reqIndex.set(`${m} ${normalizeRoute(p.join(' '))}`, shape);
}

// ---- program TypeScript (hanya src/app/core, tanpa spec)
const files = fs.readdirSync(coreDir).filter((f) => f.endsWith('.ts') && !f.endsWith('.spec.ts')).map((f) => path.join(coreDir, f));
const cfgPath = path.join(root, 'tsconfig.app.json');
const cfg = ts.parseJsonConfigFileContent(ts.readConfigFile(cfgPath, ts.sys.readFile).config, ts.sys, root);
const program = ts.createProgram(files, { ...cfg.options, noEmit: true, skipLibCheck: true });
const checker = program.getTypeChecker();

// ---- tipe TS -> skema
export function toSchema(type, depth = 0) {
  const F = ts.TypeFlags;
  if (depth > 8) return { kind: 'any' };
  if (type.flags & (F.Any | F.Unknown)) return { kind: 'any' };
  if (type.flags & F.Null) return { kind: 'null' };
  if (type.flags & F.Undefined) return { kind: 'null', optional: true };
  if (type.flags & F.StringLike) return { kind: 'string' };
  if (type.flags & F.NumberLike) return { kind: 'number' };
  if (type.flags & F.BooleanLike) return { kind: 'boolean' };

  if (type.isUnion()) {
    let nullable = false;
    let optional = false;
    const rest = [];
    for (const t of type.types) {
      if (t.flags & F.Null) nullable = true;
      else if (t.flags & F.Undefined) optional = true;
      else rest.push(t);
    }
    const parts = [];
    for (const t of rest) {
      const s = toSchema(t, depth + 1);
      if (!parts.some((p) => JSON.stringify(p) === JSON.stringify(s))) parts.push(s);
    }
    const base = parts.length === 1 ? parts[0] : { kind: 'union', options: parts };
    return { ...base, nullable: nullable || base.nullable, optional: optional || base.optional };
  }

  if (checker.isArrayType(type)) {
    return { kind: 'array', elem: toSchema(checker.getTypeArguments(type)[0], depth + 1) };
  }

  if (type.flags & F.Object || type.isIntersection()) {
    const idx = checker.getIndexInfoOfType(type, ts.IndexKind.String);
    const props = checker.getPropertiesOfType(type);
    if (idx && props.length === 0) return { kind: 'map', value: toSchema(idx.type, depth + 1) };
    const out = {};
    for (const p of props) {
      const decl = p.valueDeclaration ?? p.declarations?.[0];
      const pt = checker.getTypeOfSymbolAtLocation(p, decl);
      const s = toSchema(pt, depth + 1);
      out[p.name] = { ...s, optional: s.optional || !!(p.flags & ts.SymbolFlags.Optional) };
    }
    return { kind: 'object', props: out };
  }
  return { kind: 'any' };
}

// ---- URL statis dari AST
function classBase(node) {
  for (let n = node; n; n = n.parent) {
    if (ts.isClassDeclaration(n)) {
      for (const m of n.members) {
        if (ts.isPropertyDeclaration(m) && m.name.getText() === 'base' && m.initializer && ts.isStringLiteralLike(m.initializer)) {
          return m.initializer.text;
        }
      }
    }
  }
  return null;
}

function evalUrl(node) {
  if (ts.isStringLiteralLike(node)) return node.text;
  if (ts.isTemplateExpression(node)) {
    let out = node.head.text;
    for (const span of node.templateSpans) {
      const txt = span.expression.getText();
      if (txt === 'this.base') {
        const b = classBase(node);
        if (b === null) return null;
        out += b;
      } else {
        out += '{}';
      }
      out += span.literal.text;
    }
    return out;
  }
  return null;
}

// ---- kumpulkan panggilan
const METHODS = new Set(['get', 'post', 'put', 'delete']);
const calls = [];
const problems = [];

function visit(sf) {
  (function walk(node) {
    if (ts.isCallExpression(node) && ts.isPropertyAccessExpression(node.expression)) {
      const pa = node.expression;
      if (METHODS.has(pa.name.text) && pa.expression.getText() === 'this.http') {
        const { line } = sf.getLineAndCharacterOfPosition(node.getStart());
        const where = `${path.relative(root, sf.fileName)}:${line + 1}`;
        const url = node.arguments[0] ? evalUrl(node.arguments[0]) : null;
        if (url === null) {
          problems.push(`${where}: URL tidak statis (harus string/template literal agar kontrak bisa diperiksa)`);
        } else {
          calls.push({ where, method: pa.name.text.toUpperCase(), url, node });
        }
      }
    }
    ts.forEachChild(node, walk);
  })(sf);
}
for (const f of files) visit(program.getSourceFile(f));

for (const c of calls) {
  const route = `${c.method} ${normalizeRoute(c.url)}`;
  const label = `${c.where} ${c.method} ${c.url.split('?')[0]}`;

  // 1. rute
  if (!routeSet.has(route)) {
    problems.push(`${label}: rute TIDAK ADA di backend (contract/routes.json)`);
    continue;
  }

  // 2. respons
  const typeArg = c.node.typeArguments?.[0];
  if (typeArg) {
    const schema = toSchema(checker.getTypeFromTypeNode(typeArg));
    const ok = (respIndex.get(route) ?? []).filter((s) => s.status >= 200 && s.status < 300);
    if (ok.length === 0) {
      problems.push(`${label}: tidak ada sampel respons 2xx di contract/samples.json (tambahkan ke alur backend)`);
    }
    for (const s of ok) {
      for (const e of compat(schema, s.shape)) problems.push(`${label} respons ${s.status}: ${e}`);
    }
  }

  // 3. body permintaan
  const body = c.node.arguments[1];
  if ((c.method === 'POST' || c.method === 'PUT') && body && !(ts.isObjectLiteralExpression(body) && body.properties.length === 0)) {
    const schema = toSchema(checker.getTypeAtLocation(body));
    const sample = reqIndex.get(route);
    if (!sample) problems.push(`${label}: tidak ada sampel permintaan di contract/samples.json (tambahkan body ke alur backend)`);
    else for (const e of checkRequestBody(schema, sample)) problems.push(`${label} permintaan: ${e}`);
  }
}

// ---- galat: frontend membaca body.error pada semua respons non-2xx; 402 juga membaca required/balance
const ERROR_EXTRA = {
  'POST /api/contributions/{id}/pay 402': ['required', 'balance'],
  'POST /api/wallet/withdrawals 402': ['required', 'balance'],
};
for (const [key, shape] of Object.entries(samples.responses)) {
  const status = Number(key.slice(key.lastIndexOf(' ') + 1));
  if (status < 400) continue;
  const need = { error: 'string' };
  for (const f of ERROR_EXTRA[key] ?? []) need[f] = 'number';
  for (const [f, t] of Object.entries(need)) {
    if (!shape || typeof shape !== 'object' || shape[f] !== t) problems.push(`${key}: galat harus memuat ${f}:${t}, sampel: ${JSON.stringify(shape)}`);
  }
}

const MIN_CALLS = 30;
if (calls.length < MIN_CALLS) problems.push(`hanya ${calls.length} panggilan HTTP terdeteksi (minimal ${MIN_CALLS}); pemindai kemungkinan rusak`);

if (problems.length) {
  console.error(`KONTRAK TIDAK COCOK (${problems.length} masalah):\n` + problems.map((p) => `  - ${p}`).join('\n'));
  process.exit(1);
}
console.log(`Kontrak OK: ${calls.length} panggilan HTTP cocok dengan ${routes.length} rute dan ${Object.keys(samples.responses).length} sampel respons backend.`);
