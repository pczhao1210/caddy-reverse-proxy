const test = require('node:test');
const assert = require('node:assert/strict');
const { spawnSync } = require('node:child_process');
const { mkdtempSync, mkdirSync, writeFileSync, readFileSync, existsSync, rmSync } = require('node:fs');
const { createHash } = require('node:crypto');
const { tmpdir } = require('node:os');
const { join, resolve } = require('node:path');

const script = resolve(__dirname, 'scan-image.sh');
const imageID = 'sha256:' + 'a'.repeat(64);
const hash = (value) => createHash('sha256').update(value).digest('hex');
const ambientModes = [
  'ambient-ignore-unfixed', 'ambient-severity', 'ambient-ignore-status',
  'ambient-vex', 'ambient-skip-files', 'ambient-disabled-analyzers',
  'ambient-config', 'ambient-ignorefile', 'ambient-default-config',
  'ambient-default-ignore', 'ambient-future-filter'
];
const allowedEnvironment = [
  'TRIVY_CACHE_DIR', 'TRIVY_DB_REPOSITORY', 'TRIVY_JAVA_DB_REPOSITORY',
  'TRIVY_USERNAME', 'TRIVY_PASSWORD', 'TRIVY_REGISTRY_TOKEN',
  'TRIVY_TIMEOUT', 'TRIVY_NO_PROGRESS'
].sort();

