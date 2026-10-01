"""Disposable packaged PHP/FPM/nginx + Go acceptance under proposed memory caps.
No production credentials. HTTP is loopback-only; cookies are explicitly carried
by this test client. Public TLS is a separate target acceptance gate.
"""
import base64
import http.client
from http.cookies import SimpleCookie
import json
import os
from pathlib import Path
import subprocess
import time
import uuid

prefix='northcloud-check-'+uuid.uuid4().hex[:10]
names=[prefix+'-api',prefix+'-php',prefix+'-web'];volumes=[prefix+'-data',prefix+'-accounts',prefix+'-restored-data',prefix+'-restored-accounts']
seed=base64.b64encode(os.urandom(32)).decode()
env=dict(os.environ,APP_URL='https://northcloud.one',WAASEYAA_APP_SECRET='base64:'+base64.b64encode(os.urandom(32)).decode(),NORTHCLOUD_SIGNING_SEED=seed,NORTHCLOUD_SIGNING_KEY_ID='test',NORTHCLOUD_API_URL='http://127.0.0.1:8080')
pub=subprocess.check_output(['php','-r','echo base64_encode(sodium_crypto_sign_publickey(sodium_crypto_sign_seed_keypair(base64_decode(getenv("NORTHCLOUD_SIGNING_SEED")))));'],env=env,text=True)
env.update(NORTHCLOUD_ASSERTION_ISSUER=env['APP_URL'],NORTHCLOUD_ASSERTION_AUDIENCE='northcloud-api',NORTHCLOUD_ASSERTION_KEYS=json.dumps({'test':pub}))
def docker(*args,input=None,check=True):
    return subprocess.run(['docker',*args],input=input,text=True,env=env,check=check,stdout=subprocess.PIPE,stderr=subprocess.PIPE,timeout=180).stdout.strip()
phpenv=sum((['-e',key] for key in ['APP_URL','WAASEYAA_APP_SECRET','NORTHCLOUD_SIGNING_SEED','NORTHCLOUD_SIGNING_KEY_ID','NORTHCLOUD_API_URL']),[])
apienv=sum((['-e',key] for key in ['NORTHCLOUD_ASSERTION_ISSUER','NORTHCLOUD_ASSERTION_AUDIENCE','NORTHCLOUD_ASSERTION_KEYS']),[])
def request(port,path,method='GET',body=None,cookies=None,bearer=None):
    conn=http.client.HTTPConnection('127.0.0.1',port,timeout=35)
    headers={'Host':'northcloud.one','X-Real-IP':'127.0.0.1'}
    if body is not None:headers['Content-Type']='application/json'
    if cookies:
        headers['Cookie']='; '.join(k+'='+v for k,v in cookies.items())
        if method!='GET':headers['X-XSRF-TOKEN']=cookies.get('XSRF-TOKEN','')
    if bearer:headers['Authorization']='Bearer '+bearer
    conn.request(method,path,body=None if body is None else json.dumps(body),headers=headers)
    response=conn.getresponse();payload=response.read();status=response.status
    if cookies is not None:
        for name,value in response.getheaders():
            if name.lower()=='set-cookie':
                jar=SimpleCookie();jar.load(value)
                for k,v in jar.items():cookies[k]=v.value
    conn.close();return status,payload
