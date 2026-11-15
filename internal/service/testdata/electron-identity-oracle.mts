// Exercise the reference transport's real key-loading/auth handler. Only the
// SSH socket is replaced; no production keys, agent, server or UI are used.
import {readFileSync} from 'node:fs'
import {createRequire} from 'node:module'
import {join} from 'node:path'
import {pathToFileURL} from 'node:url'
const reference=process.argv[2]
const {SSHTransport}=await import(pathToFileURL(join(reference,'src/main/terminal/sshTransport.ts')).href)
const require=createRequire(join(reference,'package.json'))
const {Client}=require('ssh2')
process.env.SSH_AUTH_SOCK=''
let offered=false
Client.prototype.connect=function(config) {
  queueMicrotask(()=>config.authHandler(['publickey'],false,(method)=>{
    if (method && typeof method==='object' && method.type==='publickey') {
      offered=true
      this.emit('ready')
    } else this.emit('error',Error('Unexpected fixture authentication method'))
  }))
  return this
}
Client.prototype.end=function(){return this}
const results=[]
for (const vector of JSON.parse(readFileSync(process.argv[3],'utf8'))) {
  offered=false
  const prompts=[]
  const keys={readIdentity:()=>vector.managed?{bytes:Buffer.from(vector.material,'base64'),name:'Fixture Key'}:undefined}
  const transport=new SSHTransport({}, {}, async(prompt)=>{
    prompts.push({title:prompt.title,message:prompt.message,fields:prompt.fields})
    return [vector.passphrase]
  },vector.directory,keys)
  let error=''
  try {
    await transport.connectHost({id:'fixture',alias:'fixture',hostName:'fixture.invalid',port:22,user:'fixture',identityFile:vector.explicit?vector.path:'',passwordAuth:false})
  } catch(failure) {
    // An exhausted publickey-only fixture means key discovery skipped a file;
    // overall password/interactive authentication is outside this loader oracle.
    if(failure.code!=='sshAuthenticationFailedCheckTheUserPasswordPrivateKeyOrSshAgent') error=failure.message
  }
  transport.close()
  results.push({offered,error,prompts})
}
process.stdout.write(JSON.stringify(results))
