// Executed before any plugin markup in an opaque-origin, CSP-restricted frame.
function initializePluginView(config) {
  for (const key of ['RTCPeerConnection','webkitRTCPeerConnection','Worker','SharedWorker','webkit','chrome','external','_wails','wails','Wails']) {
    try { Object.defineProperty(globalThis,key,{value:undefined,configurable:false}); }
    catch { if (typeof globalThis[key] !== 'undefined' && !(config.nativeBridgeGuarded && ['webkit','chrome','external'].includes(key))) throw Error('Cannot isolate native global: '+key); }
  }
  const pending=new Map(),listeners=new Set();let sequence=0,current;
  function translate(bundle,locale) {
    const fallback=bundle?.defaultLocale??'en';let language=locale;
    try{language=Intl.getCanonicalLocales(locale.replaceAll('_','-'))[0]??fallback}catch{language=fallback}
    const languages=[...new Set([language,language.split('-')[0],fallback])];
    return (key,options={})=>{
      for(const candidate of languages){const messages=bundle?.messages[candidate];if(!messages)continue;const variants=[key];
        if(typeof options.count==='number'&&Number.isFinite(options.count)){const category=new Intl.PluralRules(candidate).select(options.count);variants.unshift(`${key}_${category}`,`${key}_other`)}
        for(const variant of variants){if(!Object.hasOwn(messages,variant))continue;return messages[variant].replace(/{{\s*([\w.-]+)\s*}}/g,(match,name)=>Object.hasOwn(options,name)?String(options[name]):match)}
      }return key;
    };
  }
  let readyResolve;const ready=new Promise(resolve=>readyResolve=resolve);
  function request(method,args){
    if(pending.size>=16)return Promise.reject(Error('Too many view requests'));
    const requestId=++sequence;
    return new Promise((resolve,reject)=>{
      const timer=setTimeout(()=>{pending.delete(requestId);reject(Error('View request timed out'))},50000);
      pending.set(requestId,{resolve,reject,timer});
      parent.postMessage({channel:'aterm-view',instanceId:config.instanceId,requestId,method,args},'*');
    });
  }
  Object.defineProperty(globalThis,'atermView',{value:Object.freeze({
    ready,get state(){return current},
    t(key,options){return translate(config.bundle,current?.environment.locale||'en')(key,options)},
    onDidChange(callback){listeners.add(callback);return()=>listeners.delete(callback)},
    executeCommand(command,args=null){return request('command',{command,args})},
    close(){return request('close',null)}
  })});
  addEventListener('message',({source,data})=>{
    if(source!==parent||data?.channel!=='aterm-view'||data.instanceId!==config.instanceId)return;
    if(data.type==='state'){
      current=data.state;
      for(const [key,value] of Object.entries(current.environment.tokens))document.documentElement.style.setProperty(key,value);
      document.documentElement.lang=current.environment.locale;document.documentElement.dataset.theme=current.environment.appearance;
      readyResolve();for(const listener of listeners){try{listener(current)}catch{}}
    }
    if(data.type==='response'){
      const request=pending.get(data.requestId);if(!request)return;pending.delete(data.requestId);clearTimeout(request.timer);
      data.error?request.reject(Error(data.error)):request.resolve(data.result);
    }
  });
  // Do not parse untrusted markup unless all hardening above succeeded. This
  // parser-blocking script runs before the document's remaining empty shell.
  document.write(config.html);
}
