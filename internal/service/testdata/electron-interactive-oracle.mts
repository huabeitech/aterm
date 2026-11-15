// Run the original keyboard-interactive handler; replace only socket startup.
import {readFileSync} from 'node:fs'
import {createRequire} from 'node:module'
import {join} from 'node:path'
import {pathToFileURL} from 'node:url'
const reference=process.argv[2]
const {SSHTransport}=await import(pathToFileURL(join(reference,'src/main/terminal/sshTransport.ts')).href)
const {Client}=createRequire(join(reference,'package.json'))('ssh2')
let vector, answers
Client.prototype.connect=function() {
  queueMicrotask(()=>this.emit('keyboard-interactive','Fixture',vector.instruction,'en',vector.questions.map((prompt,i)=>({prompt,echo:vector.echo[i]})),value=>{answers=value;this.emit('ready')}))
  return this
}
Client.prototype.end=function(){return this}
const results=[]
for (vector of JSON.parse(readFileSync(process.argv[3],'utf8'))) {
  const prompts=[]
  const transport=new SSHTransport({getHostPassword:()=> 'saved-password'}, {},async(prompt)=>{
    prompts.push({title:prompt.title,message:prompt.message,fields:prompt.fields})
    return vector.questions.map(()=> '123456')
  })
  await transport.connectHost({id:'fixture',alias:'Fixture',hostName:'fixture.invalid',port:22,user:'fixture',identityFile:'',passwordAuth:true})
  transport.close()
  results.push({answers,prompts})
}
process.stdout.write(JSON.stringify(results))