for (const mode of ['valid', 'tampered-binary', 'wrong-architecture', 'wrong-proof', 'unrelated-vulnerability', 'extra-binary', 'raw-scan-failure', 'remaining-vulnerability', 'tampered-sbom', 'dependency-vulnerability', 'unparsed-sbom', ...ambientModes]) {
  test(`image-bound VEX: ${mode}`, () => {
    const root = mkdtempSync(join(tmpdir(), 'gateway-vex-test-'));
    try {
      const bin = join(root, 'bin');
      const fixtures = join(root, 'fixtures');
      const output = join(root, 'report');
      mkdirSync(bin);
      mkdirSync(fixtures);
      const ambient = {};
      const externalConfig = join(root, 'external-trivy.yaml');
      const externalIgnore = join(root, 'external.ignore');
      writeFileSync(externalConfig, 'severity: [CRITICAL]\nvulnerability:\n  ignore-unfixed: true\n');
      writeFileSync(externalIgnore, 'CVE-TEST-UNFIXED\n');
      switch (mode) {
        case 'ambient-ignore-unfixed': ambient.TRIVY_IGNORE_UNFIXED = 'true'; break;
        case 'ambient-severity': ambient.TRIVY_SEVERITY = 'CRITICAL'; break;
        case 'ambient-ignore-status': ambient.TRIVY_IGNORE_STATUS = 'affected'; break;
        case 'ambient-vex': ambient.TRIVY_VEX = 'external-vex.json'; break;
        case 'ambient-skip-files': ambient.TRIVY_SKIP_FILES = '**'; break;
        case 'ambient-disabled-analyzers': ambient.TRIVY_DISABLED_ANALYZERS = 'gobinary'; break;
        case 'ambient-config': ambient.TRIVY_CONFIG = externalConfig; break;
        case 'ambient-ignorefile': ambient.TRIVY_IGNOREFILE = externalIgnore; break;
        case 'ambient-default-config': writeFileSync(join(root, 'trivy.yaml'), readFileSync(externalConfig)); break;
        case 'ambient-default-ignore': writeFileSync(join(root, '.trivyignore'), readFileSync(externalIgnore)); break;
        case 'ambient-future-filter': ambient.TRIVY_FUTURE_FILTER = 'hide'; break;
      }
      const evidence = {
        version: 1,
        sources: [
          { Path: 'go.step.sm/crypto', Version: 'v0.91.1' },
          { Path: 'github.com/smallstep/certificates', Version: 'v0.30.2' }
        ],
        artifacts: [
          { path: '/usr/bin/caddy', sha256: hash('caddy-fixture'), goos: 'linux', goarch: mode === 'wrong-architecture' ? 'arm64' : 'amd64', cryptoVersion: 'v0.58.0', packageCount: 100, moduleCount: 20 },
          { path: '/usr/local/bin/platform', sha256: hash('platform-fixture'), goos: 'linux', goarch: 'amd64', cryptoVersion: 'v0.58.0', packageCount: 100, moduleCount: 20 }
        ]
      };
      const proof = JSON.stringify(evidence) + '\n';
      writeFileSync(join(fixtures, 'evidence.json'), proof);
      writeFileSync(join(fixtures, 'patched-dependencies.cdx.json'), JSON.stringify({
        bomFormat: 'CycloneDX', specVersion: '1.6',
        components: evidence.sources.map(source => ({ purl: 'pkg:golang/' + source.Path + '@' + (mode === 'tampered-sbom' ? 'v0.0.0' : source.Version) }))
      }));
      writeFileSync(join(fixtures, 'openpgp.vex.json'), JSON.stringify({
        '@context': 'https://openvex.dev/ns/v0.2.0',
        '@id': 'urn:sha256:' + (mode === 'wrong-proof' ? 'b'.repeat(64) : hash(proof)),
        statements: [{
          vulnerability: { name: mode === 'unrelated-vulnerability' ? 'CVE-OTHER' : 'GO-2026-5932' },
          status: 'not_affected', justification: 'vulnerable_code_not_present',
          products: [{ '@id': 'pkg:golang/golang.org/x/crypto@v0.58.0' }]
        }]
      }));
      writeFileSync(join(fixtures, 'caddy'), mode === 'tampered-binary' ? 'modified-caddy' : 'caddy-fixture');
      writeFileSync(join(fixtures, 'platform'), 'platform-fixture');
      const executable = (name, body) => writeFileSync(join(bin, name), '#!' + process.execPath + '\n' + body, { mode: 0o755 });
      executable('docker', `
        const fs = require('node:fs'), path = require('node:path');
        const args = process.argv.slice(2);
        fs.appendFileSync(process.env.LOG, JSON.stringify(['docker', ...args]) + '\\n');
        if (args[0] === 'image' && args[1] === 'inspect') console.log(args.includes('{{.Id}}') ? '${imageID}' : 'amd64');
        else if (args[0] === 'create') console.log('fixture-container');
        else if (args[0] === 'cp') {
          const dest = args[2];
          if (args[1].endsWith('/security/.')) {
            const names = ['evidence.json', 'openpgp.vex.json'];
            if (process.env.MODE === 'tampered-sbom') names.push('patched-dependencies.cdx.json');
            for (const name of names) fs.copyFileSync(path.join(process.env.FIXTURES, name), path.join(dest, name));
          } else fs.copyFileSync(path.join(process.env.FIXTURES, path.basename(args[1])), dest);
        } else if (args[0] !== 'rm') process.exit(2);
      `);
      executable('trivy', `
        const fs = require('node:fs');
        const args = process.argv.slice(2);
        const option = name => args.includes(name) ? args[args.indexOf(name) + 1] : undefined;
        fs.appendFileSync(process.env.LOG, JSON.stringify(['trivy', ...args]) + '\\n');
        fs.appendFileSync(process.env.LOG + '.policy', JSON.stringify({
          environment: Object.keys(process.env).filter(key => key.startsWith('TRIVY_')).sort(),
          cache: process.env.TRIVY_CACHE_DIR, database: process.env.TRIVY_DB_REPOSITORY,
          timeout: process.env.TRIVY_TIMEOUT, noProgress: process.env.TRIVY_NO_PROGRESS,
          registryCredentialsPreserved: process.env.TRIVY_USERNAME === 'fixture-user' &&
            process.env.TRIVY_PASSWORD === 'fixture-password' && process.env.TRIVY_REGISTRY_TOKEN === 'fixture-token'
        }) + '\\n');
        if (process.env.MODE === 'raw-scan-failure') process.exit(1);
        if (process.env.MODE === 'dependency-vulnerability' && args[0] === 'sbom') process.exit(1);
        const results = ['usr/bin/caddy', 'usr/local/bin/platform'];
        if (process.env.MODE === 'extra-binary') results.push('usr/bin/unverified');
        const report = { Results: results.map(Target => ({ Target, Type: 'gobinary' })) };
        if (args[0] === 'sbom') {
          const sources = JSON.parse(fs.readFileSync(process.env.FIXTURES + '/evidence.json', 'utf8')).sources;
          report.Results = [{ Type: 'gobinary', Packages: process.env.MODE === 'unparsed-sbom' ? [] : sources.map(source => ({ Name: source.Path, Version: source.Version })) }];
        }
        const configPath = option('--config') ?? process.env.TRIVY_CONFIG ?? 'trivy.yaml';
        const ignorePath = option('--ignorefile') ?? process.env.TRIVY_IGNOREFILE ?? '.trivyignore';
        const configuredFilter = configPath && fs.existsSync(configPath) && fs.readFileSync(configPath, 'utf8').includes('CRITICAL');
        const ignoredFinding = ignorePath && fs.existsSync(ignorePath) && fs.readFileSync(ignorePath, 'utf8').includes('CVE-TEST-UNFIXED');
        const hidden = configuredFilter || ignoredFinding ||
          (!args.includes('--ignore-unfixed=false') && process.env.TRIVY_IGNORE_UNFIXED === 'true') ||
          (option('--severity') ?? process.env.TRIVY_SEVERITY) === 'CRITICAL' ||
          process.env.TRIVY_IGNORE_STATUS === 'affected' || process.env.TRIVY_FUTURE_FILTER === 'hide';
        const vulnerable = process.env.MODE === 'remaining-vulnerability' && args[0] === 'image' && !hidden;
        if (vulnerable) report.Results[0].Vulnerabilities = [{ VulnerabilityID: 'CVE-TEST-UNFIXED', Severity: 'HIGH', Status: 'affected' }];
        fs.writeFileSync(args[args.indexOf('--output') + 1], JSON.stringify(report));
        if (vulnerable && args.includes('--exit-code')) process.exit(1);
      `);
      const log = join(root, 'calls.jsonl');
      const result = spawnSync('/bin/sh', [script, 'fixture:mutable-tag', output], {
        cwd: root,
        encoding: 'utf8',
        env: {
          ...process.env, PATH: bin + ':' + process.env.PATH, FIXTURES: fixtures, LOG: log,
          MODE: ambientModes.includes(mode) ? 'remaining-vulnerability' : mode,
          TRIVY_CACHE_DIR: join(root, 'cache with spaces'), TRIVY_DB_REPOSITORY: 'fixture.invalid/trivy-db:2',
          TRIVY_JAVA_DB_REPOSITORY: 'fixture.invalid/trivy-java-db:1',
          TRIVY_USERNAME: 'fixture-user', TRIVY_PASSWORD: 'fixture-password', TRIVY_REGISTRY_TOKEN: 'fixture-token',
          TRIVY_TIMEOUT: '7m', TRIVY_NO_PROGRESS: 'true', ...ambient
        }
      });
      assert.equal(result.status, mode === 'valid' ? 0 : 1, result.stdout + result.stderr);
      const calls = readFileSync(log, 'utf8').trim().split('\n').map(JSON.parse);
      const scans = calls.filter(call => call[0] === 'trivy');
      assert.equal(scans.length, ['valid', 'remaining-vulnerability', ...ambientModes].includes(mode) ? 3 : ['dependency-vulnerability', 'unparsed-sbom'].includes(mode) ? 2 : ['extra-binary', 'raw-scan-failure'].includes(mode) ? 1 : 0);
      for (const scan of scans.filter(scan => scan[1] === 'image')) assert.equal(scan.at(-1), imageID);
      for (const scan of scans) {
        assert.equal(scan[scan.indexOf('--config') + 1], '');
        assert.equal(scan[scan.indexOf('--ignorefile') + 1], '');
        assert.equal(scan[scan.indexOf('--severity') + 1], 'UNKNOWN,LOW,MEDIUM,HIGH,CRITICAL');
        assert(scan.includes('--ignore-unfixed=false'));
      }
      if (scans.length > 0) {
        const policies = readFileSync(log + '.policy', 'utf8').trim().split('\n').map(JSON.parse);
        for (const policy of policies) {
          assert.deepEqual(policy.environment, allowedEnvironment);
          assert.equal(policy.cache, join(root, 'cache with spaces'));
          assert.equal(policy.database, 'fixture.invalid/trivy-db:2');
          assert.equal(policy.timeout, '7m');
          assert.equal(policy.noProgress, 'true');
          assert.equal(policy.registryCredentialsPreserved, true);
        }
      }
      if (ambientModes.includes(mode)) {
        assert.equal(JSON.parse(readFileSync(join(output, 'raw.json'), 'utf8')).Results[0].Vulnerabilities[0].VulnerabilityID, 'CVE-TEST-UNFIXED');
      }
      assert(calls.some(call => call[0] === 'docker' && call[1] === 'rm'), 'temporary container was not cleaned');
      assert(!existsSync(join(output, 'caddy')));
      assert(!existsSync(join(output, 'platform')));
      if (mode === 'valid') {
        assert(scans[2].includes('--vex'));
        assert(scans[2].includes('--exit-code'));
        assert.equal(JSON.parse(readFileSync(join(output, 'image.json'), 'utf8')).imageID, imageID);
      }
    } finally {
      rmSync(root, { recursive: true });
    }
  });
}