try:
    for volume in volumes:docker('volume','create',volume)
    docker('run','--rm','-v',volumes[0]+':/data','northcloud-api:local-check','migrate','--database','/data/northway/northway.sqlite')
    docker('run','--rm',*phpenv,'-v',volumes[1]+':/app/storage','northcloud-php:local-check','php','vendor/bin/waaseyaa','db:init','--no-sync-schema','--no-interaction')
    # An ordinary production consumer must still refuse before genesis.
    probe=r'require "vendor/autoload.php"; try {$k=new \Waaseyaa\Foundation\Kernel\ConsoleKernel("/app");$k->getProviders();(new ReflectionMethod($k,"boot"))->invoke($k);exit(3);} catch (\Waaseyaa\Config\Authority\ConfigurationAuthorityUnavailableException $e) {echo "generation refused";}'
    assert docker('run','--rm',*phpenv,'-v',volumes[1]+':/app/storage','northcloud-php:local-check','php','-r',probe)=='generation refused'
    first=docker('run','--rm',*phpenv,'-v',volumes[1]+':/app/storage','northcloud-php:local-check','php','vendor/bin/waaseyaa','install:init','--no-interaction')
    assert 'Activated generation' in first
    second=docker('run','--rm',*phpenv,'-v',volumes[1]+':/app/storage','northcloud-php:local-check','php','vendor/bin/waaseyaa','install:init','--no-interaction')
    assert 'Configuration already initialized' in second
    print('PASS: production refuses before genesis; fresh and idempotent install:init')
    docker('run','-d','--name',names[0],'-p','127.0.0.1::8081','-p','127.0.0.1::8080','--memory','128m','--memory-swap','128m','--cpus','.25','--pids-limit','64','--read-only','--tmpfs','/tmp:size=8m,mode=1777','--cap-drop','ALL','--security-opt','no-new-privileges',*apienv,'-e','NORTHWAY_DATABASE_PATH=/data/northway/northway.sqlite','-e','GOMEMLIMIT=96MiB','-e','GOMAXPROCS=1','-e','NORTHCLOUD_CATALOGUE=developer-v1','-v',volumes[0]+':/data','northcloud-api:local-check')
    docker('run','-d','--name',names[1],'--network','container:'+names[0],'--memory','256m','--memory-swap','256m','--cpus','.25','--pids-limit','32','--cap-drop','ALL','--cap-add','CHOWN','--cap-add','SETUID','--cap-add','SETGID','--security-opt','no-new-privileges',*phpenv,'-v',volumes[1]+':/app/storage','northcloud-php:local-check')
    docker('run','-d','--name',names[2],'--network','container:'+names[0],'--memory','32m','--memory-swap','32m','--cpus','.1','--pids-limit','16','--read-only','--tmpfs','/var/cache/nginx:size=8m','--tmpfs','/var/run:size=1m','--cap-drop','ALL','--cap-add','CHOWN','--cap-add','SETUID','--cap-add','SETGID','--security-opt','no-new-privileges','northcloud-web:local-check')
    webport=int(docker('port',names[0],'8081').rsplit(':',1)[1]);apiport=int(docker('port',names[0],'8080').rsplit(':',1)[1])
    for attempt in range(60):
        try:
            if request(webport,'/login')[0]==200:break
        except (OSError,http.client.HTTPException):pass
        time.sleep(.5)
    else:
        print(docker('logs',names[1],check=False)[-3000:])
        diagnostic=subprocess.run(['docker','logs',names[1]],text=True,capture_output=True,timeout=10)
        print(diagnostic.stderr[-3000:])
        raise AssertionError('Packaged PHP did not become ready')
    docker('exec','--user','www-data',names[1],'mkdir','-m','700','/app/storage/private-test')
    def operator(data):return docker('exec','-i','--user','www-data',names[1],'php','bin/beta-account.php',input=json.dumps(data))
    accounts=[]
    for index in range(2):
        path='/app/storage/private-test/invite-'+str(index)
        operator(dict(action='invite',output_file=path))
        token=docker('exec','--user','www-data',names[1],'cat',path)
        cookies={};assert request(webport,'/register',cookies=cookies)[0]==200
        status,body=request(webport,'/api/auth/register','POST',dict(name='Fixture '+str(index),email=f'fixture{index}@example.test',password='Disposable-fixture-Only-49!',invite_token=token),cookies)
        if status!=201:
            print('Registration failure:',status,body.decode()[:1500])
            logs=subprocess.run(['docker','logs',names[1]],text=True,capture_output=True,timeout=10)
            print(logs.stderr[-2500:])
        assert status==201,('packaged registration status',status)
        data=json.loads(body);assert data['meta']['approval_required'] is True
        accounts.append(dict(id=str(data['data']['id']),cookies=cookies))
        operator(dict(action='activate',account_id=str(data['data']['id']),identity_verified=True))
        assert request(webport,'/api/auth/login','POST',dict(username=f'fixture{index}@example.test',password='Disposable-fixture-Only-49!'),cookies)[0]==200
        assert request(webport,'/api/customer/workspace','PUT',cookies=cookies)[0]==200
        status,body=request(webport,'/api/customer/keys','POST',dict(scopes='feeds:read'),cookies);assert status==201
        key=json.loads(body);secret=key['secret']
        assert request(apiport,'/v1/feeds',bearer=secret)[0]==200
        assert request(webport,'/api/customer/keys/'+key['key']['id'],'DELETE',cookies=cookies)[0]==204
        assert request(apiport,'/v1/feeds',bearer=secret)[0]==401
        accounts[-1]['revoked']=secret
        status,body=request(webport,'/api/customer/keys','POST',dict(scopes='feeds:read'),cookies);assert status==201
        accounts[-1]['active']=json.loads(body)['secret']
    for name in names:
        state=json.loads(docker('inspect','--format','{{json .State}}',name))
        assert state['Running'] and not state['OOMKilled'],name+' failed runtime cap'
    print('PASS: packaged production PHP/FPM/nginx + Go; two invited tenants, CSRF/session boundary, scoped external API and revocation under 416 MiB total caps')
    # Freeze all writers, snapshot each SQLite store, and restore into new volumes.
    # No raw live-volume copy and no restored browser sessions.
    for name in reversed(names):docker('stop','--time','15',name)
    docker('run','--rm','-v',volumes[0]+':/data','northcloud-api:local-check','backup','--database','/data/northway/northway.sqlite','--output','/data/northway/restore.sqlite')
    copygo='mkdir -p /restore/northway && cp /source/northway/restore.sqlite /restore/northway/northway.sqlite && chown -R 65532:65532 /restore && chmod 700 /restore /restore/northway && chmod 600 /restore/northway/northway.sqlite'
    docker('run','--rm','--user','0','--entrypoint','sh','-v',volumes[0]+':/source:ro','-v',volumes[2]+':/restore','northcloud-php:local-check','-c',copygo)
    backupphp='$s=new SQLite3("/source/waaseyaa.sqlite",SQLITE3_OPEN_READONLY);$d=new SQLite3("/restore/waaseyaa.sqlite");if(!$s->backup($d)){exit(1);}if($d->querySingle("PRAGMA integrity_check")!=="ok"){exit(2);}chmod("/restore/waaseyaa.sqlite",0600);'
    docker('run','--rm','--user','0','--entrypoint','php','-v',volumes[1]+':/source','-v',volumes[3]+':/restore','northcloud-php:local-check','-r',backupphp)
    docker('run','--rm','--user','0','--entrypoint','sh','-v',volumes[3]+':/restore','northcloud-php:local-check','-c','chown -R www-data:www-data /restore && chmod 700 /restore')
    # The supported migration command restores connection settings (including WAL)
    # on a backup before the fail-closed server may open it.
    docker('run','--rm','-v',volumes[2]+':/data','northcloud-api:local-check','migrate','--database','/data/northway/northway.sqlite')
    # Replace only test-owned container mounts; same image, environment and caps.
    for name in reversed(names):docker('rm',name)
    apiargs=['run','-d','--name',names[0],'-p','127.0.0.1::8081','-p','127.0.0.1::8080','--memory','128m','--memory-swap','128m','--cpus','.25','--pids-limit','64','--read-only','--tmpfs','/tmp:size=8m,mode=1777','--cap-drop','ALL','--security-opt','no-new-privileges',*apienv,'-e','NORTHWAY_DATABASE_PATH=/data/northway/northway.sqlite','-e','GOMEMLIMIT=96MiB','-e','GOMAXPROCS=1','-e','NORTHCLOUD_CATALOGUE=developer-v1','-v',volumes[2]+':/data','northcloud-api:local-check']
    docker(*apiargs)
    docker('run','-d','--name',names[1],'--network','container:'+names[0],'--memory','256m','--memory-swap','256m','--cpus','.25','--pids-limit','32','--cap-drop','ALL','--cap-add','CHOWN','--cap-add','SETUID','--cap-add','SETGID','--security-opt','no-new-privileges',*phpenv,'-v',volumes[3]+':/app/storage','northcloud-php:local-check')
    docker('run','-d','--name',names[2],'--network','container:'+names[0],'--memory','32m','--memory-swap','32m','--cpus','.1','--pids-limit','16','--read-only','--tmpfs','/var/cache/nginx:size=8m','--tmpfs','/var/run:size=1m','--cap-drop','ALL','--cap-add','CHOWN','--cap-add','SETUID','--cap-add','SETGID','--security-opt','no-new-privileges','northcloud-web:local-check')
    webport=int(docker('port',names[0],'8081').rsplit(':',1)[1]);apiport=int(docker('port',names[0],'8080').rsplit(':',1)[1])
    for attempt in range(60):
        try:
            if request(webport,'/login')[0]==200:break
        except (OSError,http.client.HTTPException):pass
        time.sleep(.5)
    else:
        diagnostic=subprocess.run(['docker','logs',names[1]],text=True,capture_output=True,timeout=10)
        print(diagnostic.stdout[-1500:]);print(diagnostic.stderr[-1500:])
        diagnostic=subprocess.run(['docker','logs',names[0]],text=True,capture_output=True,timeout=10)
        print(diagnostic.stderr[-1800:])
        raise AssertionError('Restored packaged services did not become ready')
    for index,account in enumerate(accounts):
        assert request(webport,'/api/customer/keys',cookies=account['cookies'])[0]==401
        cookies={};assert request(webport,'/login',cookies=cookies)[0]==200
        assert request(webport,'/api/auth/login','POST',dict(username=f'fixture{index}@example.test',password='Disposable-fixture-Only-49!'),cookies)[0]==200
        assert request(webport,'/api/customer/workspace','PUT',cookies=cookies)[0]==200
        assert request(apiport,'/v1/feeds',bearer=account['active'])[0]==200
        assert request(apiport,'/v1/feeds',bearer=account['revoked'])[0]==401
    for name in names:
        state=json.loads(docker('inspect','--format','{{json .State}}',name))
        assert state['Running'] and not state['OOMKilled']
    print('PASS: coherent two-store SQLite restore into independent volumes; two account logins, workspace reconciliation, retained key revocation and discarded sessions')
