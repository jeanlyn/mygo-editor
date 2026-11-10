import hashlib
from pathlib import Path
import tempfile
import unittest
from release import collect_assets, validate_run, TARGETS

class ReleaseTests(unittest.TestCase):
    def test_run_validation(self):
        data = dict(head_sha='abc', head_branch='main', status='completed', conclusion='success', path='.github/workflows/build.yml')
        validate_run(data, 'abc')
        for key, bad in [('head_sha','other'), ('head_branch','other'), ('status','in_progress'), ('conclusion','failure'), ('path','other.yml')]:
            with self.subTest(key=key), self.assertRaises(ValueError):
                validate_run(dict(data, **{key: bad}), 'abc')

    def test_checksums_and_complete_targets(self):
        with tempfile.TemporaryDirectory() as temp:
            root = Path(temp)
            for target, ext in TARGETS.items():
                folder = root / f'mygo-editor-{target}'
                folder.mkdir()
                name = f'mygo-editor-{target}{ext}'
                (folder / name).write_bytes(b'fixture')
                (folder / 'SHA256SUMS.txt').write_text(hashlib.sha256(b'fixture').hexdigest() + '  ' + name + '\n')
            self.assertEqual(len(collect_assets(root)), 4)
            self.assertEqual(len((root / 'SHA256SUMS.txt').read_text().splitlines()), 3)
            archive = root / 'mygo-editor-windows-amd64' / 'mygo-editor-windows-amd64.zip'
            archive.write_bytes(b'tampered')
            with self.assertRaises(ValueError): collect_assets(root)
            archive.unlink()
            with self.assertRaises(FileNotFoundError): collect_assets(root)


class PublicationTests(unittest.TestCase):
    def exercise(self, fault=None, check_only=False):
        import release
        from unittest.mock import patch
        import json
        calls, uploaded = [], []
        draft_created = False
        def fake_run(*args):
            nonlocal draft_created
            calls.append(args)
            if args[:3] == ('git', 'status', '--porcelain'): return ''
            if args[:3] == ('git', 'rev-parse', 'HEAD'): return 'abc'
            if args[:4] == ('gh','api','--paginate','--slurp'):
                if '/releases?' in args[-1] and draft_created:
                    return '[]' if fault == 'missing-draft' else '[[{"tag_name":"v0.1.0","id":7}]]'
                return '[[]]'
            if args[:3] == ('gh','run','download'):
                folder = Path(args[-1])
                for target, ext in TARGETS.items():
                    d=folder/f'mygo-editor-{target}'; d.mkdir()
                    name=f'mygo-editor-{target}{ext}'; (d/name).write_bytes(b'fixture')
                    (d/'SHA256SUMS.txt').write_text(hashlib.sha256(b'fixture').hexdigest()+'  '+name)
                return ''
            if args[:3] == ('gh','release','create'):
                draft_created = True
                for name in args[-4:]:
                    p=Path(name)
                    uploaded.append(dict(name=p.name,size=p.stat().st_size,digest='sha256:'+hashlib.sha256(p.read_bytes()).hexdigest(),state='uploaded'))
                return 'draft-url'
            return ''
        def fake_api(path):
            if '/git/ref/' in path:
                return {'object': {'sha': 'wrong' if fault=='tag-moved' and '/tags/' in path else 'abc'}}
            if path.endswith('/jobs?per_page=100'):
                return {'jobs':[{'name':t,'conclusion':'success'} for t in TARGETS]}
            if '/actions/runs/' in path:
                return dict(head_sha='abc',head_branch='main',status='completed',conclusion='success',path='.github/workflows/build.yml')
            if path.endswith('/releases/7'):
                assets=[dict(x) for x in uploaded]
                if fault=='asset-hash': assets[0]['digest']='sha256:wrong'
                if fault=='asset-state': assets[0]['state']='new'
                return dict(draft=True,target_commitish='wrong' if fault=='target' else 'abc',assets=assets)
            raise AssertionError(path)
        with tempfile.TemporaryDirectory() as temp:
            notes=Path(temp)/'notes.md';notes.write_text('notes')
            args=['release.py','v0.1.0','--run-id','123','--notes',str(notes)]
            if check_only: args += ['--check-only']
            with patch.object(release,'run',side_effect=fake_run), patch.object(release,'api',side_effect=fake_api), patch('sys.argv',args):
                if fault:
                    with self.assertRaises(SystemExit): release.main()
                else: release.main()
        published=any(c[:3]==('gh','release','edit') for c in calls)
        self.assertEqual(published, not fault and not check_only)
        if check_only:
            self.assertFalse(any(c[:3]==('gh','release','create') or c[:3]==('gh','api',f'repos/{release.REPO}/git/refs') for c in calls))

    def test_publish_verified_draft(self): self.exercise()
    def test_check_only_does_not_mutate(self): self.exercise(check_only=True)
    def test_integrity_failures_never_publish(self):
        for fault in ('missing-draft','asset-hash','asset-state','target','tag-moved'):
            with self.subTest(fault=fault): self.exercise(fault)

if __name__ == '__main__': unittest.main()
