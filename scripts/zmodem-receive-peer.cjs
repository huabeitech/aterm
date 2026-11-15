// Test-only receiving peer using Electron's actual protocol implementation.
// Writes only into an explicitly provided disposable directory.
const fs = require('node:fs');
const path = require('node:path');
const os = require('node:os');
const {execFileSync} = require('node:child_process');
const args = new Map();
for (let i=2;i<process.argv.length;i+=2) args.set(process.argv[i],process.argv[i+1]);
const directory = fs.realpathSync(args.get('--directory') || '');
const roots = [os.tmpdir(), ...(process.platform === 'win32' ? [] : ['/tmp'])].filter(p=>fs.existsSync(p)).map(p=>fs.realpathSync(p));
if (!roots.some(root=>directory.startsWith(root+path.sep)) || !fs.statSync(directory).isDirectory()) throw Error('Choose a disposable temporary directory.');
const library = args.get('--library') || path.resolve(__dirname,'../frontend/node_modules/zmodem.js-ex');
const Zmodem = require(library);
let outbound = Promise.resolve(), finished = false, savedTTY;
const stty = fs.existsSync('/bin/stty') ? '/bin/stty' : '/usr/bin/stty';
const descriptors = new Set();
const write = bytes => {outbound = outbound.then(()=>new Promise((resolve,reject)=>process.stdout.write(Buffer.from(bytes),err=>err?reject(err):resolve()))); void outbound.catch(fail);};
const restore = () => {
 clearTimeout(timer); process.stdin.pause();
 if (process.stdin.isTTY) {process.stdin.setRawMode(false);if(savedTTY){try{execFileSync(stty,[savedTTY],{stdio:[0,'ignore','ignore']});}catch{}}}
 for (const fd of descriptors) {try{fs.closeSync(fd);}catch{}} descriptors.clear();
};
function fail(error) {if(finished)return;finished=true;restore();process.stderr.write(`ZMODEM receive fixture: ${error.message || error}\n`,()=>process.exit(1));}
const timer = setTimeout(()=>fail(Error('peer timed out')),120000);
const session = new Zmodem.Session.Receive();
session.set_sender(write);
session.on('offer',offer=>{
 const info=offer.get_details();
 if(args.get('--skip')===info.name) {void offer.skip().catch(fail);return;}
 if(info.name!==path.basename(info.name)||/[\x00-\x1f]/.test(info.name)) return fail(Error('invalid upload filename'));
 let fd;
 try {fd=fs.openSync(path.join(directory,info.name),'wx',0o600);descriptors.add(fd);} catch(error){return fail(error);}
 const requestedOffset = Number(args.get("--resume-offset")) || 0;
 const offset = Number.isInteger(requestedOffset) && requestedOffset >= 0 && requestedOffset <= info.size ? requestedOffset : 0;
 void offer.accept({offset, on_input:bytes=>{
  const data=Buffer.from(bytes);let offset=0;while(offset<data.length)offset+=fs.writeSync(fd,data,offset,data.length-offset);
 }}).then(()=>{fs.closeSync(fd);descriptors.delete(fd);}).catch(fail);
});
session.on('session_end',()=>{
 void (async()=>{await outbound;write(Buffer.from('ZMODEM upload fixture completed\r\n'));await outbound;finished=true;restore();process.exit(0);})().catch(fail);
});
process.stdin.on('data',bytes=>{try{session.consume(Array.from(bytes));}catch(error){fail(error);}});
process.stdin.on('error',fail);process.stdout.on('error',fail);
if(process.stdin.isTTY){savedTTY=execFileSync(stty,['-g'],{stdio:[0,'pipe','ignore']}).toString().trim();process.stdin.setRawMode(true);execFileSync(stty,['raw','-echo'],{stdio:[0,'ignore','ignore']});}
process.stdin.resume();
void session.start().catch(fail);
