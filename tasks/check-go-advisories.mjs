// Read only public dependency versions from go.mod; never execute repository code.
import { readFile, writeFile } from 'node:fs/promises';
const mod = await readFile(new URL('../go.mod', import.meta.url), 'utf8');
const packages = [...mod.matchAll(/^\s+([^\s]+)\s+(v[^\s]+)(?:\s+\/\/ indirect)?$/gm)]
  .map((m) => ({ package: { ecosystem: 'Go', name: m[1] }, version: m[2] }));
packages.push({package: {ecosystem: 'Go', name: 'stdlib'}, version: '1.25.11'});
const response = await fetch('https://api.osv.dev/v1/querybatch', {
  method: 'POST', headers: {'Content-Type': 'application/json'},
  body: JSON.stringify({queries: packages}), signal: AbortSignal.timeout(60000),
});
if (!response.ok) throw new Error(`OSV HTTP ${response.status}`);
const body = await response.json();
const findings = body.results.flatMap((result, i) => result.vulns?.length
  ? [{...packages[i], vulnerabilities: result.vulns}] : []);
const details = await Promise.all([...new Set(findings.flatMap(f => f.vulnerabilities.map(v => v.id)))].map(async id => {
  const r = await fetch(`https://api.osv.dev/v1/vulns/${id}`, {signal: AbortSignal.timeout(30000)});
  if (!r.ok) throw new Error(`OSV ${id} HTTP ${r.status}`);
  return r.json();
}));
const output = {checkedAt: new Date().toISOString(), source: 'https://api.osv.dev/v1/querybatch',
  count: packages.length, findings, details, limitation: 'Version matching only; not a reachable-symbol or binary scan.'};
await writeFile(new URL('go-advisories.json', import.meta.url), JSON.stringify(output, null, 2) + '\n');
console.log(JSON.stringify({count: output.count, findings, summaries: details.map(d => ({id: d.id, summary: d.summary, affected: d.affected}))}, null, 2));
