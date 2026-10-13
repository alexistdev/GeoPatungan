// Fungsi murni untuk pemeriksaan kontrak frontend-backend (diuji di lib.test.mjs).
//
// "Bentuk sampel" berasal dari contract/samples.json (direkam backend dari respons asli):
//   "string" | "number" | "boolean" | "null" | "unknown"  skalar
//   "a|b"                                                  gabungan skalar ("absent" = field kadang tidak ada)
//   {"$optional": X, "$when": "null"|"absent"|"absent|null"}  objek/array yang kadang null dan/atau tidak ada
//   {k: bentuk}                                            objek;   [bentuk] array
// "Skema TS" berasal dari tipe yang dipakai kode frontend (lihat check.mjs):
//   {kind: 'string'|'number'|'boolean'|'null'|'any'|'array'|'object'|'map'|'union', nullable?, optional?, ...}

/** Menyeragamkan placeholder: /api/groups/{id}/x dan /api/groups/${gid}/x sama-sama /api/groups/{}/x. */
export function normalizeRoute(path) {
  return path
    .split('?')[0]
    .replace(/\$\{[^}]*\}/g, '{}')
    .replace(/\{[^}]*\}/g, '{}');
}

const isOptionalWrapper = (s) => s && typeof s === 'object' && !Array.isArray(s) && Object.keys(s).length === 2 && '$optional' in s && '$when' in s;

/** Memecah bentuk sampel menjadi: apakah null/absent mungkin, skalar apa saja, dan isi komposit. */
export function describeSample(sample) {
  if (isOptionalWrapper(sample)) {
    const inner = describeSample(sample.$optional);
    const when = String(sample.$when).split('|');
    return { ...inner, nullable: inner.nullable || when.includes('null'), absent: inner.absent || when.includes('absent') };
  }
  if (typeof sample === 'string') {
    const parts = sample.split('|');
    return {
      nullable: parts.includes('null'),
      absent: parts.includes('absent'),
      scalars: parts.filter((p) => p !== 'null' && p !== 'absent'),
      composite: null,
    };
  }
  return { nullable: false, absent: false, scalars: [], composite: sample };
}

/**
 * Memeriksa bahwa skema TS (apa yang diharapkan frontend) dapat menerima bentuk sampel (apa yang
 * sungguh dikirim backend). Mengembalikan daftar masalah (kosong = cocok).
 * Field tambahan di sampel yang tidak dibaca frontend diabaikan.
 */
export function compat(schema, sample, path = '$') {
  if (schema.kind === 'any') return [];
  const d = describeSample(sample);
  const errs = [];

  if (d.nullable && !schema.nullable && schema.kind !== 'null') {
    errs.push(`${path}: backend dapat mengirim null, tetapi tipe frontend tidak mengizinkan null`);
  }
  if (d.absent && !schema.optional) {
    errs.push(`${path}: backend kadang tidak mengirim field ini, tetapi tipe frontend mewajibkannya`);
  }

  for (const sc of d.scalars) {
    if (sc === 'unknown') continue;
    if (!acceptsScalar(schema, sc)) {
      errs.push(`${path}: backend mengirim ${sc}, frontend mengharapkan ${describeSchema(schema)}`);
    }
  }

  const c = d.composite;
  if (c === null) {
    if (d.scalars.length === 0 && !d.nullable && !d.absent) errs.push(`${path}: bentuk sampel tidak dikenali`);
    return errs;
  }
  if (Array.isArray(c)) {
    if (!schemaHas(schema, 'array')) {
      errs.push(`${path}: backend mengirim array, frontend mengharapkan ${describeSchema(schema)}`);
      return errs;
    }
    const elem = c[0];
    if (elem !== 'unknown') errs.push(...compat(pickOption(schema, 'array').elem, elem, `${path}[]`));
    return errs;
  }
  // objek
  if (schemaHas(schema, 'map')) {
    const m = pickOption(schema, 'map');
    for (const [k, v] of Object.entries(c)) errs.push(...compat(m.value, v, `${path}.${k}`));
    return errs;
  }
  if (!schemaHas(schema, 'object')) {
    errs.push(`${path}: backend mengirim objek, frontend mengharapkan ${describeSchema(schema)}`);
    return errs;
  }
  const o = pickOption(schema, 'object');
  for (const [name, prop] of Object.entries(o.props)) {
    if (!(name in c)) {
      if (!prop.optional) errs.push(`${path}.${name}: field ada di tipe frontend tetapi tidak dikirim backend`);
      continue;
    }
    errs.push(...compat(prop, c[name], `${path}.${name}`));
  }
  return errs;
}

function options(schema) {
  return schema.kind === 'union' ? schema.options : [schema];
}
function schemaHas(schema, kind) {
  return options(schema).some((o) => o.kind === kind || o.kind === 'any');
}
function pickOption(schema, kind) {
  return options(schema).find((o) => o.kind === kind) ?? options(schema)[0];
}
function acceptsScalar(schema, sc) {
  return options(schema).some((o) => o.kind === 'any' || o.kind === sc);
}

export function describeSchema(schema) {
  if (schema.kind === 'union') return schema.options.map(describeSchema).join(' | ');
  if (schema.kind === 'array') return `${describeSchema(schema.elem)}[]`;
  if (schema.kind === 'object') return 'objek';
  if (schema.kind === 'map') return `map<${describeSchema(schema.value)}>`;
  return schema.kind;
}

/** Field yang dikirim frontend dalam body harus dikenal backend (server menolak field asing). */
export function checkRequestBody(schema, sample, path = 'body') {
  const o = options(schema).find((x) => x.kind === 'object');
  if (!o) return [];
  const d = describeSample(sample);
  const known = d.composite && !Array.isArray(d.composite) ? d.composite : {};
  const errs = [];
  for (const [name, prop] of Object.entries(o.props)) {
    if (!(name in known)) {
      errs.push(`${path}.${name}: frontend dapat mengirim field ini, tetapi alur kontrak backend tidak pernah mengirimnya (server menolak field asing, atau alur belum mencakupnya)`);
      continue;
    }
    errs.push(...compat(prop, known[name], `${path}.${name}`).filter((e) => !/tidak mengirim field ini/.test(e)));
  }
  return errs;
}
