#!/usr/bin/env node
import { createHash } from 'node:crypto';
import { readFileSync, writeFileSync } from 'node:fs';
import { resolve } from 'node:path';

const SOURCE = 'https://www8.cao.go.jp/chosei/shukujitsu/syukujitsu.csv';
const OUTPUT = resolve('engine/japan-settlement-holidays.json');
const BASE_YEAR = 2021;
// Each accepted input was manually compared with the Cabinet Office's
// published year list. Pinning the full CSV rejects truncation and changed
// source bytes; review and add a new hash when the next year is published.
const REVIEWED_CSV_SHA256 = new Map([
  [2027, 'cec37a743c96995cdb9cb52b685c9003634682a9b0e1a640a6b9b96881fe964a'],
]);

function fail(message) { throw new Error(message); }

function argumentsFrom(argv) {
  const args = {};
  for (let index = 0; index < argv.length; index += 1) {
    const key = argv[index];
    if (!['--csv', '--through-year', '--retrieved-on', '--write', '--check'].includes(key)) {
      fail(`Unknown argument: ${key}`);
    }
    if (key === '--write' || key === '--check') {
      if (args.mode) fail('Choose exactly one of --write or --check.');
      args.mode = key.slice(2);
    } else {
      if (args[key]) fail(`Duplicate argument: ${key}`);
      const value = argv[++index];
      if (!value || value.startsWith('--')) fail(`Missing value for ${key}.`);
      args[key] = value;
    }
  }
  const year = Number(args['--through-year']);
  if (!args['--csv'] || !Number.isInteger(year) || year < BASE_YEAR || year > 9999 || !args.mode) {
    fail('Usage: node scripts/update-japan-settlement-holidays.mjs --csv FILE --through-year YYYY --check | --write [--retrieved-on YYYY-MM-DD]');
  }
  if (args['--retrieved-on'] && !/^\d{4}-\d{2}-\d{2}$/.test(args['--retrieved-on'])) {
    fail('--retrieved-on must use YYYY-MM-DD.');
  }
  if (args.mode === 'write' && !args['--retrieved-on']) fail('--write requires --retrieved-on YYYY-MM-DD.');
  return { csvPath: resolve(args['--csv']), throughYear: year, retrievedOn: args['--retrieved-on'], mode: args.mode };
}

function parseCsv(bytes) {
  // Cabinet Office names are Shift-JIS; the ASCII date column is sufficient.
  const lines = bytes.toString('latin1').split(/\r?\n/);
  if (!lines[0]?.includes(',')) fail('Holiday CSV header is missing.');
  const dates = [];
  for (const line of lines.slice(1)) {
    if (!line) continue;
    const match = /^(\d{4})\/(\d{1,2})\/(\d{1,2}),/.exec(line);
    if (!match) fail('Holiday CSV contains a malformed date row.');
    const [, year, month, day] = match;
    const date = `${year}-${month.padStart(2, '0')}-${day.padStart(2, '0')}`;
    const parsed = new Date(`${date}T00:00:00.000Z`);
    if (!Number.isFinite(parsed.getTime()) || parsed.toISOString().slice(0, 10) !== date) fail('Holiday CSV contains an invalid date.');
    dates.push(date);
  }
  for (let index = 1; index < dates.length; index += 1) {
    if (dates[index] <= dates[index - 1]) fail('Holiday CSV dates must be unique and sorted.');
  }
  return dates;
}

function assertReviewedSource(bytes, throughYear) {
  const expectedHash = REVIEWED_CSV_SHA256.get(throughYear);
  if (!expectedHash) fail(`No manually reviewed Cabinet Office CSV is pinned for coverage through ${throughYear}.`);
  const actualHash = createHash('sha256').update(bytes).digest('hex');
  if (actualHash !== expectedHash) fail(`Cabinet Office CSV bytes do not match the reviewed ${throughYear} source hash.`);
}

function assertPublishedYears(dates, throughYear) {
  for (let year = BASE_YEAR; year <= throughYear; year += 1) {
    const rows = dates.filter(date => date.startsWith(`${year}-`));
    if (rows.length < 16 || rows[0] !== `${year}-01-01`) {
      fail(`Official CSV does not contain a plausible complete holiday list for ${year}.`);
    }
  }
}

function expectedCalendar(bytes, current, throughYear, retrievedOn) {
  assertReviewedSource(bytes, throughYear);
  const sourceDates = parseCsv(bytes);
  assertPublishedYears(sourceDates, throughYear);
  const currentDates = current.dates.filter(date => date >= `${BASE_YEAR}-01-01`);
  const existingThroughYear = Math.max(...currentDates.map(date => Number(date.slice(0, 4))));
  if (throughYear < existingThroughYear) fail(`Refusing to reduce existing coverage from ${existingThroughYear}.`);
  const priorYears = [...new Set(currentDates.map(date => Number(date.slice(0, 4))))].filter(year => year <= throughYear);
  for (const year of priorYears) {
    const oldYear = currentDates.filter(date => date.startsWith(`${year}-`));
    const sourceYear = sourceDates.filter(date => date.startsWith(`${year}-`));
    if (JSON.stringify(oldYear) !== JSON.stringify(sourceYear)) fail(`Published calendar dates for ${year} differ from the existing embedded list.`);
  }
  const dates = sourceDates.filter(date => date >= `${BASE_YEAR}-01-01` && date <= `${throughYear}-12-31`);
  return {
    source: SOURCE,
    sourceSha256: createHash('sha256').update(bytes).digest('hex'),
    retrievedOn: retrievedOn || current.retrievedOn,
    dates,
  };
}

try {
  const args = argumentsFrom(process.argv.slice(2));
  const bytes = readFileSync(args.csvPath);
  const current = JSON.parse(readFileSync(OUTPUT, 'utf8'));
  const expected = expectedCalendar(bytes, current, args.throughYear, args.retrievedOn);
  if (args.mode === 'check') {
    if (JSON.stringify(current) !== JSON.stringify(expected)) fail('Embedded Japan settlement calendar differs from the official CSV.');
    console.log(`Japan settlement calendar matches official CSV through ${args.throughYear}-12-31 (${expected.dates.length} dates; sha256 ${expected.sourceSha256}).`);
  } else {
    writeFileSync(OUTPUT, `${JSON.stringify(expected, null, 2)}\n`);
    console.log(`Wrote ${OUTPUT} through ${args.throughYear}-12-31 (${expected.dates.length} dates; sha256 ${expected.sourceSha256}).`);
  }
} catch (error) {
  console.error(`Japan settlement calendar: ${error.message}`);
  process.exitCode = 1;
}
