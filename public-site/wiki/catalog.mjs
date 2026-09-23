// The renderer accepts only this public projection, never arbitrary repository files.
export function projectCatalog(input) {
  if (input?.schema_version !== 1 || !Array.isArray(input.capabilities) || input.capabilities.length > 200) throw new Error('Unsupported catalog');
  const text = value => {
    if (typeof value !== 'string' || !value.trim() || value.length > 4000) throw new Error('Invalid catalog text');
    return value;
  };
  const list = value => {
    if (!Array.isArray(value) || value.length > 50) throw new Error('Invalid catalog list');
    return value.map(text);
  };
  const link = value => {
    const parsed = new URL(text(value));
    if (parsed.protocol !== 'https:' || parsed.username || parsed.password) throw new Error('Catalog links must use HTTPS without credentials');
    return parsed.href;
  };
  const statuses = ['DISCOVERED','CANDIDATE','EXPERIMENTAL','STABLE','DEPRECATED','REJECTED'];
  const dispositions = ['INVESTIGATE','EXTRACT','REFERENCE_EXISTING','REJECT'];
  const domains = ['identity','billing','infrastructure','delivery','protocol'];
  const seen = new Set();
  return input.capabilities.map(record => {
    const id = text(record.id);
    if (!/^[a-z]+\.[a-z][a-z0-9-]*$/.test(id) || seen.has(id) || !domains.includes(record.domain) || !id.startsWith(record.domain + '.')) throw new Error('Invalid or duplicate capability ID');
    if (!statuses.includes(record.status) || !dispositions.includes(record.disposition)) throw new Error('Invalid lifecycle');
    seen.add(id);
    for (const field of ['alternatives','implementations','verification','evidence']) if (!Array.isArray(record[field]) || record[field].length > 50) throw new Error('Invalid catalog records');
    const alternatives = record.alternatives.map(value => ({name:text(value.name),url:link(value.url),assessment:text(value.assessment)}));
    const implementations = record.implementations.map(value => ({url:link(value.url),revision:text(value.revision)}));
    const verification = record.verification.map(value => {
      if (!['CONTRACT_TEST','SECURITY_TEST','REAL_CONSUMER','HUMAN_REVIEW'].includes(value.kind) || !['PASS','FAIL','NOT_RUN'].includes(value.result)) throw new Error('Invalid verification');
      return {kind:value.kind,result:value.result,evidence_url:link(value.evidence_url)};
    });
    if (record.evidence.some(value => !['PUBLIC','RESTRICTED'].includes(value.visibility))) throw new Error('Unknown evidence visibility');
    // Restricted evidence bodies/URLs are deliberately not projected or bundled.
    const publicEvidence = record.evidence.filter(value => value.visibility === 'PUBLIC').map(value => ({url:link(value.url),basis:text(value.basis),verification:text(value.verification)}));
    return {id,domain:record.domain,name:text(record.name),status:record.status,disposition:record.disposition,summary:text(record.summary),use_when:list(record.use_when),non_goals:list(record.non_goals),proposed_guarantees:list(record.proposed_guarantees),failure_modes:list(record.failure_modes),open_questions:list(record.open_questions),alternatives,implementations,verification,publicEvidence,restrictedEvidence:record.evidence.some(value => value.visibility === 'RESTRICTED')};
  }).sort((a,b) => a.id.localeCompare(b.id,'en'));
}
