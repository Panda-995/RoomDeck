"""Verify a public GHCR image without Docker credentials or a GitHub token."""
import json
import sys
import urllib.parse
import urllib.request

image = sys.argv[1] if len(sys.argv) > 1 else 'ghcr.io/panda-995/roomdeck:latest'
if not image.startswith('ghcr.io/'):
    raise SystemExit('Expected a ghcr.io image')
repository, tag = image.removeprefix('ghcr.io/').rsplit(':', 1)
opener = urllib.request.build_opener(urllib.request.ProxyHandler({}))
query = urllib.parse.urlencode({'service': 'ghcr.io', 'scope': f'repository:{repository}:pull'})
with opener.open('https://ghcr.io/token?' + query, timeout=30) as response:
    token = json.load(response)['token']
accept = ', '.join(['application/vnd.oci.image.index.v1+json', 'application/vnd.docker.distribution.manifest.list.v2+json', 'application/vnd.oci.image.manifest.v1+json', 'application/vnd.docker.distribution.manifest.v2+json'])

def manifest(reference):
    request = urllib.request.Request(f'https://ghcr.io/v2/{repository}/manifests/{reference}', headers={'Authorization': 'Bearer ' + token, 'Accept': accept})
    with opener.open(request, timeout=30) as response:
        return json.load(response), response.headers.get('Docker-Content-Digest')

index, digest = manifest(tag)
platforms = {}
for item in index.get('manifests', []):
    platform = item.get('platform', {})
    if platform.get('os') == 'linux' and platform.get('architecture') in ('amd64', 'arm64'):
        child, child_digest = manifest(item['digest'])
        assert child.get('layers'), 'Image has no layers'
        platforms[platform['architecture']] = child_digest
assert set(platforms) == {'amd64', 'arm64'}, platforms
print(json.dumps({'image': image, 'digest': digest, 'anonymous': True, 'platforms': platforms}, indent=2))
