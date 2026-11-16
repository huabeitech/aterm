import {execFileSync} from 'node:child_process'
import {copyFileSync, mkdirSync, mkdtempSync, readFileSync, rmSync, writeFileSync} from 'node:fs'
import {tmpdir} from 'node:os'
import {basename, dirname, join, resolve} from 'node:path'
import {fileURLToPath} from 'node:url'
import {build} from '../frontend/node_modules/esbuild/lib/main.js'

const repo = resolve(dirname(fileURLToPath(import.meta.url)), '..')

export async function buildPlugin(pluginURL, workerEntry, outputDirectory) {
  const root = dirname(fileURLToPath(pluginURL))
  const manifestPath = join(root, 'manifest.json')
  const manifest = JSON.parse(readFileSync(manifestPath, 'utf8'))
  const stage = mkdtempSync(join(tmpdir(), 'aterm-plugin-'))
  const output = resolve(outputDirectory ?? join(repo, 'build/bin/plugins'))

  try {
    const options = {
      absWorkingDir: root,
      bundle: true,
      minify: true,
      write: false,
      format: 'iife',
      jsx: 'automatic',
      nodePaths: [join(repo, 'frontend/node_modules')],
      define: {'process.env.NODE_ENV': '"production"'},
    }
    const worker = await build({...options, entryPoints: [workerEntry], target: 'es2020'})
    writeFileSync(join(stage, manifest.entry), worker.outputFiles[0].text)
    const view = await build({...options, entryPoints: ['src/ui/main.tsx'], outdir: 'output', target: ['safari16', 'chrome105']})
    const js = view.outputFiles.find(file => file.path.endsWith('.js')).text.replace(/<\/script/gi, '<\\/script')
    const css = view.outputFiles.find(file => file.path.endsWith('.css')).text.replace(/<\/style/gi, '<\\/style')
    const entry = join(stage, manifest.contributes.views[0].entry)
    mkdirSync(dirname(entry), {recursive: true})
    writeFileSync(entry, `<!doctype html>
  <html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1">
  <style>${css}</style></head><body><div id="root"></div><script>${js}</script></body></html>`)
    copyFileSync(manifestPath, join(stage, 'manifest.json'))
    for (const locale of Object.values(manifest.i18n.locales)) {
      const destination = join(stage, locale)
      mkdirSync(dirname(destination), {recursive: true})
      copyFileSync(join(root, locale), destination)
    }
    mkdirSync(output, {recursive: true})
    const archive = join(output, `${basename(root)}-${manifest.version}.aterm-plugin`)
    execFileSync('go', ['run', './scripts/pack-plugin', stage, archive], {cwd: repo, stdio: 'inherit'})
    console.log(archive)
  } finally {
    rmSync(stage, {recursive: true, force: true})
  }
}
