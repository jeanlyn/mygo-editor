"""Package already-built executables; no publishing or signing."""
from pathlib import Path
import hashlib
import sys
import tarfile
import zipfile

root = Path(__file__).resolve().parent.parent
target = sys.argv[1]
if target not in ('windows-amd64', 'macos-arm64', 'macos-amd64'):
    raise SystemExit('Unsupported target')
windows = target.startswith('windows-')
binary = root / 'build' / ('mygo-editor.exe' if windows else 'mygo-editor')
if not binary.is_file():
    raise SystemExit(f'Missing binary: {binary}')
dist = root / 'dist'
dist.mkdir(exist_ok=True)
name = f'mygo-editor-{target}'
files = [binary, root / 'README.md', root / 'VALIDATION.md', root / 'THIRD_PARTY_NOTICES.md']
files += sorted((root / 'licenses').glob('*.txt'))
if windows:
    archive = dist / (name + '.zip')
    with zipfile.ZipFile(archive, 'w', zipfile.ZIP_DEFLATED) as out:
        for file in files:
            out.write(file, f'{name}/' + (f'licenses/{file.name}' if file.parent.name == 'licenses' else file.name))
else:
    # tar preserves the executable bit when downloaded through Actions' ZIP.
    archive = dist / (name + '.tar.gz')
    with tarfile.open(archive, 'w:gz') as out:
        for file in files:
            out.add(file, arcname=f'{name}/' + (f'licenses/{file.name}' if file.parent.name == 'licenses' else file.name))
digest = hashlib.sha256(archive.read_bytes()).hexdigest()
(dist / 'SHA256SUMS.txt').write_text(f'{digest}  {archive.name}\n', encoding='utf-8')
print(archive)
