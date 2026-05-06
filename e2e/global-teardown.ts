import * as fs from 'fs';

const PID_FILE = '/tmp/autopr-e2e.pid';
const FIXTURES_FILE = '/tmp/autopr-e2e-fixtures.json';

export default function globalTeardown(): void {
  try {
    const pid = parseInt(fs.readFileSync(PID_FILE, 'utf-8').trim(), 10);
    process.kill(pid, 'SIGTERM');
  } catch {
    // daemon may have already exited
  }

  for (const f of [PID_FILE, FIXTURES_FILE]) {
    try {
      fs.unlinkSync(f);
    } catch {
      // ignore
    }
  }
}
