import { build } from '../../frontend/node_modules/vite/dist/node/index.js'
import react from '../../frontend/node_modules/@vitejs/plugin-react/dist/index.js'
import tailwind from '../../frontend/node_modules/@tailwindcss/vite/dist/index.mjs'
import { execFileSync } from 'node:child_process'
import { createHash } from 'node:crypto'
import { mkdirSync, mkdtempSync, readFileSync, writeFileSync, copyFileSync, rmSync } from 'node:fs'
import { tmpdir } from 'node:os'
import { resolve, join, dirname } from 'node:path'
import { fileURLToPath } from 'node:url'
const root = dirname(fileURLToPath(import.meta.url)),
  repo = resolve(root, '../..')
const platform = process.env.GOOS ?? process.platform
const arch = process.env.GOARCH ?? (process.arch === 'x64' ? 'amd64' : process.arch)
const target = `${platform === 'win32' ? 'windows' : platform}-${arch}`
const stage = mkdtempSync(join(tmpdir(), 'aterm-native-'))
try {
  const manifest = JSON.parse(readFileSync(join(root, 'manifest.json'), 'utf8'))
  manifest.entry = `backend/${target}/plugin${target.startsWith('windows-') ? '.exe' : ''}`
  manifest.backend.platform = target
  const binary = join(stage, manifest.entry)
  mkdirSync(dirname(binary), { recursive: true })
  execFileSync(
    process.platform === 'darwin' && target.startsWith('darwin-') ? 'python3' : 'go',
    [
      ...(process.platform === 'darwin' && target.startsWith('darwin-')
        ? ['scripts/go-macos.py']
        : []),
      'build',
      '-trimpath',
      '-ldflags=-s -w',
      '-o',
      binary,
      './plugins/ssh-tunnels/backend',
    ],
    {
      cwd: repo,
      env: { ...process.env, GOOS: target.split('-')[0], GOARCH: arch, CGO_ENABLED: '0' },
      stdio: 'inherit',
    },
  )
  if (process.platform === 'darwin' && target.startsWith('darwin-'))
    execFileSync(
      'codesign',
      ['--force', '--sign', process.env.ATERM_PLUGIN_SIGN_IDENTITY ?? '-', binary],
      { stdio: 'inherit' },
    )
  manifest.backend.sha256 = createHash('sha256').update(readFileSync(binary)).digest('hex')
  const output = await build({
    configFile: false,
    define:{'process.env.NODE_ENV':JSON.stringify('production')},
    root: join(repo, 'frontend'),
    plugins: [react(), tailwind()],
    resolve: {
      alias: {
        '@': join(repo, 'frontend/src/renderer/src'),
        ...Object.fromEntries(
          ['react', 'react-dom', 'i18next', 'react-i18next', 'lucide-react'].map((name) => [
            name,
            join(repo, 'frontend/node_modules', name),
          ]),
        ),
      },
    },
    build: {
      write: false,
      lib: { entry: join(root, 'src/ui/main.tsx'), name: 'TunnelPlugin', formats: ['iife'] },
      cssCodeSplit: false,
      assetsInlineLimit: 10000000,
      rollupOptions: { output: { inlineDynamicImports: true } },
    },
  })
  const files = (Array.isArray(output) ? output[0] : output).output
  const js = files.find((f) => f.type === 'chunk').code.replace(/<\/script/gi, '<\\/script')
  const css = files
    .filter((f) => f.type === 'asset' && f.fileName.endsWith('.css'))
    .map((f) => f.source)
    .join('\n')
    .replace(/<\/style/gi, '<\\/style')
  mkdirSync(join(stage, 'ui'))
  writeFileSync(
    join(stage, 'ui/workspace.html'),
    `<!doctype html><html><head><meta charset="utf-8"><style>${css}</style></head><body><div id="root"></div><script>${js}</script></body></html>`,
  )
  mkdirSync(join(stage, 'locales'))
  for (const path of Object.values(manifest.i18n.locales))
    copyFileSync(join(root, path), join(stage, path))
  writeFileSync(join(stage, 'manifest.json'), JSON.stringify(manifest, null, 2))
  const out = resolve(process.argv[2] ?? join(repo, 'build/bin/plugins'))
  mkdirSync(out, { recursive: true })
  const archive = join(out, `ssh-tunnels-${manifest.version}-${target}.aterm-plugin`)
  execFileSync('go', ['run', './scripts/pack-plugin', stage, archive], {
    cwd: repo,
    stdio: 'inherit',
    env: {
      ...process.env,
      GOOS: process.platform === 'win32' ? 'windows' : process.platform,
      GOARCH: process.arch === 'x64' ? 'amd64' : process.arch,
      CGO_ENABLED: '0',
    },
  })
  console.log(archive)
} finally {
  rmSync(stage, { recursive: true, force: true })
}
