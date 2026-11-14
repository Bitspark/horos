// Produce npm's signed bundle on the actual public GitHub runner. No upload.
const assert = require('node:assert/strict');
const { createHash } = require('node:crypto');
const { execFileSync } = require('node:child_process');
const fs = require('node:fs');
const path = require('node:path');
const { createRequire } = require('node:module');

function context(receipt, env) {
  assert.equal(env.GITHUB_REPOSITORY.toLowerCase(), 'bitspark/horos');
  assert.equal(env.GITHUB_REPOSITORY_ID, '1441363998');
  assert.equal(env.GITHUB_REF, 'refs/heads/main');
  assert.equal(env.GITHUB_EVENT_NAME, 'workflow_dispatch');
  assert.equal(env.RUNNER_ENVIRONMENT, 'github-hosted');
  assert.equal(env.GITHUB_WORKFLOW_REF,
    `${env.GITHUB_REPOSITORY}/.github/workflows/release.yml@refs/heads/main`);
  assert.equal(receipt.source, env.GITHUB_SHA);
  assert.equal(receipt.mode, 'rehearse');
  assert.equal(receipt.version, '0.11.1');
  return {
    repository: `https://github.com/${env.GITHUB_REPOSITORY}`,
    source: receipt.source,
    invocation: `https://github.com/${env.GITHUB_REPOSITORY}/actions/runs/${env.GITHUB_RUN_ID}/attempts/${env.GITHUB_RUN_ATTEMPT}`,
  };
}

function subject(archive) {
  return { name: 'pkg:npm/%40bitspark/horos@0.11.1',
    digest: { sha512: createHash('sha512').update(archive).digest('hex') } };
}

function statement(bundle, expectedSubject, expected) {
  assert.equal(bundle.dsseEnvelope.payloadType, 'application/vnd.in-toto+json');
  const value = JSON.parse(Buffer.from(bundle.dsseEnvelope.payload, 'base64').toString('utf8'));
  assert.equal(value._type, 'https://in-toto.io/Statement/v1');
  assert.equal(value.predicateType, 'https://slsa.dev/provenance/v1');
  assert.deepEqual(value.subject, [expectedSubject]);
  const build = value.predicate.buildDefinition;
  assert.equal(build.buildType, 'https://slsa-framework.github.io/github-actions-buildtypes/workflow/v1');
  assert.deepEqual(build.externalParameters.workflow, {
    ref: 'refs/heads/main', repository: expected.repository, path: '.github/workflows/release.yml',
  });
  assert.equal(build.internalParameters.github.repository_id, '1441363998');
  assert.deepEqual(build.resolvedDependencies, [{
    uri: `git+${expected.repository}@refs/heads/main`, digest: { gitCommit: expected.source },
  }]);
  assert.equal(value.predicate.runDetails.builder.id, 'https://github.com/actions/runner/github-hosted');
  assert.equal(value.predicate.runDetails.metadata.invocationId, expected.invocation);
  assert.ok(bundle.verificationMaterial.tlogEntries.length > 0, 'transparency entry required');
  return value;
}

async function main(directory) {
  const receipt = JSON.parse(fs.readFileSync(path.join(directory, 'candidate.json')));
  const expected = context(receipt, process.env);
  assert.equal(execFileSync('git', ['rev-parse', 'HEAD'], { encoding: 'utf8' }).trim(), receipt.source);
  const archive = fs.readFileSync(path.join(directory, 'bitspark-horos-0.11.1.tgz'));
  const pkg = receipt.packages.find(p => p.registry === 'npm');
  assert.equal(pkg.name, '@bitspark/horos');
  assert.equal(pkg.sha256, createHash('sha256').update(archive).digest('hex'));
  assert.equal(pkg.integrity, `sha512-${createHash('sha512').update(archive).digest('base64')}`);
  const npmRoot = path.join(execFileSync('npm', ['root', '--global'], { encoding: 'utf8' }).trim(), 'npm');
  assert.equal(JSON.parse(fs.readFileSync(path.join(npmRoot, 'package.json'))).version, '12.2.1');
  const npmRequire = createRequire(path.join(npmRoot, 'package.json'));
  const npa = npmRequire('npm-package-arg');
  const expectedSubject = subject(archive);
  assert.equal(npa.toPurl(npa.resolve(pkg.name, pkg.version)), expectedSubject.name);
  const { generateProvenance, verifyProvenance } = npmRequire('libnpmpublish/lib/provenance.js');
  const sigstore = npmRequire('sigstore');
  // npm supplies the GitHub OIDC integration and standard statement format.
  // Never replace GITHUB_* values or synthesize a local CI context.
  const bundle = await generateProvenance([expectedSubject], {});
  statement(bundle, expectedSubject, expected);
  await sigstore.verify(bundle, {
    certificateIdentityURI: `${expected.repository}/.github/workflows/release.yml@refs/heads/main`,
    certificateIssuer: 'https://token.actions.githubusercontent.com',
  });
  const file = path.join(directory, 'bitspark-horos-0.11.1.sigstore');
  fs.writeFileSync(file, JSON.stringify(bundle) + '\n', { flag: 'wx' });
  await verifyProvenance(expectedSubject, file);
  const result = { source: receipt.source, candidateRun: receipt.run_id,
    signingRun: expected.invocation, archiveSha256: pkg.sha256,
    bundleSha256: createHash('sha256').update(fs.readFileSync(file)).digest('hex'),
    certificateIdentity: `${expected.repository}/.github/workflows/release.yml@refs/heads/main`,
    npmVersion: '12.2.1', published: false };
  fs.writeFileSync(path.join(directory, 'provenance.json'), JSON.stringify(result, null, 2) + '\n', { flag: 'wx' });
  console.log(JSON.stringify(result));
}

module.exports = { context, subject, statement };
if (require.main === module) {
  main(process.argv[2]).catch(error => { console.error(error.message); process.exitCode = 1; });
}
