/** Send only fields edited here; another view may have saved other fields.
 * @param {Record<string, any>} draft
 * @param {Record<string, any> | undefined} baseline
 * @param {readonly string[]} [fields]
 */
export function changedFields(draft, baseline, fields=Object.keys(draft)) {
 return Object.fromEntries(fields.filter(key=>draft[key]!==baseline?.[key]).map(key=>[key,draft[key]]));
}
/**
 * @param {Record<string, any>} saved
 * @param {Record<string, any> | undefined} draft
 * @param {Record<string, any> | undefined} baseline
 * @param {readonly string[]} fields
 */
export function mergeDraft(saved,draft,baseline,fields) {
 return draft?{...saved,...changedFields(draft,baseline,fields)}:saved;
}
