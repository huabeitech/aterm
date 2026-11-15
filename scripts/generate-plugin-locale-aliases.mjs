// Generate the canonical-locale rejection table used by the public plugin format.
// Intl accepts well-formed unknown tags, so enumerating known languages alone
// would reject valid dictionaries. Store only aliases that change the spelling.
// Run from the repository root with the Node version used for Electron SDK tests.
import {writeFileSync} from 'node:fs'
const alphabet='abcdefghijklmnopqrstuvwxyz',languages=[],regions=[]
for(const a of alphabet) for(const b of alphabet){languages.push(a+b);regions.push((a+b).toUpperCase());for(const c of alphabet) languages.push(a+b+c)}
for(let region=0;region<1000;region++) regions.push(String(region).padStart(3,'0'))
function changed(values,prefix='') {return values.filter(value=>{try{return Intl.getCanonicalLocales(prefix+value)[0]!==prefix+value}catch{return true}}).sort()}
const data={
 languages:changed(languages),
 regions:changed(regions,'und-'),
 scripts:['Qaai'],
 // Deprecated variants from the Unicode locale identifier alias data; each
 // changes or removes the variant, sometimes adding a language/script/region.
 variants:['aaland','arevela','arevmda','baku1926','bokmal','colb1945','hakka','heploc','lojban','luna1918','nynorsk','petr1708','polytoni','saaho','xiang']
}
writeFileSync('internal/desktop/plugins/locale-aliases.json',JSON.stringify(data,null,2)+'\n')
