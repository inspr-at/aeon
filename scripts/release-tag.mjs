// SPDX-License-Identifier: AGPL-3.0-only
// A release tag is "v" plus one inspr-calendar-v2 coordinate (YYMMDDhhmmss.0.0)
// on a real proleptic Gregorian date. Writes version (without v) to GITHUB_OUTPUT.
import { appendFileSync } from "node:fs";
import { validCalendarVersion } from "./verify-release.mjs";

const tag = process.argv[2] || process.env.GITHUB_REF_NAME || "";
const version = tag.startsWith("v") ? tag.slice(1) : "";
if (tag !== `v${version}` || !validCalendarVersion(version)) {
  console.error(`tag ${JSON.stringify(tag)} is not an inspr-calendar-v2 version (v + YYMMDDhhmmss.0.0, real calendar date)`);
  process.exit(1);
}
if (process.env.GITHUB_OUTPUT) appendFileSync(process.env.GITHUB_OUTPUT, `version=${version}\n`);
console.log(version);
