#!/usr/bin/env python3
"""Audit a downloaded app, including the signature and downloaded-app policy."""
import argparse
import json
from pathlib import Path
import subprocess

parser = argparse.ArgumentParser()
parser.add_argument('--app', required=True)
parser.add_argument('--output', required=True)
args = parser.parse_args()
app = Path(args.app).resolve()
checks = {}
commands = {
    'bundle_signature': ['codesign', '--verify', '--deep', '--strict', '--verbose=2', str(app)],
    'signature_identity': ['codesign', '-d', '--verbose=4', str(app)],
    'gatekeeper': ['spctl', '--assess', '--type', 'execute', '--verbose=4', str(app)],
    'notarization_ticket': ['xcrun', 'stapler', 'validate', str(app)],
}
for name in ['videoflow', 'ffmpeg', 'ffprobe']:
    commands['engine_signature_' + name] = ['codesign', '--verify', '--strict', '--verbose=2', str(app / 'Contents/Resources/engine' / name)]
for name, command in commands.items():
    result = subprocess.run(command, capture_output=True, text=True)
    checks[name] = {'returncode': result.returncode, 'detail': (result.stdout + result.stderr).strip()}
Path(args.output).write_text(json.dumps(checks, indent=2) + '\n')
print(json.dumps(checks, indent=2))
