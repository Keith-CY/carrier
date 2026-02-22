const sanitizeProjectIdValue = (value) => String(value || '').trim().replace(/^["']|["']$/g, '');

const USER_OR_ORG_PROJECT_URL = /^https?:\/\/github\.com\/(users|orgs)\/([^/]+)\/projects\/(\d+)(?:[/?#].*)?$/i;
const REPO_PROJECT_URL = /^https?:\/\/github\.com\/([^/]+)\/([^/]+)\/projects\/(\d+)(?:[/?#].*)?$/i;

async function resolveProjectId(candidate, options = {}) {
  const raw = sanitizeProjectIdValue(candidate);
  const { graphql, log = () => {} } = options;

  if (!raw) return '';

  if (/^PVT_[A-Za-z0-9_]+$/.test(raw) || /^PD_[A-Za-z0-9_]+$/.test(raw)) {
    return raw;
  }

  if (typeof graphql !== 'function') {
    return raw;
  }

  const userOrOrg = raw.match(USER_OR_ORG_PROJECT_URL);
  if (userOrOrg) {
    const [, scope, login, number] = userOrOrg;
    const num = Number.parseInt(number, 10);
    if (!Number.isFinite(num)) return raw;
    try {
      const query = scope === 'users'
        ? 'query($login: String!, $number: Int!) { user(login: $login) { projectV2(number: $number) { id } } }'
        : 'query($login: String!, $number: Int!) { organization(login: $login) { projectV2(number: $number) { id } } }';
      const response = await graphql(query, { login, number: num });
      const node = scope === 'users' ? response?.user?.projectV2 : response?.organization?.projectV2;
      if (node?.id) return node.id;
    } catch (error) {
      log(`Could not resolve project URL "${raw}": ${error?.message || error}`);
    }
    return '';
  }

  const repoProject = raw.match(REPO_PROJECT_URL);
  if (repoProject) {
    const [, owner, repo, number] = repoProject;
    const num = Number.parseInt(number, 10);
    if (!Number.isFinite(num)) return raw;
    try {
      const response = await graphql(
        `query($owner: String!, $repo: String!, $number: Int!) {
          repository(owner: $owner, name: $repo) {
            projectV2(number: $number) { id }
          }
        }`,
        { owner, repo, number: num }
      );
      if (response?.repository?.projectV2?.id) return response.repository.projectV2.id;
    } catch (error) {
      log(`Could not resolve repo project URL "${raw}": ${error?.message || error}`);
    }
    return '';
  }

  return raw;
}

async function resolveProjectIdFromCandidates(candidates, options = {}) {
  for (const c of candidates) {
    const resolved = await resolveProjectId(c, options);
    if (resolved) return resolved;
  }
  return '';
}

module.exports = {
  sanitizeProjectIdValue,
  resolveProjectId,
  resolveProjectIdFromCandidates,
};
