const assert = require('node:assert/strict');
const { spawnSync } = require('node:child_process');
const { mkdtempSync, writeFileSync, readFileSync, existsSync, rmSync } = require('node:fs');
const { tmpdir } = require('node:os');
const { join, resolve } = require('node:path');
const { test } = require('node:test');

const root = resolve(__dirname, '..');
const index = 'sha256:' + '1'.repeat(64);
const dockerStub = `#!/usr/bin/env node
const fs = require('node:fs');
const crypto = require('node:crypto');
const path = require('node:path');
const args = process.argv.slice(2);
const scenario = process.env.SCENARIO;
const work = process.env.FIXTURE_DIR;
fs.appendFileSync(work + '/commands', JSON.stringify(['docker', ...args]) + '\\n');
const digest = digit => 'sha256:' + digit.repeat(64);
const emit = value => console.log(JSON.stringify(value));
const manifests = [
  {digest:digest('2'),platform:{os:'linux',architecture:'amd64'}},
  {digest:digest('3'),platform:{os:'linux',architecture:'arm64'}},
  {digest:digest('4'),platform:{os:'unknown',architecture:'unknown'},annotations:{'vnd.docker.reference.type':'attestation-manifest'}}
];
if (args[0] === 'buildx' && args[1] === 'inspect') process.exit(scenario === 'missing-builder' ? 1 : 0);
if (args[0] === 'buildx' && args[1] === 'build') {
  if (scenario === 'build-failure') process.exit(1);
  fs.writeFileSync(args[args.indexOf('--metadata-file')+1], JSON.stringify({'containerimage.digest':digest('1')}));
} else if (args[0] === 'buildx' && args[1] === 'imagetools' && args[2] === 'inspect') {
  if (args[3].includes('@')) {
    if (scenario === 'missing-arm64') manifests.splice(1,1);
    if (scenario === 'duplicate-platform') manifests.push(manifests[0]);
    emit({digest:digest('1'),manifests});
  } else if (fs.existsSync(work + '/promoted')) {
    emit({digest:digest(scenario === 'wrong-published-digest' ? '9' : '1'),manifests});
  } else {
    const count = Number(fs.existsSync(work+'/reads') ? fs.readFileSync(work+'/reads','utf8') : 0);
    fs.writeFileSync(work+'/reads',String(count+1));
    if (scenario === 'new-target') { console.error('manifest unknown'); process.exit(1); }
    if (scenario === 'registry-error') { console.error('unauthorized'); process.exit(1); }
    emit({digest:digest(scenario === 'concurrent-publisher' && count > 0 ? '9' : '0')});
  }
} else if (args[0] === 'buildx' && args[1] === 'imagetools' && args[2] === 'create') {
  fs.writeFileSync(work+'/promoted','yes');
} else if (args[0] === 'image' && args[1] === 'inspect') {
  const arm = args[2].includes(digest('3')) || args[2] === digest('7');
  if (args.includes('{{.Id}}')) console.log(digest(arm ? '7' : '6'));
  else if (args.includes('{{.Architecture}}')) console.log(arm ? 'arm64' : 'amd64');
  else emit({Os:'linux',Architecture:arm ? 'arm64' : 'amd64'});
} else if (args[0] === 'create') {
  console.log(args.at(-1) === digest('7') ? 'fixture-scan-arm64' : 'fixture-scan-amd64');
} else if (args[0] === 'cp') {
  const hash = data => crypto.createHash('sha256').update(data).digest('hex');
  if (args[1].endsWith('/security/.')) {
    const architecture = args[1].includes('arm64') ? 'arm64' : 'amd64';
    const sources = [{Path:'go.step.sm/crypto',Version:'v0.91.1'},{Path:'github.com/smallstep/certificates',Version:'v0.30.2'}];
    const proof = JSON.stringify({version:1,sources,artifacts:['caddy','platform'].map(name => ({
      path:name === 'caddy' ? '/usr/bin/caddy' : '/usr/local/bin/platform',
      sha256:hash(name + '-fixture'),goos:'linux',goarch:architecture,cryptoVersion:'v0.58.0',packageCount:100,moduleCount:20
    }))}) + '\\n';
    fs.writeFileSync(path.join(args[2],'evidence.json'),proof);
    fs.writeFileSync(path.join(args[2],'openpgp.vex.json'),JSON.stringify({
      '@context':'https://openvex.dev/ns/v0.2.0','@id':'urn:sha256:' + hash(proof),
      statements:[{vulnerability:{name:'GO-2026-5932'},status:'not_affected',justification:'vulnerable_code_not_present',
        products:[{'@id':'pkg:golang/golang.org/x/crypto@v0.58.0'}]}]
    }));
  } else {
    const name = path.basename(args[1]);
    fs.writeFileSync(args[2],scenario === 'scan-evidence-tampered' && name === 'caddy' ? 'tampered' : name + '-fixture');
  }
} else if (args[0] === 'run') {
  const arm = args.includes('linux/arm64');
  if (scenario === 'arm64-run-failure' && arm) process.exit(1);
  console.log(arm ? 'fixture-arm64' : 'fixture-amd64');
} else if (args[0] === 'exec') {
  if (args[2] === '/bin/sh') {
    if (scenario === 'arm64-exits-before-elf' && args[1] === 'fixture-arm64') {
      console.error('container is not running');
      process.exit(1);
    }
    console.log(args[1] === 'fixture-arm64' && scenario !== 'wrong-elf' ? '183\\n183' : '62\\n62');
  }
  if (args[3] === 'version') console.log(scenario === 'wrong-caddy-version' ? 'v2.11.4 fixture' : 'v2.11.7 fixture');
  if (args[3] === 'list-modules') console.log('dns.providers.azure');
} else if (args[0] === 'container' && args[1] === 'inspect') {
  if (args.includes('{{json .State}}')) {
    const arm = args[2] === 'fixture-arm64';
    const countFile = work + '/state-' + args[2];
    const count = fs.existsSync(countFile) ? Number(fs.readFileSync(countFile,'utf8')) : 0;
    fs.writeFileSync(countFile,String(count+1));
    const stopped = arm && (scenario === 'arm64-startup-exit' || (scenario === 'arm64-exits-before-elf' && count > 0));
    emit({Status:stopped ? 'exited' : 'running',Running:!stopped,ExitCode:stopped ? 255 : 0,OOMKilled:false,Error:''});
  } else emit({'8080/tcp':[{HostIp:'127.0.0.1',HostPort:'28080'}]});
} else if (args[0] === 'logs') {
  if (scenario === 'arm64-startup-exit' || scenario === 'arm64-exits-before-elf') console.log('exec /usr/local/bin/platform: exec format error');
}
`;
const curlStub = `#!/usr/bin/env node
const fs = require('node:fs');
const args = process.argv.slice(2);
fs.appendFileSync(process.env.FIXTURE_DIR+'/commands',JSON.stringify(['curl',...args])+'\\n');
if (process.env.SCENARIO === 'unready' && args.at(-1).endsWith('/readyz')) process.exit(22);
if (args.includes('--write-out')) process.stdout.write(process.env.SCENARIO === 'auth-bypass' ? '200' : '401');
const output = args[args.indexOf('--output')+1];
if (output && output !== '/dev/null') fs.writeFileSync(output,'{}');
`;
const trivyStub = `#!/usr/bin/env node
const fs = require('node:fs');
const args = process.argv.slice(2);
fs.appendFileSync(process.env.FIXTURE_DIR+'/commands',JSON.stringify(['trivy',...args])+'\\n');
const report = {Results:[
  {Type:'gobinary',Target:'usr/bin/caddy'},{Type:'gobinary',Target:'usr/local/bin/platform'}
]};
if (args[0] === 'sbom') {
  const bom = JSON.parse(fs.readFileSync(args.at(-1),'utf8'));
  report.Results = [{Type:'gobinary',Packages:bom.components.map(component => ({Name:component.name,Version:component.version}))}];
}
fs.writeFileSync(args[args.indexOf('--output')+1],JSON.stringify(report));
if (process.env.SCENARIO === 'scan-vulnerability' && args.includes('--vex') &&
    (process.env.TRIVY_IGNORE_UNFIXED !== 'true' || args.includes('--ignore-unfixed=false'))) process.exit(1);
`;

