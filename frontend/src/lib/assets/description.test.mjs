import {test} from 'node:test';import assert from 'node:assert/strict';import {parseDescription,formatDescription} from './description.mjs';
const raw=`Storyboard keyframe of Pawprints to Mama. Pip follows the pawprints.
A lost stray puppy finds his mother.
Visual style: warm 2D picture book
Spoken language: English
Mode: dialogue
Direction:

Character Pip (char_pip): A lost stray puppy.. Appearance: Golden puppy with floppy ears.. Personality: Determined.. Visual rules: []. Voice rules: []. Sample dialogue: .
Character Mama (char_mama): Pip's mother.. Appearance: Adult golden dog.. Personality: Loving.. Visual rules: []. Voice rules: []. Sample dialogue: .`;
test('parses the reported description into narrative, settings and character fields',()=>{const d=parseDescription(raw);assert.match(d.summary,/Pip follows/);assert(!d.summary.includes('Visual style:'));assert.equal(d.style,'warm 2D picture book');assert.equal(d.language,'English');assert.equal(d.mode,'dialogue');assert.equal(d.direction,'');assert.equal(d.characters.length,2);assert.equal(d.characters[0].id,'char_pip');assert.equal(d.characters[0].appearance,'Golden puppy with floppy ears.');assert.equal(d.characters[0].visual_rules,'');assert.equal(d.characters[0].sample_dialogue,'')});
test('canonical formatting round trips all meaningful fields',()=>{const d=parseDescription(raw);d.direction='Keep the camera at puppy eye level.';d.characters[0].voice_rules='Soft voice; short lines.';assert.deepEqual(parseDescription(formatDescription(d)),d)});
test('plain imported descriptions and unknown content remain readable',()=>{const raw='Red collar with a brass buckle.\nKeep the worn leather texture.';assert.equal(parseDescription(raw).summary,raw);assert.equal(formatDescription(parseDescription(raw)),raw)});
test('multiline fields and character descriptions are preserved',()=>{const d=parseDescription('A bridge.\nVisual style: watercolour\nSoft paper texture.\nCharacter Pip (char_pip):\nDescription: A puppy.\nAppearance: Golden fur.\nWhite paws.\nSample dialogue: Mama!');assert.equal(d.style,'watercolour\nSoft paper texture.');assert.equal(d.characters[0].appearance,'Golden fur.\nWhite paws.');assert.deepEqual(parseDescription(formatDescription(d)),d)});

test('meaningful dialogue ellipses and nonempty rules survive formatting',()=>{const d=parseDescription('A reunion.\nCharacter Pip (pip): A puppy. Appearance: Golden fur. Visual rules: ["floppy ears"]. Sample dialogue: Mama...');assert.equal(d.characters[0].sample_dialogue,'Mama...');assert.match(d.characters[0].visual_rules,/floppy ears/);assert.deepEqual(parseDescription(formatDescription(d)),d)});