except subprocess.CalledProcessError as error:
    # Do not print command/env/body: operator calls can contain disposable tokens.
    if 'install:init' in error.cmd:
        diagnostic='require "vendor/autoload.php"; try {$k=new \\Waaseyaa\\Foundation\\Kernel\\ConsoleKernel("/app");$k->bootForSchemaSync();echo "restricted boot OK\\n";$c=$k->buildHandlerContainer();$c->get(\\Waaseyaa\\CLI\\Handler\\InstallInitHandler::class);echo "install handler OK\\n";} catch (Throwable $e) {echo get_class($e),"\\n",$e->getTraceAsString(),"\\n";}'
        print(docker('run','--rm',*phpenv,'-v',volumes[1]+':/app/storage','northcloud-php:local-check','php','-d','zend.exception_ignore_args=1','-r',diagnostic,check=False))
    if 'apiargs' in locals():
        diagnostic=subprocess.run(['docker','logs',names[0]],text=True,capture_output=True,timeout=10)
        print(diagnostic.stderr[-1500:])
    if 'backupphp' in locals() and error.cmd[-1]==backupphp:
        print(error.stdout[-1800:])
    raise RuntimeError('Container operation failed at '+str(error.cmd[-4:])+': '+error.stderr[-1200:]) from None
finally:
    for name in reversed(names):docker('rm','-f',name,check=False)
    for volume in volumes:docker('volume','rm',volume,check=False)
