import {readFile,writeFile} from 'node:fs/promises';
const mod=await readFile('go.mod','utf8');
const queries=[...mod.matchAll(/^\s+([^\s]+)\s+(v[^\s]+)(?:\s+\/\/ indirect)?$/gm)].map(m=>({package:{ecosystem:'Go',name:m[1]},version:m[2]}));
queries.push({package:{ecosystem:'Go',name:'stdlib'},version:mod.match(/^go (\S+)/m)[1]});
const lock=JSON.parse(await readFile('web/package-lock.json','utf8')); const packages={};
for(const [path,p] of Object.entries(lock.packages)){if(!path||!p.version)continue;const name=p.name||path.split('node_modules/').at(-1); (packages[name]??=[]).push(p.version);}
const request=async(url,body)=>{const r=await fetch(url,{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify(body),signal:AbortSignal.timeout(60000)});if(!r.ok)throw Error(`${url}: ${r.status}`);return r.json();};
const [go,npm]=await Promise.all([request('https://api.osv.dev/v1/querybatch',{queries}),request('https://registry.npmjs.org/-/npm/v1/security/advisories/bulk',packages)]);
const findings=go.results.flatMap((r,i)=>r.vulns?.length?[{...queries[i],vulnerabilities:r.vulns}]:[]);
const result={checkedAt:new Date().toISOString(),go:{count:queries.length,findings},npm,limitation:'Version matching, not reachable-symbol analysis'};
await writeFile('tasks/remediation-dependency-audit.json',JSON.stringify(result,null,2)+'\n');console.log(JSON.stringify(result,null,2));
