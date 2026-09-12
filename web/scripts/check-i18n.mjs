import fs from 'node:fs';
import assert from 'node:assert/strict';
const zh=JSON.parse(fs.readFileSync(new URL('../src/locales/zh.json',import.meta.url),'utf8'));
const en=JSON.parse(fs.readFileSync(new URL('../src/locales/en.json',import.meta.url),'utf8'));
const flatten=(obj,prefix='')=>Object.entries(obj).flatMap(([key,value])=>typeof value==='string'?[[prefix+key,value]]:flatten(value,prefix+key+'.'));
const a=new Map(flatten(zh)),b=new Map(flatten(en));assert.deepEqual([...a.keys()].sort(),[...b.keys()].sort());
for(const [key,value] of a){const tokens=v=>[...new Set([...v.matchAll(/\{(\w+)\}/g)].map(m=>m[1]))].sort();assert.deepEqual(tokens(value),tokens(b.get(key)),key+' interpolation mismatch');assert.ok(value.trim()&&b.get(key).trim(),key+' empty translation');}
console.log(`Validated ${a.size} bilingual keys and interpolation variables.`);
