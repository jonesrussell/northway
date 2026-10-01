"""Supported Linux, candidate-local PHP qualification. No production mutations.

Copies this candidate's source to an isolated Unix filesystem and installs its
exact lock offline from the local Composer cache. No donor vendor or autoload shim.
The Windows-mounted source's file modes are not a site-doctor qualification.
"""
import json
import os
from pathlib import Path
import shutil
import subprocess
import tempfile

root = Path(__file__).resolve().parents[1]
if os.name != 'posix':
    raise SystemExit('Run in the supported Linux environment.')
with tempfile.TemporaryDirectory(prefix='northcloud-php-') as tmp:
    app = Path(tmp) / 'app'
    shutil.copytree(root / 'control-plane', app, ignore=shutil.ignore_patterns('vendor', 'storage', '.env', '.phpunit.cache'))
    metadata = json.loads((app / '.waaseyaa/generated.json').read_text())
    for artifact in metadata['artifacts']:
        path = (app / artifact['path']).resolve()
        assert path.is_relative_to(app.resolve())
        path.chmod(int(artifact['mode'], 8))
    env = dict(os.environ, APP_ENV='local', COMPOSER_DISABLE_NETWORK='1')
    for command in [
        ['composer','install','--quiet','--no-interaction','--no-progress','--no-plugins','--no-scripts'],
        ['php','vendor/bin/waaseyaa','install:init','--no-interaction'],
        ['php','bin/maintenance/site-verify'],
        ['php','vendor/bin/phpunit','--no-coverage'],
    ]:
        subprocess.run(command,cwd=app,env=env,check=True)
    print('PASS: isolated candidate-local PHP dependencies, strict site diagnostics and tests')