function run(scenario, command = ['sh', join(__dirname, 'publish-multiarch.sh'), 'example/gateway:latest'], overrides = {}) {
  const directory = mkdtempSync(join(tmpdir(), 'gateway-publish-test-'));
  try {
    writeFileSync(join(directory, 'docker'), dockerStub, { mode: 0o755 });
    writeFileSync(join(directory, 'curl'), curlStub, { mode: 0o755 });
    writeFileSync(join(directory, 'trivy'), trivyStub, { mode: 0o755 });
    const result = spawnSync(command[0], command.slice(1), {
      cwd: root,
      encoding: 'utf8',
      env: { ...process.env, PATH: directory + ':' + process.env.PATH, FIXTURE_DIR: directory, SCENARIO: scenario, MULTIARCH_BUILDER: 'fixture-builder', IMAGE: 'example/gateway:latest', PUSH_IMAGE: '', ...overrides }
    });
    const log = join(directory, 'commands');
    const commands = existsSync(log) ? readFileSync(log, 'utf8').trim().split('\n').map(line => JSON.parse(line)) : [];
    return { ...result, commands };
  } finally {
    rmSync(directory, { recursive: true, force: true });
  }
}

const promotions = result => result.commands.filter(command => command.slice(0, 4).join(' ') === 'docker buildx imagetools create');

test('ambient ignore-unfixed cannot bypass the publishing vulnerability gate', () => {
  const result = run('scan-vulnerability', undefined, { TRIVY_IGNORE_UNFIXED: 'true', TRIVY_SEVERITY: 'CRITICAL' });
  assert.notEqual(result.status, 0);
  assert.equal(promotions(result).length, 0);
});

