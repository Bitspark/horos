const assert = require('node:assert/strict');
const { test } = require('node:test');
const { argumentsFor, observations, diagnose } = require('./diagnose-npm-oidc.cjs');

const notice = 'npm notice Publishing to https://registry.npmjs.org/ with tag latest and public access (dry-run)';
const noCredentials = 'npm warn publish This command requires you to be logged in to https://registry.npmjs.org/ (dry-run)';
const success = 'npm verbose oidc Successfully retrieved and set token';
const env = { GITHUB_ACTIONS: 'true', GITHUB_EVENT_NAME: 'workflow_dispatch', RUNNER_ENVIRONMENT: 'github-hosted', GITHUB_REPOSITORY: 'Bitspark/horos',
  GITHUB_REF: 'refs/heads/main', GITHUB_WORKFLOW_REF: 'Bitspark/horos/.github/workflows/release.yml@refs/heads/main',
  ACTIONS_ID_TOKEN_REQUEST_URL: 'secret-url', ACTIONS_ID_TOKEN_REQUEST_TOKEN: 'secret-request-token',
  NODE_AUTH_TOKEN: '', NPM_CONFIG_USERCONFIG: '/runner/config' };

test('dry-run exit zero alone is not authentication; native credential lookup matters', () => {
  assert.equal(observations(`${noCredentials}\n${notice}`, 0).stage, 'oidc-not-observed');
  assert.equal(observations(`${success}\n${noCredentials}\n${notice}`, 0).stage, 'credential-lookup-missing-after-exchange');
  assert.equal(observations(`${success}\n${notice}`, 0).stage, 'dry-run-authenticated');
  assert.equal(observations(success, 0).stage, 'exchange-succeeded-dry-run-incomplete');
  assert.equal(observations(`${success}\n${notice}`, 1).stage, 'exchange-succeeded-dry-run-incomplete');
});

test('provider messages, headers, request URLs and token-shaped text never reach output', () => {
  const secrets = ['secret-request-token', 'npm_secret_registry_token', 'eyJsecret.header.signature', 'secret-body'];
  const raw = [
    `npm http fetch GET https://example.invalid/id?token=${secrets[0]}&audience=npm 200 17ms`,
    'npm http fetch POST 404 https://registry.npmjs.org/-/npm/v1/oidc/token/exchange/package/@bitspark%2fhoros 28ms (cache skip)',
    `npm verbose oidc Failed token exchange request with body message: ${secrets[3]}`,
    `Authorization: Bearer ${secrets[1]}`, `npm error code ${secrets[2]}`, noCredentials, notice,
  ].join('\n');
  const result = observations(raw, 0);
  assert.equal(result.stage, 'npm-token-exchange-rejected');
  assert.deepEqual(result.githubStatuses, [200]);
  assert.deepEqual(result.exchangeStatuses, [404]);
  assert.deepEqual(result.codes, []);
  for (const secret of secrets) assert.ok(!JSON.stringify(result).includes(secret));
  assert.ok(!JSON.stringify(result).includes('https://'));
});

test('missing permissions, malformed token responses and transport errors remain distinct', () => {
  for (const [line, stage] of [
    ['npm silly oidc Skipped because incorrect permissions for id-token within GitHub workflow', 'github-permissions-missing'],
    ['npm verbose oidc Failed to fetch id_token from GitHub: received an invalid response', 'github-token-request-rejected'],
    ['npm verbose oidc Failed to fetch id_token from GitHub: missing value', 'github-token-response-missing-value'],
    ['npm verbose oidc Failed because token exchange was missing the token in the response body', 'npm-token-response-missing-token'],
    ['npm verbose oidc Failure with message: secret', 'oidc-client-failure'],
  ]) assert.equal(observations(line, 0).stage, stage);
  assert.deepEqual(observations('npm error code ENOTFOUND', 1).codes, ['ENOTFOUND']);
  assert.deepEqual(observations('npm http fetch POST 200 https://unrelated.invalid/ 7ms', 0).exchangeStatuses, []);
});

test('native invocation always forces dry-run and disables scripts, disk logs and streamed output', () => {
  for (const version of ['11.19.1', '12.2.1']) {
    const calls = [];
    const result = diagnose(version, '/candidate', env, (command, args, options) => {
      calls.push({ command, args, options });
      return args[0] === '--version' ? { status: 0, stdout: `${version}\n` } :
        { status: 0, stdout: '+ @bitspark/horos@0.7.0', stderr: `${success}\n${notice}` };
    });
    assert.equal(result.stage, 'dry-run-authenticated');
    assert.equal(calls.length, 2);
    assert.equal(calls[1].command, 'npm');
    assert.deepEqual(calls[1].args, argumentsFor('/candidate'));
    for (const flag of ['--dry-run=true', '--ignore-scripts=true', '--logs-max=0']) assert.ok(calls[1].args.includes(flag));
    assert.deepEqual(calls[1].options.stdio, ['ignore', 'pipe', 'pipe']);
    assert.equal(calls[1].options.env, env);
    assert.ok(!JSON.stringify(result).includes('secret-request-token'));
  }
});

test('wrong contexts, reusable credentials, unexpected clients and process errors fail closed', () => {
  const forbidden = () => assert.fail('must not invoke npm');
  for (const change of [{ GITHUB_REF: 'refs/heads/other' }, { GITHUB_REPOSITORY: 'other/repo' },
    { GITHUB_WORKFLOW_REF: 'other' }, { RUNNER_ENVIRONMENT: 'self-hosted' }, { GITHUB_ACTIONS: '' }, { GITHUB_EVENT_NAME: 'pull_request' },
    { NPM_ID_TOKEN: 'secret' }, { NODE_AUTH_TOKEN: 'secret' }, { NPM_TOKEN: 'secret' }]) {
    assert.equal(diagnose('11.19.1', '/candidate', { ...env, ...change }, forbidden).stage, 'diagnostic-context-refused');
  }
  assert.throws(() => diagnose('latest', '/candidate', env, forbidden));
  assert.throws(() => argumentsFor(''));
  assert.equal(diagnose('11.19.1', '/candidate', env, () => ({ status: 0, stdout: 'other' })).stage, 'diagnostic-client-mismatch');
  const result = diagnose('11.19.1', '/candidate', env, (_, args) => args[0] === '--version' ?
    { status: 0, stdout: '11.19.1' } : { error: new Error('secret'), stderr: 'secret' });
  assert.equal(result.stage, 'diagnostic-process-failed');
  assert.ok(!JSON.stringify(result).includes('secret'));
});
