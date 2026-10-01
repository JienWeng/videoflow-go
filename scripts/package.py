#!/usr/bin/env python3
"""Build beginner downloads after `cd frontend && npm ci && npm run build`."""
import argparse
import hashlib
import os
from pathlib import Path
import shutil
import subprocess
import tempfile
import zipfile

ROOT = Path(__file__).resolve().parents[1]
TARGETS = [('linux', 'amd64'), ('linux', 'arm64'), ('darwin', 'amd64'), ('darwin', 'arm64')]


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument('--target', choices=[f'{s}-{a}' for s, a in TARGETS])
    parser.add_argument('--output', default=str(ROOT / 'dist'))
    args = parser.parse_args()
    if not (ROOT / 'frontend/build/index.html').exists():
        parser.error('Build the frontend first: cd frontend && npm ci && npm run build')
    output = Path(args.output).resolve()
    output.mkdir(parents=True, exist_ok=True)
    checksums = []
    for system, arch in TARGETS:
        target = f'{system}-{arch}'
        if args.target and target != args.target:
            continue
        with tempfile.TemporaryDirectory(prefix='videoflow-package-') as temp:
            folder = Path(temp) / 'VideoFlow'
            folder.mkdir()
            binary = 'videoflow.exe' if system == 'windows' else 'videoflow'
            env = dict(os.environ, GOOS=system, GOARCH=arch, CGO_ENABLED='0')
            subprocess.run(['go', 'build', '-trimpath', '-o', str(folder / binary), './cmd/server'], cwd=ROOT, env=env, check=True)
            shutil.copytree(ROOT / 'frontend/build', folder / 'frontend')
            shutil.copyfile(ROOT / 'docs/QUICKSTART.txt', folder / 'START HERE.txt')
            if system == 'windows':
                (folder / 'Start VideoFlow.bat').write_text('@echo off\ncd /d "%~dp0"\nvideoflow.exe --desktop\nif errorlevel 1 pause\n', encoding='utf-8')
            else:
                launcher = folder / ('Start VideoFlow.command' if system == 'darwin' else 'Start VideoFlow.sh')
                launcher.write_text('#!/bin/sh\ncd "$(dirname "$0")" || exit 1\nexec ./videoflow --desktop\n')
                launcher.chmod(0o755)
            archive = output / f'VideoFlow-{target}.zip'
            with zipfile.ZipFile(archive, 'w', zipfile.ZIP_DEFLATED) as bundle:
                for path in sorted(folder.rglob('*')):
                    if path.is_file():
                        bundle.write(path, path.relative_to(folder.parent))
            checksums.append(f'{hashlib.sha256(archive.read_bytes()).hexdigest()}  {archive.name}')
            print(archive)
    (output / 'SHA256SUMS.txt').write_text('\n'.join(checksums) + '\n')


if __name__ == '__main__':
    main()
