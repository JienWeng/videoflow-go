import test from 'node:test';
import assert from 'node:assert/strict';
import { serviceName } from './services.mjs';

test('video services are named the way the owner knows them', () => {
 assert.equal(serviceName('fal'), 'fal.ai');
 assert.equal(serviceName('kie'), 'Kie.ai');
 assert.equal(serviceName(''), 'The video service');
 assert.equal(serviceName('something-new'), 'The video service');
});
