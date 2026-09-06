// Temporary loopback transport adapter for Go's public module proxy.
// Keeps TLS validation and Go checksum verification enabled.
import http from 'node:http';
import {Readable} from 'node:stream';
import {pipeline} from 'node:stream/promises';
const npm = process.argv[2] === 'npm';
const upstream = npm ? 'https://registry.npmjs.org' : 'https://proxy.golang.org';
const port = npm ? 18744 : 18743;
http.createServer(async (req,res) => {
  try {
    if (req.method !== 'GET' || !req.url.startsWith('/')) {res.writeHead(405);res.end();return;}
    const response = await fetch(upstream + req.url, {signal:AbortSignal.timeout(120000)});
    res.writeHead(response.status, {'content-type':response.headers.get('content-type') || 'application/octet-stream'});
    if (response.body) await pipeline(Readable.fromWeb(response.body),res); else res.end();
  } catch (err) { if (!res.headersSent) res.writeHead(502); res.end(); console.error(err.message); }
}).listen(port,'127.0.0.1',()=>console.log(`Dependency transport ready at http://127.0.0.1:${port} (${upstream})`));
