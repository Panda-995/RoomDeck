"""Run only against an isolated CI deployment; never against production data."""
import http.cookiejar
import json
import os
import pathlib
import subprocess
import sys
import time
import urllib.request

base = os.environ.get('ROOMDECK_TEST_URL', 'http://127.0.0.1:8080')
opener = urllib.request.build_opener(urllib.request.ProxyHandler({}), urllib.request.HTTPCookieProcessor(http.cookiejar.CookieJar()))
def api(path, method='GET', data=None):
    req = urllib.request.Request(base+'/api/v1'+path, method=method, data=json.dumps(data).encode() if data is not None else None, headers={'Content-Type':'application/json','X-RoomDeck-Request':'1'})
    with opener.open(req, timeout=20) as res:
        return json.load(res)
for attempt in range(60):
    try:
        with opener.open(base+'/readyz', timeout=3) as res:
            assert json.load(res)['ok']
        break
    except Exception:
        if attempt == 59: raise
        time.sleep(1)
credentials = {'username':'e2e-host','password':'test-only-strong-password-2026'}
state_file = pathlib.Path('.local/container-smoke.json')
if sys.argv[1] == 'init':
    status = api('/status')
    if status['setup_required']:
        token = subprocess.check_output(['docker','compose','exec','-T','roomdeck','cat','/data/setup-token'],text=True).strip()
        api('/setup','POST',dict(credentials,token=token))
        token = None
    else:
        api('/login','POST',credentials)
    room = api('/rooms','POST',{'name':'Container persistence','duration':7200,'retention':86400})
    path = '/rooms/'+room['id']
    note = api(path+'/contents','POST',{'kind':'note','body':'Persist across container restart'})
    api(path+'/board','POST',{'action':'stroke','epoch':1,'stroke':{'id':'container-persist','color':'#30664d','width':7,'points':[{'x':10,'y':20},{'x':100,'y':150}]}})
    state_file.parent.mkdir(exist_ok=True)
    state_file.write_text(json.dumps({'room':room['id'],'note':note['id']}))
    print('Container initialized; authenticated writes and board fixture created.')
else:
    api('/login','POST',credentials)
    state = json.loads(state_file.read_text())
    path = '/rooms/'+state['room']
    snap = api(path+'/snapshot')
    assert any(c['id']==state['note'] for c in snap['contents'])
    board = api(path+'/board')
    assert any(s['id']=='container-persist' for s in board['strokes'])
    print('Restart persistence and login verified.')
