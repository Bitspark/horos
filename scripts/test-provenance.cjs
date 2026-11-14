const assert = require('node:assert/strict');
const { test } = require('node:test');
const { context, subject, statement } = require('./provenance.cjs');

function fixture() {
  const receipt = { source: 'a'.repeat(40), mode: 'rehearse', version: '0.11.1' };
  const env = { GITHUB_REPOSITORY: 'Bitspark/horos', GITHUB_REPOSITORY_ID: '1441363998',
    GITHUB_REF: 'refs/heads/main', GITHUB_EVENT_NAME: 'workflow_dispatch',
    GITHUB_SHA: receipt.source, RUNNER_ENVIRONMENT: 'github-hosted',
    GITHUB_WORKFLOW_REF: 'Bitspark/horos/.github/workflows/release.yml@refs/heads/main',
    GITHUB_RUN_ID: '1', GITHUB_RUN_ATTEMPT: '1' };
  const expected = context(receipt, env);
  const archiveSubject = subject(Buffer.from('candidate bytes'));
  const value = { _type: 'https://in-toto.io/Statement/v1', subject: [archiveSubject],
    predicateType: 'https://slsa.dev/provenance/v1', predicate: {
      buildDefinition: { buildType: 'https://slsa-framework.github.io/github-actions-buildtypes/workflow/v1',
        externalParameters: { workflow: { ref: 'refs/heads/main', repository: expected.repository,
          path: '.github/workflows/release.yml' } }, internalParameters: { github: { repository_id: '1441363998' } },
        resolvedDependencies: [{ uri: `git+${expected.repository}@refs/heads/main`, digest: { gitCommit: receipt.source } }] },
      runDetails: { builder: { id: 'https://github.com/actions/runner/github-hosted' },
        metadata: { invocationId: expected.invocation } } } };
  const bundle = () => ({ dsseEnvelope: { payloadType: 'application/vnd.in-toto+json',
    payload: Buffer.from(JSON.stringify(value)).toString('base64') }, verificationMaterial: { tlogEntries: [{}] } });
  return { receipt, env, expected, archiveSubject, value, bundle };
}

test('matching statement and actual source context are accepted', () => {
  const f = fixture();
  statement(f.bundle(), f.archiveSubject, f.expected);
});

test('another repository, workflow, source, runner or event cannot be attested', () => {
  for (const [key, value] of Object.entries({ GITHUB_REPOSITORY: 'Bitspark/horos-next',
    GITHUB_REPOSITORY_ID: '1261489082', GITHUB_SHA: 'b'.repeat(40), GITHUB_REF: 'refs/heads/other',
    GITHUB_WORKFLOW_REF: 'Bitspark/horos/.github/workflows/other.yml@refs/heads/main',
    RUNNER_ENVIRONMENT: 'self-hosted', GITHUB_EVENT_NAME: 'pull_request' })) {
    const f = fixture();
    assert.throws(() => context(f.receipt, { ...f.env, [key]: value }), key);
  }
});

test('changed archive, extra subjects and changed provenance claims are refused', () => {
  const mutations = [
    f => { f.archiveSubject = subject(Buffer.from('different bytes')); },
    f => f.value.subject.push(f.archiveSubject),
    f => { f.value.subject[0].name = 'pkg:npm/different@0.11.1'; f.archiveSubject = subject(Buffer.from('candidate bytes')); },
    f => { f.value.predicate.buildDefinition.resolvedDependencies[0].digest.gitCommit = 'b'.repeat(40); },
    f => { f.value.predicate.buildDefinition.externalParameters.workflow.repository = 'https://github.com/other/repo'; },
    f => { f.value.predicate.runDetails.metadata.invocationId += '0'; },
    f => { f.value.predicate.runDetails.builder.id = 'local'; },
  ];
  for (const mutate of mutations) {
    const f = fixture(); mutate(f);
    assert.throws(() => statement(f.bundle(), f.archiveSubject, f.expected));
  }
  const f = fixture(); const bundle = f.bundle(); bundle.verificationMaterial.tlogEntries = [];
  assert.throws(() => statement(bundle, f.archiveSubject, f.expected));
});
