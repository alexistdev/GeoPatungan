import assert from 'node:assert/strict';
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import { describe, it } from 'node:test';

import { buildIconsCss, usedRemixIcons } from './install.mjs';

const FIXTURE = `/*
Template Name: Velzon
Author: Themesbrand
Version: 4.3.0
Website: x
Contact: x
File: Icons Css File
*/
/* MaterialDesignIcons.com */
@font-face {
  font-family: "Material Design Icons";
  src: url("../fonts/materialdesignicons-webfont.eot?v=7");
}
.mdi:before,
.mdi-set {
  display: inline-block;
  font: normal normal normal 24px/1 "Material Design Icons";
}
.mdi-abacus::before {
  content: "\\F0001";
}
/*
* Remix Icon v4.3.0
*/
@font-face {
  font-family: "remixicon";
  src: url("../fonts/remixicon.eot?t=1"); /* IE9*/
  src: url("../fonts/remixicon.eot?t=1#iefix") format("embedded-opentype"), url("../fonts/remixicon.woff2?t=1") format("woff2");
  font-display: swap;
}
[class^=ri-], [class*=" ri-"] {
  font-family: "remixicon" !important;
}
.ri-lg {
  font-size: 1.3333em;
}
.ri-home-line:before {
  content: "\\ee2b";
}
.ri-wallet-3-line:before {
  content: "\\f2ac";
}
.ri-user-line:before {
  content: "\\f264";
}
`;

describe('buildIconsCss', () => {
  const css = buildIconsCss(FIXTURE, new Set(['ri-home-line', 'ri-user-line']));

  it('hanya menyertakan ikon yang dipakai', () => {
    assert.match(css, /\.ri-home-line:before/);
    assert.match(css, /\.ri-user-line:before/);
    assert.doesNotMatch(css, /ri-wallet-3-line/);
  });

  it('mempertahankan kelas dasar, ukuran, dan header tema', () => {
    assert.match(css, /\[class\^=ri-\]/);
    assert.match(css, /\.ri-lg \{/);
    assert.match(css, /Template Name: Velzon/);
  });

  it('MDI: hanya font dan kelas dasar, tanpa kelas ikon', () => {
    assert.match(css, /\.mdi:before/);
    assert.doesNotMatch(css, /mdi-abacus/);
    assert.match(css, /materialdesignicons-webfont\.woff2/);
    assert.doesNotMatch(css, /\.eot/);
  });

  it('font Remix hanya woff2', () => {
    assert.match(css, /remixicon\.woff2/);
    assert.doesNotMatch(css, /remixicon\.eot|embedded-opentype/);
  });

  it('tanpa ikon terpakai: tetap CSS yang valid dengan kelas dasar', () => {
    const empty = buildIconsCss(FIXTURE, new Set());
    assert.doesNotMatch(empty, /ri-home-line/);
    assert.match(empty, /font-family: "remixicon" !important/);
  });
});

describe('usedRemixIcons', () => {
  it('memindai html dan ts, mengabaikan spec', () => {
    const dir = fs.mkdtempSync(path.join(os.tmpdir(), 'icons-'));
    fs.mkdirSync(path.join(dir, 'a'));
    fs.writeFileSync(path.join(dir, 'a/x.html'), '<i class="ri-home-line"></i><i [class.ri-sun-line]="x"></i>');
    fs.writeFileSync(path.join(dir, 'a/y.ts'), "const icon = 'ri-user-line'; const notIcon = 'bri-x';");
    fs.writeFileSync(path.join(dir, 'a/z.spec.ts'), "expect('ri-bug-line')");
    fs.writeFileSync(path.join(dir, 'a/w.scss'), '.ri-scss-line{}');
    const used = usedRemixIcons(dir);
    assert.deepEqual([...used].sort(), ['ri-home-line', 'ri-sun-line', 'ri-user-line']);
    fs.rmSync(dir, { recursive: true });
  });

  it('pada proyek sebenarnya: setiap ikon terpakai ada di icons.css hasil pemasangan', () => {
    const root = path.resolve(import.meta.dirname, '../..');
    const cssPath = path.join(root, 'public/velzon/css/icons.css');
    if (!fs.existsSync(cssPath)) return; // tema belum dipasang (lihat docs/tema-velzon.md)
    const css = fs.readFileSync(cssPath, 'utf8');
    for (const name of usedRemixIcons(path.join(root, 'src'))) {
      assert.ok(css.includes(`.${name}:before`), `${name} tidak ada di icons.css; jalankan npm run theme:install`);
    }
  });
});
