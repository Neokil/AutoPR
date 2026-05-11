import { execSync, spawn } from 'child_process';
import * as fs from 'fs';
import * as path from 'path';
import * as os from 'os';

const PID_FILE = '/tmp/autopr-e2e.pid';
export const FIXTURES_FILE = '/tmp/autopr-e2e-fixtures.json';

export interface E2EFixtures {
  repoDir: string;
  ticketId: string;
}

export default async function globalSetup(): Promise<void> {
  const tmpDir = fs.mkdtempSync(path.join(os.tmpdir(), 'autopr-e2e-'));
  const repoDir = path.join(tmpDir, 'repo');
  const homeDir = path.join(tmpDir, 'home');
  const mockProviderPath = path.join(tmpDir, 'mock-provider');

  // Fake HOME so ~/.auto-pr/config.yaml resolves inside our tmpDir
  fs.mkdirSync(path.join(homeDir, '.auto-pr'), { recursive: true });

  setupGitRepo(repoDir);
  writeWorkflowAndPrompts(repoDir);
  writeMockProvider(mockProviderPath);
  writeConfig(homeDir, repoDir, mockProviderPath);

  const fixtures: E2EFixtures = { repoDir, ticketId: 'TEST-1' };
  fs.writeFileSync(FIXTURES_FILE, JSON.stringify(fixtures));

  await startDaemon(homeDir);
}

function run(cmd: string): void {
  execSync(cmd, { stdio: 'pipe' });
}

function setupGitRepo(repoDir: string): void {
  fs.mkdirSync(repoDir, { recursive: true });
  run(`git init ${repoDir}`);
  run(`git -C ${repoDir} config user.email "e2e@test.local"`);
  run(`git -C ${repoDir} config user.name "E2E Test"`);
  fs.writeFileSync(path.join(repoDir, 'README.md'), '# E2E Test Repo');
  run(`git -C ${repoDir} add README.md`);
  run(`git -C ${repoDir} commit -m "Initial commit"`);
}

function writeWorkflowAndPrompts(repoDir: string): void {
  const autoPrDir = path.join(repoDir, '.auto-pr');
  fs.mkdirSync(path.join(autoPrDir, 'prompts'), { recursive: true });

  fs.writeFileSync(
    path.join(autoPrDir, 'workflow.yaml'),
    `states:
  - name: investigate
    prompt: prompts/investigate.md
    pre_prompt_commands: []
    post_prompt_commands: []
    actions:
      - label: "Approve"
        type: move_to_state
        target: implementation
      - label: "Decline"
        type: move_to_state
        target: cancelled

  - name: implementation
    prompt: prompts/implement.md
    pre_prompt_commands: []
    post_prompt_commands: []
    actions:
      - label: "Accept"
        type: move_to_state
        target: done
      - label: "Cancel"
        type: move_to_state
        target: cancelled
`,
  );

  fs.writeFileSync(
    path.join(autoPrDir, 'prompts', 'investigate.md'),
    'Investigate the ticket and provide your findings.',
  );
  fs.writeFileSync(
    path.join(autoPrDir, 'prompts', 'implement.md'),
    'Implement the ticket based on the investigation.',
  );
}

function writeMockProvider(mockProviderPath: string): void {
  fs.writeFileSync(
    mockProviderPath,
    `#!/bin/sh
# Discard stdin (the prompt content) and return a canned response.
# Sleep 100ms to simulate realistic AI provider response latency.
cat > /dev/null
sleep 0.1
echo "Mock provider: analysis complete."
`,
  );
  fs.chmodSync(mockProviderPath, 0o755);
}

function writeConfig(homeDir: string, repoDir: string, mockProviderPath: string): void {
  fs.writeFileSync(
    path.join(homeDir, '.auto-pr', 'config.yaml'),
    `repository_directories:
  - ${repoDir}
provider: mock
create_pr: false
providers:
  mock:
    command: ${mockProviderPath}
    args: []
`,
  );
}

async function startDaemon(homeDir: string): Promise<void> {
  const daemon = spawn('/usr/local/bin/auto-prd', [], {
    env: { ...process.env, HOME: homeDir },
    stdio: ['ignore', 'pipe', 'pipe'],
    detached: false,
  });

  if (!daemon.pid) {
    throw new Error('Failed to start auto-prd daemon');
  }

  fs.writeFileSync(PID_FILE, String(daemon.pid));

  daemon.stdout.on('data', (chunk: Buffer) => process.stdout.write(`[daemon] ${chunk}`));
  daemon.stderr.on('data', (chunk: Buffer) => process.stderr.write(`[daemon] ${chunk}`));

  daemon.on('exit', (code) => {
    if (code !== null && code !== 0) {
      console.error(`[daemon] exited with code ${code}`);
    }
  });

  await waitForHealth();
}

async function waitForHealth(): Promise<void> {
  const deadline = Date.now() + 15_000;
  while (Date.now() < deadline) {
    try {
      const res = await fetch('http://localhost:8080/api/health');
      if (res.ok) return;
    } catch {
      // not ready yet
    }
    await new Promise<void>((r) => setTimeout(r, 300));
  }
  throw new Error('auto-prd daemon did not become healthy within 15s');
}
