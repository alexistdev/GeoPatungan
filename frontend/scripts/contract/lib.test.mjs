import assert from 'node:assert/strict';
import { describe, it } from 'node:test';

import { checkRequestBody, compat, describeSample, normalizeRoute } from './lib.mjs';

const S = (kind, extra = {}) => ({ kind, ...extra });
const str = S('string');
const num = S('number');
const bool = S('boolean');
const obj = (props) => S('object', { props });
const arr = (elem) => S('array', { elem });

describe('normalizeRoute', () => {
  it('menyeragamkan placeholder dan membuang query', () => {
    assert.equal(normalizeRoute('/api/groups/{id}/members/{memberID}/approve'), '/api/groups/{}/members/{}/approve');
    assert.equal(normalizeRoute('/api/groups/${gid}/rounds/${rid}/contributions'), '/api/groups/{}/rounds/{}/contributions');
    assert.equal(normalizeRoute('/api/admin/topups?status=all'), '/api/admin/topups');
    assert.equal(normalizeRoute('/api/me/referral/'), '/api/me/referral/');
  });
});

describe('describeSample', () => {
  it('skalar, gabungan, dan pembungkus', () => {
    assert.deepEqual(describeSample('string'), { nullable: false, absent: false, scalars: ['string'], composite: null });
    const d = describeSample('absent|null|number');
    assert.equal(d.nullable, true);
    assert.equal(d.absent, true);
    assert.deepEqual(d.scalars, ['number']);
    const w = describeSample({ $optional: { a: 'string' }, $when: 'null' });
    assert.equal(w.nullable, true);
    assert.equal(w.absent, false);
    assert.deepEqual(w.composite, { a: 'string' });
  });
});

describe('compat: skalar', () => {
  it('tipe sama cocok, beda tidak', () => {
    assert.deepEqual(compat(str, 'string'), []);
    assert.match(compat(num, 'string')[0], /mengirim string.*number/);
    assert.match(compat(str, { a: 'string' })[0], /objek/);
  });
  it('null hanya boleh bila tipe frontend nullable', () => {
    assert.match(compat(str, 'null|string')[0], /null/);
    assert.deepEqual(compat({ ...str, nullable: true }, 'null|string'), []);
  });
  it('absent hanya boleh bila field opsional', () => {
    assert.match(compat(str, 'absent|string')[0], /tidak mengirim/);
    assert.deepEqual(compat({ ...str, optional: true }, 'absent|string'), []);
  });
  it('union literal string diterima sebagai string; any menerima apa saja', () => {
    assert.deepEqual(compat(S('any'), { x: 'number' }), []);
    assert.deepEqual(compat(S('union', { options: [str, num] }), 'number'), []);
  });
});

describe('compat: objek', () => {
  const user = obj({ id: str, age: num, nick: { ...str, optional: true } });
  it('cocok; field tambahan di backend diabaikan', () => {
    assert.deepEqual(compat(user, { id: 'string', age: 'number', extra: 'boolean' }), []);
  });
  it('field wajib tidak dikirim backend', () => {
    const e = compat(user, { id: 'string' });
    assert.equal(e.length, 1);
    assert.match(e[0], /\$\.age.*tidak dikirim/);
  });
  it('field opsional boleh tidak ada; tipe salah dilaporkan dengan path', () => {
    assert.deepEqual(compat(user, { id: 'string', age: 'number' }), []);
    assert.match(compat(user, { id: 'number', age: 'number' })[0], /\$\.id/);
  });
  it('objek bersarang dan jalurnya', () => {
    const t = obj({ wallet: obj({ balance: num }) });
    assert.deepEqual(compat(t, { wallet: { balance: 'number' } }), []);
    assert.match(compat(t, { wallet: { balance: 'string' } })[0], /\$\.wallet\.balance/);
  });
  it('bentuk bersarang yang kadang null (pembungkus) cocok dengan tipe nullable saja', () => {
    const sample = { $optional: { id: 'string' }, $when: 'null' };
    assert.deepEqual(compat({ ...obj({ id: str }), nullable: true }, sample), []);
    assert.match(compat(obj({ id: str }), sample)[0], /null/);
    // bila "absent" bukan "null": tipe nullable saja tidak cukup
    const absent = { $optional: { id: 'string' }, $when: 'absent' };
    assert.match(compat({ ...obj({ id: str }), nullable: true }, absent)[0], /tidak mengirim/);
  });
  it('isi pembungkus tetap diperiksa', () => {
    const sample = { $optional: { id: 'number' }, $when: 'null' };
    assert.match(compat({ ...obj({ id: str }), nullable: true }, sample)[0], /\$\.id/);
  });
});

describe('compat: array dan map', () => {
  it('elemen array diperiksa; array kosong (unknown) selalu cocok', () => {
    assert.deepEqual(compat(arr(obj({ a: num })), [{ a: 'number' }]), []);
    assert.match(compat(arr(obj({ a: num })), [{ a: 'string' }])[0], /\$\[\]\.a/);
    assert.deepEqual(compat(arr(obj({ a: num })), ['unknown']), []);
  });
  it('array vs objek tertukar', () => {
    assert.match(compat(arr(num), { a: 'number' })[0], /objek/);
    assert.match(compat(obj({}), ['number'])[0], /array/);
  });
  it('Record<string, number> memeriksa semua nilainya', () => {
    const m = S('map', { value: num });
    assert.deepEqual(compat(m, { 1: 'number', 2: 'number' }), []);
    assert.match(compat(m, { 1: 'number', 2: 'string' })[0], /\$\.2/);
  });
});

describe('checkRequestBody', () => {
  const body = obj({ amount: num, note: { ...str, optional: true } });
  it('semua field yang dapat dikirim frontend harus dikenal backend', () => {
    assert.deepEqual(checkRequestBody(body, { amount: 'number', note: 'string' }), []);
    const e = checkRequestBody(body, { amount: 'number' });
    assert.equal(e.length, 1);
    assert.match(e[0], /body\.note/);
  });
  it('field opsional di sisi backend ("absent") diterima untuk field opsional frontend', () => {
    assert.deepEqual(checkRequestBody(body, { amount: 'number', note: 'absent|string' }), []);
  });
  it('tipe field yang salah dilaporkan', () => {
    assert.match(checkRequestBody(body, { amount: 'string', note: 'string' })[0], /body\.amount/);
  });
  it('tanpa skema objek: tidak ada pemeriksaan', () => {
    assert.deepEqual(checkRequestBody(S('any'), { a: 'number' }), []);
  });
});
