import test from 'node:test';
import assert from 'node:assert/strict';
import { serviceName } from './services.mjs';

test('video services are named the way the owner knows them', () => {
 assert.equal(serviceName('fal'), 'fal.ai');
 assert.equal(serviceName('argolink'), 'ArgoLink');
 assert.equal(serviceName(''), 'The video service');
 assert.equal(serviceName('something-new'), 'The video service');
});

test('old videos on a service the app no longer uses keep its name', () => {
 assert.equal(serviceName('kie'), 'Kie.ai');
 assert.equal(serviceName('runway'), 'Runway');
 assert.equal(serviceName('replicate'), 'Replicate');
 assert.equal(serviceName('wavespeed'), 'WaveSpeed');
 assert.equal(serviceName('loopling'), 'Loopling');
});
