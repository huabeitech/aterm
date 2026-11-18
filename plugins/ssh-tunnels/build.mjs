import { buildNativePlugin } from '../../scripts/build-native-plugin.mjs'

await buildNativePlugin(import.meta.url, process.argv[2])
