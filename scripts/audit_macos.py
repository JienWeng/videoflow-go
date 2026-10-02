#!/usr/bin/env python3
"""Audit app signatures, Gatekeeper acceptance and stapled notarization."""
import argparse
import json
from pathlib import Path
import subprocess
import sys

REQUIRED_CHECKS = ['bundle_signature', 'signature_identity', 'gatekeeper', 'notarization_ticket',
                   'engine_signature_videoflow', 'engine_signature_ffmpeg', 'engine_signature_ffprobe']

def release_errors(checks):
    errors = [name for name in REQUIRED_CHECKS if checks.get(name, {}).get('returncode') != 0]
    identity = checks.get('signature_identity', {}).get('detail', '')
    if 'Authority=Developer ID Application:' not in identity:
        errors.append('Developer ID Application identity')
    return errors

def audit(app):
    app = Path(app).resolve()
    commands = {
        'bundle_signature': ['codesign', '--verify', '--deep', '--strict', '--verbose=2', str(app)],
        'signature_identity': ['codesign', '-d', '--verbose=4', str(app)],
        'gatekeeper': ['spctl', '--assess', '--type', 'execute', '--verbose=4', str(app)],
        'notarization_ticket': ['xcrun', 'stapler', 'validate', str(app)],
    }
    for name in ['videoflow', 'ffmpeg', 'ffprobe']:
        commands['engine_signature_' + name] = ['codesign', '--verify', '--strict', '--verbose=2', str(app / 'Contents/Resources/engine' / name)]
    checks = {}
    for name, command in commands.items():
        result = subprocess.run(command, capture_output=True, text=True)
        checks[name] = {'returncode': result.returncode, 'detail': (result.stdout + result.stderr).strip()}
    return checks

def main():
    parser = argparse.ArgumentParser()
    parser.add_argument('--app', required=True)
    parser.add_argument('--output', required=True)
    parser.add_argument('--require-trusted', action='store_true')
    parser.add_argument('--require-valid-signature', action='store_true')
    args = parser.parse_args()
    checks = audit(args.app)
    Path(args.output).write_text(json.dumps(checks, indent=2) + '\n')
    print(json.dumps(checks, indent=2))
    if args.require_trusted:
        failures = release_errors(checks)
    elif args.require_valid_signature:
        failures = [name for name in REQUIRED_CHECKS if name not in ['gatekeeper', 'notarization_ticket'] and checks[name]['returncode'] != 0]
    else:
        failures = []
    if failures:
        print('macOS release verification failed: ' + ', '.join(failures), file=sys.stderr)
        return 1
    return 0

if __name__ == '__main__':
    sys.exit(main())
