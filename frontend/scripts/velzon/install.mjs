// Memasang aset tema Velzon (demo "modern") ke public/velzon dari unduhan Velzon milik Anda.
//
//   npm run theme:install -- /jalur/ke/velzon          (folder yang berisi dist/ dan src/)
//
// Hanya set minimal yang disalin, dan icons.css dibangun ulang hanya dengan ikon Remix yang dipakai
// oleh src/app (hasil pemindaian kelas "ri-*"). Jalankan ulang setelah menambah ikon baru.
//
// CATATAN LISENSI: Velzon adalah tema komersial. Berkas hasilnya (public/velzon) sengaja TIDAK
// di-commit (lihat .gitignore); lihat docs/tema-velzon.md.
import fs from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const here = path.dirname(fileURLToPath(import.meta.url));
const root = path.resolve(here, '../..'); // frontend/
const out = path.join(root, 'public/velzon');

const FILES = [
  ['css/bootstrap.min.css', 'css/bootstrap.min.css'],
  ['css/app.min.css', 'css/app.min.css'],
  ['fonts/remixicon.woff2', 'fonts/remixicon.woff2'],
  ['fonts/materialdesignicons-webfont.woff2', 'fonts/materialdesignicons-webfont.woff2'],
  ['images/auth-one-bg.jpg', 'images/auth-one-bg.jpg'],
  ['libs/bootstrap/js/bootstrap.bundle.min.js', 'js/bootstrap.bundle.min.js'],
];

/** Kelas Remix yang dipakai aplikasi (dipindai dari html/ts di src/, tanpa spec). */
export function usedRemixIcons(srcDir) {
  const used = new Set();
  (function walk(dir) {
    for (const e of fs.readdirSync(dir, { withFileTypes: true })) {
      const p = path.join(dir, e.name);
      if (e.isDirectory()) walk(p);
      else if (/\.(html|ts)$/.test(e.name) && !e.name.endsWith('.spec.ts')) {
        for (const m of fs.readFileSync(p, 'utf8').matchAll(/\bri-[a-z0-9]+(?:-[a-z0-9]+)*\b/g)) used.add(m[0]);
      }
    }
  })(srcDir);
  return used;
}

/**
 * Membangun icons.css ringkas dari icons.css Velzon: font + kelas dasar Material Design Icons
 * (dipakai CSS tema untuk panah menu), serta Remix Icon lengkap dengan kelas ukuran tetapi hanya
 * ikon yang termasuk `used`. Font hanya woff2.
 */
export function buildIconsCss(source, used) {
  const lines = source.split('\n');
  const header = lines.slice(0, 8).join('\n');

  const mdiStart = lines.findIndex((l) => l.startsWith('.mdi:before'));
  const mdiEnd = lines.findIndex((l, i) => i > mdiStart && l === '}');
  const mdiBase = lines.slice(mdiStart, mdiEnd + 1).join('\n');

  const remixStart = lines.findIndex((l) => l.includes('Remix Icon v')) - 1; // baris "/*" pembuka
  const remix = lines.slice(remixStart).join('\n');

  // pecah menjadi blok "selektor { ... }" dan saring kelas ikon
  const kept = [];
  for (const block of remix.match(/(?:\/\*[\s\S]*?\*\/\s*)?[^{}]+\{[^}]*\}/g) ?? []) {
    const m = block.match(/(?:^|\n)\s*\.(ri-[a-z0-9-]+):before\s*\{/);
    if (m && block.trim().split('{')[0].trim() === `.${m[1]}:before`) {
      if (used.has(m[1])) kept.push(block.trim());
    } else {
      kept.push(block.trim());
    }
  }
  const remixCss = kept
    .join('\n')
    .replace(/src: url\("\.\.\/fonts\/remixicon\.eot[^;]*;[^\n]*\n\s*src: url\("\.\.\/fonts\/remixicon\.eot[^;]*;/, 'src: url("../fonts/remixicon.woff2?t=1718271040674") format("woff2");');

  const mdiFace = `/* Material Design Icons: hanya font + kelas dasar (dipakai CSS tema untuk panah menu/checkbox) */
@font-face {
  font-family: "Material Design Icons";
  src: url("../fonts/materialdesignicons-webfont.woff2?v=7.2.96") format("woff2");
  font-weight: normal;
  font-style: normal;
}`;
  return `${header}\n${mdiFace}\n${mdiBase}\n\n${remixCss}\n`;
}

function main() {
  const given = process.argv[2];
  if (!given) {
    console.error('Pemakaian: npm run theme:install -- /jalur/ke/velzon   (folder yang berisi dist/modern)');
    process.exit(2);
  }
  const assets = path.join(path.resolve(given), 'dist/modern/assets');
  if (!fs.existsSync(assets)) {
    console.error(`Tidak ditemukan ${assets}. Berikan folder Velzon yang berisi dist/modern/assets.`);
    process.exit(2);
  }

  fs.rmSync(out, { recursive: true, force: true });
  for (const [from, to] of FILES) {
    const dest = path.join(out, to);
    fs.mkdirSync(path.dirname(dest), { recursive: true });
    fs.copyFileSync(path.join(assets, from), dest);
  }

  const used = usedRemixIcons(path.join(root, 'src'));
  const css = buildIconsCss(fs.readFileSync(path.join(assets, 'css/icons.css'), 'utf8'), used);
  fs.writeFileSync(path.join(out, 'css/icons.css'), css);

  const present = new Set([...css.matchAll(/\.(ri-[a-z0-9-]+):before/g)].map((m) => m[1]));
  const missing = [...used].filter((u) => !present.has(u));
  console.log(`Velzon terpasang di public/velzon (${FILES.length + 1} berkas); ${present.size} ikon Remix dari ${used.size} kelas ri-* terpakai.`);
  if (missing.length) {
    console.warn(`Peringatan: kelas tidak ditemukan di Remix (mungkin bukan ikon): ${missing.join(', ')}`);
  }
}

if (process.argv[1] === fileURLToPath(import.meta.url)) main();
