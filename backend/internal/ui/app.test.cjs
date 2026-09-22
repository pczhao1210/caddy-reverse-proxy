const assert = require('node:assert/strict');
const { readFileSync } = require('node:fs');
const { join } = require('node:path');
const { test } = require('node:test');
const { runInNewContext } = require('node:vm');

function createApp() {
  const storage = new Map([['gatewayToken', 'valid-token']]);
  let factory;
  runInNewContext(readFileSync(join(__dirname, 'static/app.js'), 'utf8'), {
    navigator: { language: 'en' },
    document: { addEventListener: (_, callback) => callback() },
    Alpine: { data: (_, callback) => { factory = callback; } },
    localStorage: {
      getItem: (key) => storage.get(key),
      removeItem: (key) => storage.delete(key)
    }
  });
  const app = factory();
  app.clearMessages = () => {};
  app.showAlert = (message) => { app.alert = message; };
  app.openLogin = () => { app.loginOpened = true; };
  app.translateBackendText = (text) => text;
  return { app, storage };
}

for (const method of ['runAction', 'refreshAll']) {
  for (const status of [401, 503]) {
    test(`${method}: ${status} only invalidates credentials for 401`, async () => {
      const { app, storage } = createApp();
      app.activeView = 'routes';
      const fail = async () => { throw Object.assign(new Error('service unavailable'), { status }); };
      app.api = fail;
      await app[method](method === 'runAction' ? fail : {});
      assert.equal(app.token, status === 401 ? '' : 'valid-token');
      assert.equal(storage.has('gatewayToken'), status !== 401);
      assert.equal(Boolean(app.loginOpened), status === 401);
      assert.equal(app.activeView, 'routes');
      assert.equal(app.isActing, false);
      assert.equal(app.isRefreshing, false);
      if (status === 503) assert.match(app.alert, /service unavailable/);
    });
  }
}

test('historical certificates do not show a renewal warning in the summary', () => {
  const { app } = createApp();
  app.locale = 'zh-CN';
  const historical = { usage: 'covered', state: 'renewal_due' };
  assert.equal(app.certificateSummaryState(historical).className, '');
  assert.match(app.certificateSummaryState(historical).text, /历史/);
  assert.match(app.certificateState(historical).text, /预计/);
  app.setCertificateRuntime({ policyKnown: true, certificates: [historical, { usage: 'managed' }] });
  assert.equal(app.filteredCertificates()[0].usage, 'managed');
  app.certificateFilter = 'history';
  assert.equal(app.filteredCertificates().length, 1);
});

test('archive requires known runtime, unchanged form, and explicit confirmation', async () => {
  const { app } = createApp();
  app.configurationImportPending = () => false;
  app.$refs = { archiveCertificateDialog: { showModal() {}, close() {} } };
  const certificate = { id: 'opaque', fingerprintSha256: 'fingerprint', canArchive: true };
  assert.equal(app.canArchiveCertificate(certificate), false);
  app.certificateRuntime.policyKnown = true;
  app.certificateDirty = true;
  assert.equal(app.canArchiveCertificate(certificate), false);
  app.certificateDirty = false;
  const calls = [];
  app.api = async (path, options) => {
    calls.push({ path, options });
    return path.endsWith('archive') ? { directory: '/archive/materials' } : { runtime: { policyKnown: true, certificates: [] } };
  };
  app.requestArchiveCertificate(certificate);
  assert.equal(calls.length, 0);
  await app.confirmArchiveCertificate();
  assert.equal(calls[0].path, '/api/certificate/archive');
  assert.deepEqual(JSON.parse(calls[0].options.body), { id: 'opaque', fingerprintSha256: 'fingerprint', confirm: true });
  assert.equal(calls[1].path, '/api/certificate');
  assert.equal(app.certificateToArchive, null);
});