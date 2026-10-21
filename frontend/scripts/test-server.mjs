import { spawn } from 'node:child_process';
import { mkdtemp, mkdir, rm } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { join, resolve } from 'node:path';

const directory = await mkdtemp(join(tmpdir(), 'quarro-e2e-'));
const executable = join(directory, process.platform === 'win32' ? 'server.exe' : 'server');
let child;
let stopping = false;
for (const signal of ['SIGINT', 'SIGTERM'])
    process.on(signal, () => {
        stopping = true;
        child?.kill(signal);
    });
function run(command, args, options = {}) {
    return new Promise((resolve, reject) => {
        child = spawn(command, args, { stdio: 'inherit', ...options });
        child.once('error', reject);
        child.once('exit', (code, signal) => {
            child = undefined;
            if (code === 0 || stopping) resolve();
            else reject(new Error(`${command} exited with ${code ?? signal}`));
        });
    });
}
try {
    await run(process.execPath, ['node_modules/vite/bin/vite.js', 'build', '--mode', 'test']);
    if (!stopping)
        await run('go', ['build', '-tags', 'server', '-o', executable, '.'], {
            cwd: resolve('..'),
        });
    await mkdir(join(directory, 'data'));
    await mkdir(join(directory, 'config'));
    if (!stopping)
        await run(executable, [], {
            env: {
                ...process.env,
                XDG_DATA_HOME: join(directory, 'data'),
                XDG_CONFIG_HOME: join(directory, 'config'),
                WAILS_SERVER_PORT: '34115',
            },
        });
} finally {
    await rm(directory, { recursive: true, force: true });
}
