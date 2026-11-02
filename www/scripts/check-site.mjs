// Check emitted pages, not just the locale configuration: catch missing routes,
// broken project subpaths and languages drifting away from the README menu.
import assert from 'node:assert/strict';
import fs from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
const root = fileURLToPath(new URL('../../', import.meta.url));
const readme = fs.readFileSync(path.join(root,'README.md'),'utf8');
const menu = readme.split('\n').find(line => line.includes('(README.md)') && line.includes('(docs/readme/README.'));
assert.ok(menu, 'README language navigation is missing');
const codes = [...menu.matchAll(/\]\((?:docs\/readme\/)?README(?:\.([\w-]+))?\.md\)/g)].map(m => m[1] || 'zh');
// README.md has no dot before the base name's extension in the optional group.
assert.equal(codes[0], 'zh');
assert.ok(!codes.includes('en'));
const dist = path.join(root,'www/dist');
const badgeSources = source => [...source.matchAll(/<img\b[^>]*src="(https:\/\/img\.shields\.io\/[^"]+)"/g)].map(match => match[1]);
const imageTag = (source, name) => [...source.matchAll(/<img\b[^>]*>/g)].map(match => match[0]).find(tag => tag.includes(`assets/${name}"`)) || '';
for (const code of codes) {
  const html = fs.readFileSync(path.join(dist,code === 'zh' ? '' : code,'index.html'),'utf8');
  const options = [...html.matchAll(/<option\b[^>]*value="([^"]+)"/g)].map(m=>m[1]);
  assert.equal(options.length,codes.length, `${code}: language dropdown missing options`);
  assert.ok(!options.some(url=>url.endsWith('/en/')), `${code}: unexpected English route`);
  assert.ok(html.includes('id="site-language"'), `${code}: missing language selector`);
  assert.ok(html.includes(`dir="${['ar','fa','ps','prs'].includes(code)?'rtl':'ltr'}"`), `${code}: incorrect reading direction`);
  const readmePath = code === 'zh' ? 'README.md' : `docs/readme/README.${code}.md`;
  const source = fs.readFileSync(path.join(root,readmePath),'utf8');
  assert.ok(/^<p align="center"><strong>.+<\/strong><\/p>$/m.test(source), `${code}: README slogan is not explicitly centered`);
  assert.deepEqual(badgeSources(source), badgeSources(readme), `${code}: README badges differ from the main README`);
  const table = source.match(/<table\b[^>]*>[\s\S]*?<\/table>/)?.[0] || '';
  for (const name of ['network-checks.png','community-browser-check.png','community-qq.png']) {
    const tag = imageTag(source,name);
    assert.ok(tag, `${code}: missing README image ${name}`);
    const width = imageTag(readme,name).match(/\bwidth="(\d+)"/)?.[1];
    assert.ok(width, `Main README image ${name} has no explicit width`);
    assert.equal(tag.match(/\bwidth="(\d+)"/)?.[1], width, `${code}: README image width differs for ${name}`);
    if (name !== 'community-qq.png') assert.ok(table.includes(tag), `${code}: screenshot ${name} is outside the README table`);
  }
  assert.ok(html.includes(`href="https://github.com/Lee-xeo/Claude-Cage/blob/main/${readmePath}"`), `${code}: incorrect README link`);
  for (const match of html.matchAll(/href="https:\/\/github\.com\/Lee-xeo\/Claude-Cage\/blob\/main\/([^"#]+)(?:#[^"]*)?"/g)) {
    const relative = decodeURIComponent(match[1]);
    assert.ok(!relative.split('/').includes('..'), `${code}: unresolved document path ${relative}`);
    assert.ok(fs.existsSync(path.join(root,relative)), `${code}: broken repository document link ${relative}`);
  }
  for (const match of html.matchAll(/(?:src|href)="(\/Claude-Cage\/[^"#]+)"/g)) {
    const relative = decodeURIComponent(match[1].slice('/Claude-Cage/'.length));
    const target = path.join(dist,relative.endsWith('/') ? relative+'index.html' : relative);
    assert.ok(fs.existsSync(target), `${code}: broken local asset or link ${match[1]}`);
  }
  assert.ok(!html.includes('/Users/'), `${code}: local developer path leaked`);
}
assert.ok(!fs.existsSync(path.join(dist,'en/index.html')), 'Old English page still present');
console.log(`Verified ${codes.length} README languages, dropdowns, directions and local assets.`);