test('an older Caddy runtime cannot be promoted', () => {
  const result = run('wrong-caddy-version');
  assert.notEqual(result.status, 0);
  assert.equal(promotions(result).length, 0);
});

for (const scenario of ['arm64-startup-exit', 'arm64-exits-before-elf']) {
  test(`${scenario} reports startup diagnostics, not a wrong ELF architecture`, () => {
    const result = run(scenario);
    assert.notEqual(result.status, 0);
    assert.equal(promotions(result).length, 0);
    assert.match(result.stderr, /exec format error/);
    assert.match(result.stderr, /"ExitCode":255/);
    assert.doesNotMatch(result.stderr, /Wrong ELF architecture/);
    assert(result.commands.some(command => command[1] === 'logs' && command.at(-1) === 'fixture-arm64'));
    assert(result.commands.some(command => command[1] === 'rm' && command.at(-1) === 'fixture-arm64'));
  });
}

test('a genuine ELF mismatch still blocks promotion', () => {
  const result = run('wrong-elf');
  assert.notEqual(result.status, 0);
  assert.equal(promotions(result).length, 0);
  assert.match(result.stderr, /Wrong ELF architecture/);
});

test('both architectures are verified before promoting the immutable candidate', () => {
  const result = run('success');
  assert.equal(result.status, 0, result.stderr);
  const build = result.commands.find(command => command[1] === 'buildx' && command[2] === 'build');
  assert.equal(build[build.indexOf('--platform') + 1], 'linux/amd64,linux/arm64');
  assert.match(build[build.indexOf('--tag') + 1], /^example\/gateway:multiarch-/);
  assert.equal(promotions(result).length, 1);
  assert.equal(promotions(result)[0].at(-1), 'example/gateway@' + index);
  const containers = result.commands.filter(command => command[1] === 'run');
  assert.equal(containers.length, 2);
  assert(containers.every(command => !command.includes('-v') && command.includes('GATEWAY_DOCKER_ENABLED=false')));
  assert.deepEqual(result.commands.filter(command => command[1] === 'rm').map(command => command.at(-1)), ['fixture-scan-amd64', 'fixture-amd64', 'fixture-scan-arm64', 'fixture-arm64']);
  assert.equal(result.commands.filter(command => command[0] === 'trivy').length, 6);
  assert(!result.commands.some(command => command[1] === 'push'));
});

for (const scenario of ['missing-builder', 'build-failure', 'missing-arm64', 'duplicate-platform', 'arm64-run-failure', 'wrong-elf', 'unready', 'auth-bypass', 'concurrent-publisher', 'registry-error', 'scan-vulnerability', 'scan-evidence-tampered']) {
  test(`${scenario} cannot update the target tag`, () => {
    const result = run(scenario);
    assert.notEqual(result.status, 0);
    assert.equal(promotions(result).length, 0);
    assert(result.commands.filter(command => command[1] === 'rm').every(command => /^fixture-(scan-)?(amd64|arm64)$/.test(command.at(-1))));
  });
}

test('a new target can be published, but post-promotion mismatch is reported', () => {
  assert.equal(run('new-target').status, 0);
  const result = run('wrong-published-digest');
  assert.notEqual(result.status, 0);
  assert.match(result.stderr, /Published digest differs/);
  assert(!result.stderr.includes('has not updated'));
});

test('start.sh push normalizes the selected repository to latest', () => {
  for (const overrides of [{ IMAGE: 'example/custom:old' }, { IMAGE: 'local', PUSH_IMAGE: 'example/custom@sha256:' + '8'.repeat(64) }]) {
    const result = run('success', ['sh', join(root, 'start.sh'), 'push'], overrides);
    assert.equal(result.status, 0, result.stderr);
    const promotion = promotions(result)[0];
    assert.equal(promotion[promotion.indexOf('--tag') + 1], 'example/custom:latest');
  }
});

test('make docker-push retains an explicitly supplied release tag', () => {
  const result = run('success', ['make', 'docker-push', 'IMAGE=example/gateway:release-test']);
  assert.equal(result.status, 0, result.stderr);
  const promotion = promotions(result)[0];
  assert.equal(promotion[promotion.indexOf('--tag') + 1], 'example/gateway:release-test');
});

test('local build remains local and start.sh rejects a non-Docker-Hub push', () => {
  const build = run('success', ['sh', join(root, 'start.sh'), 'build']);
  assert.equal(build.status, 0, build.stderr);
  assert(build.commands.some(command => command[1] === 'build'));
  assert.equal(promotions(build).length, 0);
  assert(!build.commands.some(command => command.includes('--push')));
  const invalid = run('success', ['sh', join(root, 'start.sh'), 'push'], { PUSH_IMAGE: 'registry.example.test/gateway' });
  assert.notEqual(invalid.status, 0);
  assert.equal(promotions(invalid).length, 0);
});