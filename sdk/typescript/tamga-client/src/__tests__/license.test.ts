// The package carries its own copy of the licence text so that the npm tarball
// ships it when this directory is packed on its own. This test keeps that copy
// identical to the LICENSE at the repository root.
import { describe, it, expect } from 'vitest';
import { existsSync, readFileSync } from 'node:fs';
import { dirname, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';

const pkgDir = resolve(dirname(fileURLToPath(import.meta.url)), '..', '..');
const pkgLicense = resolve(pkgDir, 'LICENSE');
const rootLicense = resolve(pkgDir, '..', '..', '..', 'LICENSE');

// Compare text, not bytes: a checkout may convert line endings.
const text = (p: string) => readFileSync(p, 'utf8').split('\r\n').join('\n');

describe('LICENSE', () => {
  it('is present in the package and listed in "files"', () => {
    expect(text(pkgLicense)).toContain('GNU AFFERO GENERAL PUBLIC LICENSE');
    const pkg = JSON.parse(readFileSync(resolve(pkgDir, 'package.json'), 'utf8'));
    expect(pkg.files).toContain('LICENSE');
  });

  it.skipIf(!existsSync(rootLicense))('matches the repository root LICENSE', () => {
    expect(text(pkgLicense)).toBe(text(rootLicense));
  });
});
