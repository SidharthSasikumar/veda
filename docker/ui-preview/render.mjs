// Trusted preview harness. Repository code is served as static files only.
import http from 'node:http';
import path from 'node:path';
import {readFile,stat,realpath} from 'node:fs/promises';
import {spawn} from 'node:child_process';
const entry=process.argv[2];
if(!entry||entry.startsWith('/')||entry.split('/').includes('..')||!entry.endsWith('.html'))throw new Error('Invalid captured HTML entry');
const root='/source',missing=new Set();
const types={'.html':'text/html; charset=utf-8','.css':'text/css; charset=utf-8','.js':'text/javascript; charset=utf-8','.mjs':'text/javascript; charset=utf-8','.json':'application/json','.svg':'image/svg+xml','.png':'image/png'};
const server=http.createServer(async(req,res)=>{
 try{
  if(req.method!=='GET'){res.writeHead(405);res.end();return;}
  const url=new URL(req.url,'http://127.0.0.1');
  const pathname=decodeURIComponent(url.pathname);
  let filename=path.resolve(root,'.'+pathname);
  if(filename!==root&&!filename.startsWith(root+'/'))throw new Error('outside capture');
  if((await stat(filename)).isDirectory())filename=path.join(filename,'index.html');
  const resolved=await realpath(filename);if(!resolved.startsWith(root+'/'))throw new Error('outside capture');
  let content=await readFile(resolved);
  const source=content.toString();
  // Only inspect resources actually served to this page, not unrelated repo files.
  const externalAsset=/<(?:script|img|link|iframe|source|video|audio)\b[^>]*(?:src|href)\s*=\s*["'](?:https?:)?\/\//i.test(source)||/url\(\s*["']?(?:https?:)?\/\//i.test(source)||/@import\s+["'](?:https?:)?\/\//i.test(source);
  if(externalAsset)missing.add('This page references external assets that are blocked in isolated previews.');
  res.setHeader('Content-Type',types[path.extname(filename)]||'text/plain');
  res.setHeader('Content-Security-Policy',"default-src 'self' data:; script-src 'self' 'unsafe-inline'; style-src 'self' 'unsafe-inline'; connect-src 'self'; frame-src 'none'; object-src 'none'; base-uri 'self'; form-action 'none'");
  if(filename.endsWith('.html'))content=Buffer.from(content.toString().replace(/<\/head>/i,'<style>*,*::before,*::after{animation:none!important;transition:none!important;caret-color:transparent!important}</style></head>'));
  res.end(content);
 }catch{
  const missingPath=req.url.split('?')[0];if(missing.size<12&&!missingPath.includes('favicon'))missing.add('Unavailable preview resource: '+missingPath.slice(0,200));
  res.writeHead(404);res.end('Resource not captured');
 }
});
await new Promise(resolve=>server.listen(0,'127.0.0.1',resolve));
const address='http://127.0.0.1:'+server.address().port+'/'+entry.split('/').map(encodeURIComponent).join('/');
const args=['--headless','--no-sandbox','--disable-dev-shm-usage','--disable-gpu','--no-first-run','--disable-background-networking','--disable-extensions','--disable-sync','--hide-scrollbars','--force-device-scale-factor=1','--window-size=1280,900','--user-data-dir=/tmp/browser-profile','--virtual-time-budget=1800','--timeout=15000','--screenshot=/tmp/capture.png',address];
try{
 const browser=spawn('/usr/bin/chromium',args,{stdio:['ignore','ignore','pipe']});
 let errors='';browser.stderr.on('data',b=>{if(errors.length<4000)errors+=b.toString()});
 const timeout=setTimeout(()=>browser.kill('SIGKILL'),25000);
 try{await new Promise((resolve,reject)=>{browser.on('error',reject);browser.on('exit',code=>code===0?resolve():reject(new Error('Chromium capture failed: '+errors.slice(-1200))))})}finally{clearTimeout(timeout)}
 const png=await readFile('/tmp/capture.png');
 if(png.length>8*1024*1024)throw new Error('Screenshot exceeds capture limit');
 process.stdout.write(JSON.stringify({png:png.toString('base64'),missing:[...missing]}));
}finally{server.closeAllConnections();server.close()}
