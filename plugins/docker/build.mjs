import {buildPlugin} from '../../scripts/build-plugin.mjs'

await buildPlugin(import.meta.url, 'src/plugin.ts', process.argv[2])
