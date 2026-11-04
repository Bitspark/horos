// Exercise the unmodified npm publisher's OIDC path, always without uploading.
// Never forward native output: it can contain request URLs and provider secrets.
const { spawnSync } = require('node:child_process');
const { resolve } = require('node:path');

const versions = new Set(['11.19.1', '12.2.1']);
const registry = 'https://registry.npmjs.org/';
const exchange = 'https://registry.npmjs.org/-/npm/v1/oidc/token/exchange/package/@bitspark%2fhoros';

function argumentsFor(directory) {
  if (typeof directory !== 'string' || !directory) throw new Error('candidate-directory-required');
  return ['publish', resolve(directory, 'bitspark-horos-0.9.0.tgz'),
    '--dry-run=true', '--ignore-scripts=true', '--access=public', '--provenance=true',
    `--registry=${registry}`, '--loglevel=silly', '--logs-max=0', '--color=false',
    '--update-notifier=false'];
}

function observations(output, exitCode) {
  const lines = String(output).split(/\r?\n/);
  const has = pattern => lines.some(line => pattern.test(line));
  // Only numeric statuses escape these matches. No URL or response body does.
  const githubStatuses = [];
  const exchangeStatuses = [];
  for (const line of lines) {
    const github = line.match(/^npm http fetch GET https:\/\/\S+ [1-5]\d\d \d+ms$/);
    if (github) githubStatuses.push(Number(line.match(/ ([1-5]\d\d) \d+ms$/)[1]));
    const response = line.match(/^npm http fetch POST ([1-5]\d\d) (\S+)(?: |$)/);
    if (response && response[2].toLowerCase() === exchange) exchangeStatuses.push(Number(response[1]));
  }
  const exchanged = has(/^npm verbose oidc Successfully retrieved and set token$/);
  const missingCredentials = has(/^npm warn publish This command requires you to be logged in to https:\/\/registry\.npmjs\.org\/ \(dry-run\)$/);
  const completed = has(/^npm notice Publishing to https:\/\/registry\.npmjs\.org\/ with tag latest and public access \(dry-run\)$/);
  const knownCodes = new Set(['ENEEDAUTH', 'E401', 'E403', 'E404', 'E429', 'E500', 'E502', 'E503', 'ECONNRESET', 'ENOTFOUND', 'ETIMEDOUT']);
  const codes = lines.map(line => line.match(/^npm error code ([A-Z0-9]+)$/)?.[1]).filter(code => knownCodes.has(code));
  let stage = 'oidc-not-observed';
  if (has(/^npm silly oidc Skipped because incorrect permissions for id-token within GitHub workflow$/)) stage = 'github-permissions-missing';
  else if (has(/^npm verbose oidc Failed to fetch id_token from GitHub: received an invalid response$/)) stage = 'github-token-request-rejected';
  else if (has(/^npm verbose oidc Failed to fetch id_token from GitHub: missing value$/)) stage = 'github-token-response-missing-value';
  else if (has(/^npm silly oidc Skipped because no id_token available$/)) stage = 'identity-token-missing';
  else if (has(/^npm verbose oidc Failed token exchange request with body message:/)) stage = 'npm-token-exchange-rejected';
  else if (has(/^npm verbose oidc Failed because token exchange was missing the token in the response body$/)) stage = 'npm-token-response-missing-token';
  else if (exchanged && missingCredentials) stage = 'credential-lookup-missing-after-exchange';
  else if (exchanged && completed && exitCode === 0) stage = 'dry-run-authenticated';
  else if (exchanged) stage = 'exchange-succeeded-dry-run-incomplete';
  else if (has(/^npm verbose oidc Failure with message:/)) stage = 'oidc-client-failure';
  return { stage, githubStatuses, exchangeStatuses, exchanged, missingCredentials,
    completed, codes: [...new Set(codes)], exitCode: Number.isInteger(exitCode) ? exitCode : null };
}

function diagnose(version, directory, env = process.env, run = spawnSync) {
  if (!versions.has(version)) throw new Error('unsupported-diagnostic-client');
  const presence = Object.fromEntries(['ACTIONS_ID_TOKEN_REQUEST_URL', 'ACTIONS_ID_TOKEN_REQUEST_TOKEN',
    'NPM_ID_TOKEN', 'NODE_AUTH_TOKEN', 'NPM_TOKEN', 'NPM_CONFIG_USERCONFIG'].map(key => [key, Boolean(env[key])]));
  const base = { npm: version, dryRun: true, presence };
  if (env.GITHUB_ACTIONS !== 'true' || env.GITHUB_EVENT_NAME !== 'workflow_dispatch' || env.RUNNER_ENVIRONMENT !== 'github-hosted' ||
      env.GITHUB_REPOSITORY !== 'Bitspark/horos' || env.GITHUB_REF !== 'refs/heads/main' ||
      env.GITHUB_WORKFLOW_REF !== 'Bitspark/horos/.github/workflows/release.yml@refs/heads/main' ||
      presence.NPM_ID_TOKEN || presence.NODE_AUTH_TOKEN || presence.NPM_TOKEN) {
    return { ...base, stage: 'diagnostic-context-refused' };
  }
  const options = { env, encoding: 'utf8', stdio: ['ignore', 'pipe', 'pipe'],
    timeout: 120000, maxBuffer: 2 * 1024 * 1024, windowsHide: true };
  const client = run('npm', ['--version'], options);
  if (client.error || client.status !== 0 || client.stdout.trim() !== version) {
    return { ...base, stage: 'diagnostic-client-mismatch' };
  }
  const result = run('npm', argumentsFor(directory), options);
  if (result.error) return { ...base, stage: 'diagnostic-process-failed' };
  return { ...base, ...observations(`${result.stdout}\n${result.stderr}`, result.status) };
}

if (require.main === module) {
  try {
    if (process.argv.length !== 3) throw new Error('invalid-arguments');
    // Even a failed authentication is a diagnostic result, not release success.
    console.log(JSON.stringify(diagnose(process.argv[2], process.env.CANDIDATE)));
  } catch {
    console.log(JSON.stringify({ stage: 'diagnostic-harness-failed', dryRun: true }));
    process.exitCode = 1;
  }
}

module.exports = { argumentsFor, observations, diagnose };
