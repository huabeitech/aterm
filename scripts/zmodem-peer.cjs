// Disposable protocol peer for native migration acceptance. Uses the same
// zmodem.js-ex dependency as Electron and sends only an explicitly selected
// temporary fixture file; it is not part of the production runtime.
const fs = require('node:fs');
const path = require('node:path');
const os = require('node:os');
const {execFileSync} = require('node:child_process');
const stty = fs.existsSync('/bin/stty') ? '/bin/stty' : '/usr/bin/stty';
let savedTTY;
const args = new Map();
for (let i = 2; i < process.argv.length; i += 2) args.set(process.argv[i], process.argv[i + 1]);
const file = path.resolve(args.get('--file') || '');
const temporaryRoots = [os.tmpdir(), ...(process.platform === 'win32' ? [] : ['/tmp'])].filter(value => fs.existsSync(value)).map(value => fs.realpathSync(value));
const realFile = fs.realpathSync(file);
if (!temporaryRoots.some(root => realFile.startsWith(root + path.sep)) || !fs.statSync(realFile).isFile())
  throw Error('Provide a regular disposable file under the system temporary directory.');
const library = args.get('--library') || path.resolve(__dirname, '../frontend/node_modules/zmodem.js-ex');
const Zmodem = require(library);
const {Header} = require(path.join(library, 'src/zheader.js'));
let outbound = Promise.resolve();
let finished = false;
let running = false;
const write = bytes => {
  const data = Buffer.from(bytes);
  outbound = outbound.then(() => new Promise((resolve, reject) => process.stdout.write(data, error => error ? reject(error) : resolve())));
};
const restore = () => {
  if (process.stdin.isTTY) {
    process.stdin.setRawMode(false);
    if (savedTTY) {try {execFileSync(stty, [savedTTY], {stdio: [0, 'ignore', 'ignore']});} catch {}}
  }
  process.stdin.pause(); clearTimeout(timer);
};
const fail = error => {
  if (finished) return;
  finished = true; restore();
  process.stderr.write(`ZMODEM fixture: ${error.message || error}\n`, () => process.exit(1));
};
const timer = setTimeout(() => fail(Error('peer timed out')), 120000);
const sentry = new Zmodem.Sentry({
  to_terminal: () => {},
  sender: write,
  on_retract: () => {},
  on_detect: detection => {
    if (running) { detection.deny(); return; }
    running = true;
    const session = detection.confirm();
    void (async () => {
      if (session.type !== 'send') throw Error('expected a receiving terminal');
      const info = fs.statSync(realFile);
      const transfer = await session.send_offer({name: args.get('--name') || path.basename(file), size: info.size, mtime: info.mtime, files_remaining: 1, bytes_remaining: info.size});
      if (transfer) {
        for await (const chunk of fs.createReadStream(realFile, {start: transfer.get_offset(), highWaterMark: 8192})) {
          transfer.send(Array.from(chunk));
          await outbound;
          const delay = Math.min(1000, Math.max(0, Number(args.get('--delay-ms')) || 0));
          if (delay) await new Promise(resolve => setTimeout(resolve, delay));
        }
        await transfer.end(); await outbound;
      }
      await session.close(); await outbound;
      write(Buffer.from('ZMODEM fixture completed\r\n')); await outbound;
      finished = true; restore(); process.exit(0);
    })().catch(fail);
  }
});
process.stdin.on('data', bytes => {try {sentry.consume(Array.from(bytes));} catch (error) {fail(error);}});
process.stdin.on('error', fail);
process.stdout.on('error', fail);
if (process.stdin.isTTY) {
  savedTTY = execFileSync(stty, ['-g'], {stdio: [0, 'pipe', 'ignore']}).toString().trim();
  process.stdin.setRawMode(true);
  // libuv's keyboard raw mode can preserve output postprocessing. A wire
  // protocol peer must also prevent LF -> CRLF conversion on its own PTY.
  execFileSync(stty, ['raw', '-echo'], {stdio: [0, 'ignore', 'ignore']});
}
process.stdin.resume();
write(Header.build('ZRQINIT').to_hex());
void outbound.catch(fail);
