"""Build the NAS deployment bundle from an explicit, credential-free file list."""
import hashlib
from pathlib import Path
import re
import zipfile

root = Path(__file__).resolve().parent.parent
output = root / 'dist'
output.mkdir(exist_ok=True)
files = {
    'compose.nas.yaml': 'compose.yaml',
    'compose.https.yaml': 'compose.https.yaml',
    'deploy/compose.nas.turn-tls.yaml': 'deploy/compose.nas.turn-tls.yaml',
    'deploy/Caddyfile': 'deploy/Caddyfile',
    'docs/NAS.md': 'docs/NAS.md',
    'docs/DEPLOYMENT.md': 'docs/DEPLOYMENT.md',
    'docs/SCREEN-SHARING.md': 'docs/SCREEN-SHARING.md',
}
readme = '''# RoomDeck NAS

1. Edit ROOMDECK_MEDIA_IP in compose.yaml to your NAS LAN IP.
2. Import compose.yaml into the NAS Compose project manager and start it.
3. Open http://NAS-IP:8080 and initialize using the token in roomdeck logs.

中文完整教程 / Full installation guide: docs/NAS.md

No .env or external initialization script is needed. Persistent Docker volumes
are the default. Screen capture requires trusted HTTPS and reachable media ports.
Keep the existing project name, data mounts and custom settings when migrating.
Do not run docker compose down -v against an installation you want to keep.

Images: ghcr.io/panda-995/roomdeck:latest (linux/amd64 and linux/arm64).
Source: https://github.com/Panda-995/RoomDeck
'''.encode('utf8')
payload = {'README.md': readme}
for source, target in files.items():
    payload[target] = (root / source).read_bytes()
for name, data in payload.items():
    if re.search(rb'gh[pousr]_[A-Za-z0-9]{20,}|github_pat_[A-Za-z0-9_]{20,}|-----BEGIN .*PRIVATE KEY', data):
        raise SystemExit(f'Credential-like content detected in {name}')
archive = output / 'RoomDeck-NAS.zip'
with zipfile.ZipFile(archive, 'w', zipfile.ZIP_DEFLATED) as bundle:
    for name, data in sorted(payload.items()):
        info = zipfile.ZipInfo('RoomDeck-NAS/' + name, (2026, 9, 13, 0, 0, 0))
        info.compress_type = zipfile.ZIP_DEFLATED
        info.external_attr = 0o100644 << 16
        bundle.writestr(info, data)
with zipfile.ZipFile(archive) as bundle:
    assert bundle.testzip() is None
digest = hashlib.sha256(archive.read_bytes()).hexdigest()
(output / 'RoomDeck-NAS-SHA256SUMS.txt').write_text(f'{digest}  {archive.name}\n', encoding='ascii')
print(f'{archive.name}: {len(payload)} files; SHA256 {digest}')
