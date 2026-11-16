import {buildPlugin} from '../../scripts/build-plugin.mjs'

await buildPlugin(import.meta.url, 'src/plugin.js', process.argv[2])
