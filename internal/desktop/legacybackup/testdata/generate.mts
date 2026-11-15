import {mkdirSync, mkdtempSync, writeFileSync, rmSync} from 'node:fs';
import {tmpdir} from 'node:os';
import {join} from 'node:path';
import {resolve, dirname} from 'node:path';
import {pathToFileURL, fileURLToPath} from 'node:url';
const reference=resolve(process.argv[2]);
const {LibraryStore}=await import(pathToFileURL(join(reference,'src/main/storage/libraryStore.ts')).href);
const {encryptBackup,decryptBackup}=await import(pathToFileURL(join(reference,'src/main/storage/backupCodec.ts')).href);
const {platformDefaults}=await import(pathToFileURL(join(reference,'src/shared/contracts/preferences.ts')).href);
const output=process.argv[3] || join(dirname(fileURLToPath(import.meta.url)),'electron-aes-v1.enc');
const directory=mkdtempSync(join(tmpdir(),'aterm-legacy-fixture-'));
try {
 const library=new LibraryStore(directory);
 library.unlockVault('legacy-fixture-password',true);
 const parent=library.createDirectory({name:'Production',parentID:''});
 const child=library.createDirectory({name:'Servers 中文',parentID:parent.id});
 const defaults={directoryID:child.id,user:'fixture',port:2222,identityFile:'/fixture/id_ed25519',passwordAuth:false,password:'',proxyJump:'',jumpHostID:'',forwardAgent:true,compression:true,serverAliveInterval:30,serverAliveCountMax:3,connectTimeout:12,remark:'Fixture only',favorite:true};
 const jump=library.createHost({...defaults,alias:'fixture-jump',hostName:'127.0.0.1'});
 const target=library.createHost({...defaults,alias:'fixture-target',hostName:'target.test',jumpHostID:jump.id,passwordAuth:true,password:'fixture-secret-中文'});
 library.reorderHosts('all','',[target.id,jump.id]);
 const preferences={...platformDefaults('darwin'),language:'en',appFontScale:110,terminalFontSize:18,terminalWordNavigation:'terminal'};
 if(process.argv[4]==='--embedded') {
  const {createCompatibleVault}=await import(pathToFileURL(join(reference,'src/main/storage/compatibleVault.ts')).href);
  const {exportWorkspace}=await import(pathToFileURL(join(reference,'src/main/storage/workspaceSnapshot.ts')).href);
  const created=await createCompatibleVault('legacy-fixture-password',{schemaVersion:1,snapshot:exportWorkspace(library.snapshot()).toString('base64'),passwords:library.passwordSnapshot(),sourceDevice:'Legacy fixture',hostCount:2,directoryCount:2,syncedAt:new Date().toISOString()});
  try {library.setCompatibleVault(created.encoded,created.key);}finally{created.key.fill(0);}
 }
 const snapshot=library.snapshot(),key=library.vaultKey();
 try {
  const bytes=encryptBackup({format:'aterm-library',version:1,library:snapshot,preferences},key,snapshot.vault!.salt);
  const check=decryptBackup(bytes,'legacy-fixture-password');check.key.fill(0);
  writeFileSync(output,bytes,{mode:0o600});
 }finally{key.fill(0);library.lock();}
}finally{rmSync(directory,{recursive:true,force:true});}
