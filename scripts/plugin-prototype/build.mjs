import {execFileSync} from 'node:child_process'
import {mkdtempSync, mkdirSync, rmSync, writeFileSync} from 'node:fs'
import {tmpdir} from 'node:os'
import {join, resolve} from 'node:path'
import {fileURLToPath} from 'node:url'

// Use the same esbuild installation as the app frontend.
const repo = fileURLToPath(new URL('../../', import.meta.url))
const esbuild = await import(new URL('../../frontend/node_modules/esbuild/lib/main.js', import.meta.url))
const live = process.argv.includes('--ssh')
const out = resolve(process.argv.slice(2).find(arg => arg !== '--ssh') ??
  (live ? 'build/bin/plugin-ssh' : 'build/bin/plugin-prototype'))
mkdirSync(out, {recursive: true})

const versions = [
  {version: '0.0.1', key: 'MemTotal', title: 'Memory overview', label: 'Total memory'},
  {version: '0.0.2', key: 'MemAvailable', title: 'Memory availability', label: 'Available memory'},
]

for (const {version, key, title, label} of versions) {
  const bundle = await esbuild.build({
    absWorkingDir: repo,
    entryPoints: ['scripts/plugin-prototype/detail.jsx'],
    nodePaths: [join(repo, 'frontend', 'node_modules')],
    bundle: true,
    minify: true,
    write: false,
    define: {
      'process.env.NODE_ENV': '"production"',
      DETAIL_TITLE: JSON.stringify(title),
      LIVE_SSH: JSON.stringify(live),
      METRIC_LABEL: JSON.stringify(label),
    },
  })
  const stage = mkdtempSync(join(tmpdir(), 'resource-monitor-'))
  try {
    mkdirSync(join(stage, 'ui'))
    writeFileSync(join(stage, 'manifest.json'), JSON.stringify({
      schemaVersion: 1,
      id: 'huabei.resource-monitor',
      name: 'Server resources prototype',
      version,
      runtime: 'sandbox',
      entry: 'plugin.js',
      permissions: live ? ['process.execute'] : [],
      ...(live && {execution: {programs: ['sh'], targets: ['ssh']}}),
      contributes: {
        terminalStatusBar: {id: 'huabei.resource-monitor.status'},
        commands: live
          ? [{id: 'monitor.open', title: 'Open resource details', action: {type: 'view.open', view: 'monitor.details'}}]
          : [{id: 'monitor.snapshot', title: 'Read snapshot'}],
        views: [{id: 'monitor.details', title: 'Resource details', entry: 'ui/details.html', placement: 'terminal.sidePanel'}],
      },
      engines: {hostApi: 1},
    }))
    const parser = `function sample(raw) {
  const line = raw.split(/\\r?\\n/).find((line) => line.startsWith('${key}:'));
  const field = line && line.match(/^\\S+:\\s+(\\d+)\\s+kB$/);
  if (!field) throw Error('Missing ${key}');
  return { value: Math.floor(Number(field[1]) / 1024) };
}
`
    const probe = `
const api = globalThis.atermPlugin;
api.onRefresh(async ({token}) => {
  try {
    const response = await api.process.run({
      token, executable: 'sh', args: ['-c', 'uname -s && grep "^${key}:" /proc/meminfo'], timeoutMs: 5000
    });
    if (response.exitCode !== 0) throw Error('Linux sample failed');
    const snapshot = sample(response.stdout);
    await api.request('status.publish', {
      token, state: 'ready', data: snapshot,
      items: [{id: 'memory', label: ${JSON.stringify(label)}, value: snapshot.value + ' MiB', command: 'monitor.open'}]
    });
  } catch {
    await api.request('status.publish', {token, state: 'unavailable', items: []});
  }
});
`
    writeFileSync(join(stage, 'plugin.js'), parser + (live ? probe : ''))
    const js = bundle.outputFiles[0].text.replace(/<\/script/gi, '<\\/script')
    writeFileSync(join(stage, 'ui/details.html'), `<!doctype html>
<html lang="en"><meta charset="utf-8"><title>${title}</title>
<style>
  :root { color-scheme: light dark; font: menu; }
  main { padding: 16px; }
  h1 { font-size: 16px; font-weight: 600; margin: 0 0 16px; }
  output { display: block; font-size: 24px; margin-top: 4px; }
</style>
<div id="root"></div><script>${js}</script></html>`)
    execFileSync('go', ['run', './scripts/pack-plugin', stage, join(out, `resource-monitor-${version}.aterm-plugin`)], {cwd: repo, stdio: 'inherit'})
  } finally {
    rmSync(stage, {recursive: true, force: true})
  }
}
