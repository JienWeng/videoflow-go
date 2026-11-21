import assert from 'node:assert/strict';
import test from 'node:test';
import {uploadProblem,UPLOAD_LIMIT_BYTES} from './upload-limits.ts';
test('upload limit includes the multipart request overhead and combined files',()=>{
 assert.equal(uploadProblem([{name:'photo.png',size:1024}]),null);
 assert.match(uploadProblem([{name:'large.png',size:UPLOAD_LIMIT_BYTES}]),/50 MiB/);
 assert.match(uploadProblem([{name:'a.mp4',size:30*1024*1024},{name:'b.mp4',size:30*1024*1024}]),/50 MiB/);
});
test('character references explain supported image types without restricting general assets',()=>{
 assert.equal(uploadProblem([{name:'portrait.WEBP',size:1024}],true),null);
 assert.match(uploadProblem([{name:'photo.heic',size:1024}],true),/JPEG, PNG, GIF or WebP/);
 assert.equal(uploadProblem([{name:'voice.wav',size:1024}]),null);
});
