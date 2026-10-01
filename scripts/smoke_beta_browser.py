"""Real local beta journey. Two approved public feeds may be read, bounded by Go.
No credentials or service changes outside this disposable test environment.
"""
import base64
import json
import os
from pathlib import Path
import shutil
import signal
import socket
import subprocess
import tempfile
import time
import urllib.request

root=Path(__file__).resolve().parents[1]
def port():
    with socket.socket() as s:
        s.bind(('127.0.0.1',0)); return s.getsockname()[1]
with tempfile.TemporaryDirectory(prefix='northcloud-browser-') as tmp:
    tmp=Path(tmp);app=tmp/'app';shutil.copytree(root/'control-plane',app,ignore=shutil.ignore_patterns('vendor','storage','.env','.phpunit.cache'))
    for item in json.loads((app/'.waaseyaa/generated.json').read_text())['artifacts']:
        (app/item['path']).chmod(int(item['mode'],8))
    phpport,goport=port(),port();origin=f'http://127.0.0.1:{phpport}';api=f'http://127.0.0.1:{goport}'
    seed=base64.b64encode(os.urandom(32)).decode()
    # Generate only ephemeral test signing material, without printing it.
    pub=subprocess.check_output(['php','-r','echo base64_encode(sodium_crypto_sign_publickey(sodium_crypto_sign_seed_keypair(base64_decode(getenv("TEST_SEED")))));'],env=dict(os.environ,TEST_SEED=seed),text=True)
    env=dict(os.environ,APP_ENV='local',APP_URL=origin,WAASEYAA_APP_SECRET='base64:'+base64.b64encode(os.urandom(32)).decode(),NORTHCLOUD_API_URL=api,NORTHCLOUD_SIGNING_KEY_ID='test',NORTHCLOUD_SIGNING_SEED=seed,COMPOSER_DISABLE_NETWORK='1',PHP_CLI_SERVER_WORKERS='4')
    subprocess.run(['composer','install','--quiet','--no-interaction','--no-progress','--no-plugins','--no-scripts'],cwd=app,env=env,check=True)
    subprocess.run(['php','vendor/bin/waaseyaa','install:init','--no-interaction'],cwd=app,env=env,check=True,stdout=subprocess.DEVNULL)
    binary=tmp/'northway';subprocess.run(['go','build','-o',str(binary),'./cmd/northway'],cwd=root,check=True)
    dbdir=tmp/'data';dbdir.mkdir(mode=0o700);database=dbdir/'northway.sqlite'
    subprocess.run([str(binary),'migrate','--database',str(database)],check=True,stdout=subprocess.DEVNULL)
    goenv=dict(env,NORTHWAY_DATABASE_PATH=str(database),NORTHWAY_LISTEN_ADDR=f'127.0.0.1:{goport}',NORTHCLOUD_ASSERTION_ISSUER=origin,NORTHCLOUD_ASSERTION_AUDIENCE='northcloud-api',NORTHCLOUD_ASSERTION_KEYS=json.dumps({'test':pub}),NORTHCLOUD_CATALOGUE='developer-v1')
    processes=[]
    with (tmp/'process.log').open('w+') as log:
        try:
            processes.append(subprocess.Popen([str(binary),'serve'],cwd=root,env=goenv,stdout=log,stderr=log,start_new_session=True))
            processes.append(subprocess.Popen(['php','-S',f'127.0.0.1:{phpport}','-t','public','public/index.php'],cwd=app,env=env,stdout=log,stderr=log,start_new_session=True))
            for _ in range(60):
                try:
                    with urllib.request.urlopen(origin+'/login',timeout=3) as response:
                        if response.status==200:break
                except Exception:time.sleep(.5)
            else:raise RuntimeError('Local PHP server did not become ready')
            private=tmp/'private';private.mkdir(mode=0o700)
            module=os.environ.get('PLAYWRIGHT_MODULE') or subprocess.check_output(['node','-p','require.resolve("playwright")'],cwd=root,text=True).strip()
            browserenv=dict(env,BETA_ORIGIN=origin,BETA_API_ORIGIN=api,BETA_APP=str(app),BETA_PRIVATE_DIR=str(private),PLAYWRIGHT_MODULE=module)
            subprocess.run(['node',str(app/'tests/browser.cjs')],cwd=app,env=browserenv,check=True,timeout=240)
            for process in reversed(processes):
                os.killpg(process.pid,signal.SIGTERM);process.wait(timeout=10)
            processes.clear()
            # Exercise support against the populated real-journey data, with all
            # writers quiesced. No private account/export contents are printed.
            accounts=[json.loads((private/f'account-{i}.json').read_text()) for i in range(2)]
            first=accounts[0];tenant=first['workspace_uuid']
            subprocess.run([str(binary),'customer','export','--database',str(database),'--tenant',tenant,'--output',str(private/'workspace.ndjson')],check=True)
            records=[json.loads(line) for line in (private/'workspace.ndjson').read_text().splitlines()]
            assert any(r.get('type')=='feedback' and r['records'] for r in records)
            assert any(r.get('type')=='articles' and r['records'] for r in records)
            for action in ['suspend','delete']:
                subprocess.run([str(binary),'customer',action,'--database',str(database),'--tenant',tenant,'--confirm-tenant',tenant],check=True)
            subprocess.run(['php','bin/beta-account.php'],cwd=app,env=env,input=json.dumps(dict(action='delete',account_id=first['account_id'],confirm_account_id=first['account_id'],workspace_uuid=tenant,identity_verified=True,data_plane_deleted=True)),text=True,check=True,stdout=subprocess.DEVNULL)
            subprocess.run([str(binary),'customer','export','--database',str(database),'--tenant',accounts[1]['workspace_uuid'],'--output',str(private/'other-workspace.ndjson')],check=True)
            assert any(r.get('type')=='feeds' and r['records'] for r in map(json.loads,(private/'other-workspace.ndjson').read_text().splitlines()))
            print('PASS: populated private account/workspace export, offline suspension/deletion and other-workspace preservation')
        except Exception:
            # Show only bounded safe diagnostics, never bodies or environment.
            log.flush();print('Local process diagnostics:')
            for line in (tmp/'process.log').read_text(errors='replace').splitlines()[-20:]:
                if 'Fatal error' in line or 'Uncaught' in line or 'publisher' in line or 'ERROR' in line: print(line[:400])
            raise
        finally:
            for process in reversed(processes):
                os.killpg(process.pid,signal.SIGTERM)
                try:process.wait(timeout=10)
                except subprocess.TimeoutExpired:os.killpg(process.pid,signal.SIGKILL);process.wait()
