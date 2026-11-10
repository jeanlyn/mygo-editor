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

if __name__ == '__main__': unittest.main()
