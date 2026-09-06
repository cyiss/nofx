import {mkdir, readFile, writeFile} from 'node:fs/promises';
import {createHash} from 'node:crypto';
const url='https://github.com/skeeto/w64devkit/releases/download/v2.9.1/w64devkit-x64-2.9.1.7z.exe';
const size=61462208, chunk=4*1024*1024;
const dir='.cache/toolchain/compiler-parts';
await mkdir(dir,{recursive:true});
await Promise.all(Array.from({length:Math.ceil(size/chunk)},async(_,i)=>{
 const start=i*chunk,end=Math.min(size-1,start+chunk-1),path=`${dir}/${i}`;
 try {if((await readFile(path)).length===end-start+1)return;}catch{}
 for(let attempt=0;attempt<3;attempt++) {
  try {
   const r=await fetch(url,{headers:{Range:`bytes=${start}-${end}`},signal:AbortSignal.timeout(180000)});
   if(r.status!==206)throw Error(`Expected206 got${r.status}`);
   const bytes=Buffer.from(await r.arrayBuffer());
   if(bytes.length!==end-start+1)throw Error('Wrong chunk length');
   await writeFile(path,bytes);console.log(`Verified length part ${i+1}/${Math.ceil(size/chunk)}`);return;
  }catch(err){if(attempt===2)throw err;}
 }
}));
const bytes=Buffer.concat(await Promise.all(Array.from({length:Math.ceil(size/chunk)},(_,i)=>readFile(`${dir}/${i}`))));
if(createHash('sha256').update(bytes).digest('hex')!=='9208c19755cd4964b7915b9afcf02c66d493a4c870c4b3e83f6c538d9c1237a5')throw Error('Compiler SHA256 mismatch');
await writeFile('.cache/toolchain/w64devkit.7z.exe',bytes);
console.log('Full compiler verified SHA256 against official GitHub release metadata.');
