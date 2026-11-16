import assert from 'node:assert/strict';
import {changedFields,mergeDraft} from './drafts.mjs';
const old={title:'Old',summary:'Old summary'},draft={title:'My title',summary:'Old summary'},saved={title:'Old',summary:'New summary'};
assert.deepEqual(changedFields(draft,old),{title:'My title'});
assert.deepEqual(mergeDraft(saved,draft,old,['title','summary']),{title:'My title',summary:'New summary'});
assert.deepEqual(mergeDraft(saved,old,old,['title','summary']),saved);
assert.deepEqual(changedFields({duration:7,other:'x'},{duration:5},['duration']),{duration:7});
console.log('PASS 4 draft merge assertions');
