import assert from 'node:assert/strict';
import test from 'node:test';
import {errorMessage,friendlyError} from './errors.ts';

test('errorMessage reads Error messages and strings, else falls back',()=>{
 assert.equal(errorMessage(new Error('boom')),'boom');
 assert.equal(errorMessage('plain'),'plain');
 assert.equal(errorMessage({message:'shaped'}),'shaped');
 assert.equal(errorMessage(null,'fallback'),'fallback');
 assert.equal(errorMessage(new Error(''),'fallback'),'fallback');
 assert.equal(errorMessage(42,'fallback'),'fallback');
});

test('friendlyError softens known provider failures',()=>{
 assert.equal(friendlyError('request timed out'),'The provider took too long to respond — try again.');
 assert.equal(friendlyError(new Error('429: Too Many Requests')),'Rate limited by the provider — wait a moment and retry.');
 assert.equal(friendlyError('Insufficient balance'),'The provider account is out of credit/quota.');
 assert.equal(friendlyError('401: bad api key'),'Provider rejected the API key — check Settings.');
 assert.equal(friendlyError('content policy violation'),'The prompt was blocked by the provider content filter.');
 assert.equal(friendlyError('ECONNREFUSED'),'Network error reaching the provider — check the connection.');
});

test('friendlyError keeps unknown text, capped, and uses the fallback when empty',()=>{
 assert.equal(friendlyError('  odd failure  '),'odd failure');
 assert.equal(friendlyError('x'.repeat(200)),'x'.repeat(157)+'…');
 assert.equal(friendlyError(null),'Something went wrong (no detail provided).');
 assert.equal(friendlyError('',"Couldn't render"),"Couldn't render");
});
