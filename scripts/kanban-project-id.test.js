const assert = require('node:assert/strict');
const { resolveProjectId, sanitizeProjectIdValue } = require('../.github/scripts/kanban-project-id');

async function main() {
  // sanitizeProjectIdValue edge cases
  assert.equal(sanitizeProjectIdValue(undefined), '');
  assert.equal(sanitizeProjectIdValue(''), '');
  assert.equal(sanitizeProjectIdValue('  PVT_abc123  '), 'PVT_abc123');
  assert.equal(sanitizeProjectIdValue('"PVT_quoted"'), 'PVT_quoted');
  assert.equal(sanitizeProjectIdValue("'PD_quoted'"), 'PD_quoted');

  // canonical IDs should pass through unchanged
  assert.equal(await resolveProjectId('PVT_kwDOabc123_456'), 'PVT_kwDOabc123_456');
  assert.equal(await resolveProjectId('PD_kwDOabc123_456'), 'PD_kwDOabc123_456');

  // unknown non-empty values pass through without graphql
  assert.equal(await resolveProjectId('custom-value'), 'custom-value');

  // user project URL resolves to canonical ID
  {
    const calls = [];
    const graphql = async (_query, vars) => {
      calls.push(vars);
      return { user: { projectV2: { id: 'PVT_user_123' } } };
    };
    const id = await resolveProjectId('https://github.com/users/octocat/projects/12', { graphql });
    assert.equal(id, 'PVT_user_123');
    assert.deepEqual(calls, [{ login: 'octocat', number: 12 }]);
  }

  // org project URL resolves to canonical ID
  {
    const graphql = async (_query, vars) => {
      assert.deepEqual(vars, { login: 'openclaw', number: 7 });
      return { organization: { projectV2: { id: 'PVT_org_007' } } };
    };
    const id = await resolveProjectId('https://github.com/orgs/openclaw/projects/7', { graphql });
    assert.equal(id, 'PVT_org_007');
  }

  // repo project URL resolves to canonical ID
  {
    const graphql = async (_query, vars) => {
      assert.deepEqual(vars, { owner: 'Keith-CY', repo: 'carrier', number: 18 });
      return { repository: { projectV2: { id: 'PVT_repo_018' } } };
    };
    const id = await resolveProjectId('https://github.com/Keith-CY/carrier/projects/18', { graphql });
    assert.equal(id, 'PVT_repo_018');
  }

  // resolution failures return empty string (so candidate fallbacks can continue)
  {
    const graphql = async () => {
      throw new Error('boom');
    };
    const id = await resolveProjectId('https://github.com/orgs/openclaw/projects/999', { graphql, log: () => {} });
    assert.equal(id, '');
  }

  console.log('kanban-project-id: all assertions passed');
}

main().catch((err) => {
  console.error(err);
  process.exit(1);
});
