#!/usr/bin/env python3
"""Release only artifacts from a successful build of the current clean commit."""
import argparse
import hashlib
import json
from pathlib import Path
import re
import subprocess
import tempfile

ROOT = Path(__file__).resolve().parent.parent
REPO = 'jeanlyn/mygo-editor'
TARGETS = {'windows-amd64': '.zip', 'macos-arm64': '.tar.gz', 'macos-amd64': '.tar.gz'}


def run(*args):
    return subprocess.check_output(args, cwd=ROOT, text=True).strip()


def api(path):
    return json.loads(run('gh', 'api', path))


def validate_run(data, sha):
    if data['head_sha'] != sha or data['head_branch'] != 'main':
        raise ValueError('Build does not match the current main commit')
    if data['status'] != 'completed' or data['conclusion'] != 'success':
        raise ValueError('Build must have completed successfully')
    if data['path'] != '.github/workflows/build.yml':
        raise ValueError('Unexpected workflow')


def collect_assets(directory):
    assets, lines = [], []
    for target, ext in TARGETS.items():
        folder = directory / f'mygo-editor-{target}'
        name = f'mygo-editor-{target}{ext}'
        archive = folder / name
        expected = (folder / 'SHA256SUMS.txt').read_text().strip().split()
        if len(expected) != 2 or expected[1] != name or not re.fullmatch(r'[0-9a-f]{64}', expected[0]):
            raise ValueError(f'Invalid checksum manifest for {target}')
        actual = hashlib.sha256(archive.read_bytes()).hexdigest()
        if actual != expected[0]:
            raise ValueError(f'Checksum mismatch: {name}')
        assets.append(archive)
        lines.append(f'{actual}  {name}\n')
    sums = directory / 'SHA256SUMS.txt'
    sums.write_text(''.join(lines), encoding='utf-8')
    return assets + [sums]


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('version', help='New version, e.g. v0.1.1')
    parser.add_argument('--run-id', required=True, type=int)
    parser.add_argument('--notes', required=True, type=Path)
    parser.add_argument('--stable', action='store_true', help='Publish as stable/latest; default is prerelease')
    parser.add_argument('--check-only', action='store_true', help='Verify and download without creating a tag or release')
    args = parser.parse_args()
    if not re.fullmatch(r'v[0-9]+\.[0-9]+\.[0-9]+(?:-[A-Za-z0-9.-]+)?', args.version):
        parser.error('Use a version such as v0.1.0')
    if not args.notes.is_file():
        parser.error('Release notes file is missing')
    if run('git', 'status', '--porcelain'):
        raise SystemExit('Commit or stash changes before releasing')
    sha = run('git', 'rev-parse', 'HEAD')
    if api(f'repos/{REPO}/git/ref/heads/main')['object']['sha'] != sha:
        raise SystemExit('Push main before releasing; remote HEAD differs')
    # Refuse existing tags/releases; never move a version or overwrite assets.
    tags = json.loads(run('gh', 'api', '--paginate', '--slurp', f'repos/{REPO}/tags?per_page=100'))
    releases = json.loads(run('gh', 'api', '--paginate', '--slurp', f'repos/{REPO}/releases?per_page=100'))
    if any(x['name'] == args.version for page in tags for x in page) or any(x['tag_name'] == args.version for page in releases for x in page):
        raise SystemExit('Version already exists; inspect it or choose a new version')
    validate_run(api(f'repos/{REPO}/actions/runs/{args.run_id}'), sha)
    jobs = api(f'repos/{REPO}/actions/runs/{args.run_id}/jobs?per_page=100')['jobs']
    if {j['name'] for j in jobs} != set(TARGETS) or any(j['conclusion'] != 'success' for j in jobs):
        raise SystemExit('All three platform jobs must succeed')
    with tempfile.TemporaryDirectory(prefix='mygo-release-') as temp:
        directory = Path(temp)
        run('gh', 'run', 'download', str(args.run_id), '--repo', REPO, '--dir', temp)
        assets = collect_assets(directory)
        print(f'Verified {sha}: all three packages and checksums')
        if args.check_only:
            return
        notes = directory / 'release-notes.md'
        notes.write_text(args.notes.read_text() + f'\n\nCommit: `{sha}`\nBuild: https://github.com/{REPO}/actions/runs/{args.run_id}\n', encoding='utf-8')
        # Create the exact tag atomically; never reuse a concurrent tag.
        run('gh', 'api', f'repos/{REPO}/git/refs', '-f', f'ref=refs/tags/{args.version}', '-f', f'sha={sha}')
        command = ['gh', 'release', 'create', args.version, '--repo', REPO, '--verify-tag', '--target', sha,
                   '--title', f'MyGo Editor {args.version}', '--notes-file', str(notes), '--draft']
        if not args.stable:
            command += ['--prerelease']
        command += [str(p) for p in assets]
        # On partial failure leave the draft for inspection; never retry blindly.
        print(run(*command))
        pages = json.loads(run('gh', 'api', '--paginate', '--slurp', f'repos/{REPO}/releases?per_page=100'))
        matches = [r for page in pages for r in page if r['tag_name'] == args.version]
        if len(matches) != 1:
            raise SystemExit('Could not identify draft; inspect before publishing')
        release_id = matches[0]['id']
        release = api(f'repos/{REPO}/releases/{release_id}')
        if not release['draft'] or release['target_commitish'] != sha:
            raise SystemExit('Draft target mismatch; inspect before publishing')
        uploaded = {a['name']: (a['size'], a.get('digest'), a['state']) for a in release['assets']}
        expected = {p.name: (p.stat().st_size, 'sha256:' + hashlib.sha256(p.read_bytes()).hexdigest(), 'uploaded') for p in assets}
        if uploaded != expected:
            raise SystemExit('Draft asset size/hash mismatch; inspect before publishing')
        if api(f'repos/{REPO}/git/ref/tags/{args.version}')['object']['sha'] != sha:
            raise SystemExit('Release tag moved; inspect before publishing')
        print(run('gh', 'release', 'edit', args.version, '--repo', REPO, '--draft=false',
                  '--latest=' + ('true' if args.stable else 'false')))
        print(f'https://github.com/{REPO}/releases/tag/{args.version}')


if __name__ == '__main__':
    main()
