#!/usr/bin/env python3
"""Check the linked CLI dependency policy and local-only runtime contracts."""
import argparse
import json
import os
from pathlib import Path
import subprocess
import sys

ROOT = Path(__file__).resolve().parents[1]


def main():
    args = argparse.ArgumentParser(description=__doc__)
    args.add_argument('--dependencies-only', action='store_true')
    options = args.parse_args()
    policy = json.loads((ROOT / 'x/fork-policy.json').read_text())
    result = subprocess.run(
        ['go', 'list', '-tags', 'fts5', '-deps', './cmd/agentsview'],
        cwd=ROOT, check=True, text=True, capture_output=True,
    )
    packages = result.stdout.splitlines()
    violations = sorted({p for p in packages for prefix in policy['forbidden_import_prefixes']
                         if p == prefix or p.startswith(prefix + '/')})
    if violations:
        print('Forbidden runtime dependencies:\n' + '\n'.join(violations), file=sys.stderr)
        return 1
    print(f'Dependency policy passed for {len(packages)} linked packages.', flush=True)
    if not options.dependencies_only:
        env = dict(os.environ, TMPDIR='/tmp')
        for check in policy['behavior_checks']:
            # Fail if a renamed/deleted regression would otherwise silently pass.
            listing = subprocess.run(
                ['go', 'test', '-tags', 'fts5', check['package'], '-list', check['pattern']],
                cwd=ROOT, env=env, check=True, text=True, capture_output=True,
            )
            if not any(line.startswith('Test') for line in listing.stdout.splitlines()):
                raise RuntimeError('No tests matched: ' + str(check))
            subprocess.run(
                ['go', 'test', '-p', '1', '-count=1', '-tags', 'fts5',
                 check['package'], '-run', check['pattern']],
                cwd=ROOT, env=env, check=True,
            )
    return 0


if __name__ == '__main__':
    try:
        sys.exit(main())
    except (subprocess.CalledProcessError, RuntimeError) as error:
        print(str(error), file=sys.stderr)
        if isinstance(error, subprocess.CalledProcessError) and error.stderr:
            print(error.stderr, file=sys.stderr)
        sys.exit(1)
