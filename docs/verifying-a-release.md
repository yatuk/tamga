# Verifying a release

**No release has been made with the release workflow yet.** No tag exists,
no image is in a registry and nothing has been signed. The workflow
(`.github/workflows/release.yml`) has not run, not even as a dry run. The
commands below are what a verifier runs once a release exists; they were
written from the tools' documentation and have not been run against a real
Tamga release. If one of them does not work on the first release, that is a
defect in this page or in the workflow: please open an issue.

Until then, build from source ([deploy/README.md](../deploy/README.md)).

## What a release contains

A release is made by pushing a tag `vMAJOR.MINOR.PATCH` that is on `main`.
The workflow builds everything on GitHub-hosted runners and publishes:

| Artefact | Where | Signed by |
|---|---|---|
| Proxy image | `ghcr.io/yatuk/tamga:<version>` | cosign signature, SBOM attestation, SLSA provenance |
| Scanner service image | `ghcr.io/yatuk/tamga-scanner-service:<version>` | the same |
| Analyzer image | `ghcr.io/yatuk/tamga-analyzer:<version>` | the same |
| Classifier image (no model inside) | `ghcr.io/yatuk/tamga-classifier:<version>` | the same |
| Console image | `ghcr.io/yatuk/tamga-dashboard:<version>` | the same |
| Binaries `tamga` and `scanner-service`, one archive per platform: `tamga_<version>_<os>_<arch>.tar.gz` for linux/amd64, linux/arm64, darwin/amd64, darwin/arm64, windows/amd64 | GitHub release | listed in the signed `SHA256SUMS`; SLSA provenance |
| An SBOM for each archive (`….spdx.json`) and each image (`tamga-image-<name>_<version>.spdx.json`) | GitHub release | the same |
| Helm chart `tamga-proxy-<chart version>.tgz` | GitHub release | the same |
| Browser extension sources `tamga-extension-<version>.zip` | GitHub release | the same |
| Python SDK (wheel and sdist) and TypeScript SDK (npm tarball) | GitHub release | the same |
| `image-digests_<version>.txt`: each image as `name@sha256:…` | GitHub release | the same |
| `SHA256SUMS` and `SHA256SUMS.sigstore.json` | GitHub release | the bundle is the signature of `SHA256SUMS` |

Things to know:

- Images are `linux/amd64` only.
- There is no `latest` tag. Use the version, or better the digest.
- The console image is built with `NEXT_PUBLIC_API_URL=http://localhost:8443`
  compiled in. A console that reaches the proxy at another address has to be
  built from `dashboard/Dockerfile` with that address.
- The SDK packages are attached to the release and are not published to PyPI
  or npm. Their versions are their own, not the release's.
- The extension archive is the committed source. It is not signed for a
  browser and is in no store ([browser-extension.md](browser-extension.md)).
- The chart, the extension and the SDK packages have no SBOM: the chart holds
  no software, the extension has no dependencies, and the SDKs' runtime
  dependencies are what their own manifests declare (`httpx` for Python, none
  for TypeScript).
- The release is created as a draft and published by the maintainer. A
  version tag on an image is added only after that image's signature has
  been made and verified.

## What you need

- [cosign](https://docs.sigstore.dev/cosign/system_config/installation/)
  v3 or later. The workflow signs with cosign v3.0.6.
- The [GitHub CLI](https://cli.github.com/) (`gh`), for the provenance
  attestations.
- `sha256sum` and `jq`.

## The identity to require

Signing is keyless: there is no Tamga public key to fetch. Each signature
carries a short-lived certificate that Sigstore's CA issued to the workflow
run, and what you check is who the certificate was issued to. Require both:

| | Value |
|---|---|
| Certificate identity | `https://github.com/yatuk/tamga/.github/workflows/release.yml@refs/tags/<version>` |
| OIDC issuer | `https://token.actions.githubusercontent.com` |

That identity says: signed by the file `.github/workflows/release.yml` of the
repository `yatuk/tamga`, running for the tag `<version>`. A signature made
by any other repository (a fork included), any other workflow file or any
other ref does not match. Do not loosen it to a pattern that accepts other
workflows or branches.

```bash
VERSION=v0.9.0   # the release you are checking
IDENTITY="https://github.com/yatuk/tamga/.github/workflows/release.yml@refs/tags/${VERSION}"
ISSUER="https://token.actions.githubusercontent.com"
```

## Release files

Download the release and check the signature of the checksum file first,
then the files against it.

```bash
gh release download "$VERSION" --repo yatuk/tamga --dir "tamga-${VERSION}"
cd "tamga-${VERSION}"

# 1. SHA256SUMS was signed by the release workflow at this tag.
cosign verify-blob SHA256SUMS \
  --bundle SHA256SUMS.sigstore.json \
  --certificate-identity "$IDENTITY" \
  --certificate-oidc-issuer "$ISSUER"

# 2. Every file you downloaded is the file that was signed for.
sha256sum --check --strict SHA256SUMS
```

`cosign verify-blob` prints `Verified OK`. If you downloaded only some of the
files, use `sha256sum --check --ignore-missing SHA256SUMS`.

Build provenance of a single file (which commit, which workflow, which
runner built it), as a SLSA v1 statement recorded with GitHub:

```bash
gh attestation verify "tamga_${VERSION}_linux_amd64.tar.gz" \
  --repo yatuk/tamga \
  --signer-workflow yatuk/tamga/.github/workflows/release.yml \
  --source-ref "refs/tags/${VERSION}" \
  --deny-self-hosted-runners
```

Add `--format json` to read the statement: `buildDefinition` names the
workflow and the commit.

## Container images

Take the digest from `image-digests_<version>.txt`, which is covered by the
signed checksums, not from the tag: a tag can be moved, a digest cannot.

```bash
REF="$(grep '^ghcr.io/yatuk/tamga@' "image-digests_${VERSION}.txt")"
echo "$REF"      # ghcr.io/yatuk/tamga@sha256:…

# 1. The image was signed by the release workflow at this tag.
cosign verify "$REF" \
  --certificate-identity "$IDENTITY" \
  --certificate-oidc-issuer "$ISSUER"

# 2. Its SBOM, as a signed attestation on the image.
cosign verify-attestation "$REF" --type spdxjson \
  --certificate-identity "$IDENTITY" \
  --certificate-oidc-issuer "$ISSUER" > sbom-attestation.json

# 3. Its build provenance (SLSA v1).
gh attestation verify "oci://${REF}" \
  --repo yatuk/tamga \
  --signer-workflow yatuk/tamga/.github/workflows/release.yml \
  --source-ref "refs/tags/${VERSION}" \
  --deny-self-hosted-runners
```

For the other images replace the name in the `grep`:
`tamga-scanner-service`, `tamga-analyzer`, `tamga-classifier`,
`tamga-dashboard`.

Then deploy by digest, so that what runs is what you verified: in a Compose
file `image: ghcr.io/yatuk/tamga@sha256:…`. The Helm chart renders
`<image.repository>:<image.tag>` and has no digest value of its own; setting
`image.repository=ghcr.io/yatuk/tamga@sha256` and `image.tag=<the hex digest>`
renders a digest reference.

To have a cluster refuse unsigned images, give your admission controller
(Kyverno, the Sigstore policy controller) the same identity and issuer.

## The SBOMs

The SBOMs are SPDX 2.3 JSON, generated by
[syft](https://github.com/anchore/syft) from the built artefact: from the
image in the registry, and from the binaries in each archive (Go records the
modules a binary was built from inside it).

```bash
# The file attached to the release, already covered by SHA256SUMS:
jq -r '.packages[] | "\(.name) \(.versionInfo)"' \
  "tamga-image-proxy_${VERSION}.spdx.json" | sort | head

# The same document from the attestation on the image:
jq -r '.payload' sbom-attestation.json | base64 -d | jq '.predicate.packages | length'
```

An SBOM lists what the scanner found in the artefact. It is as complete as
the scanner: a file copied into an image without package metadata (the
`grpc_health_probe` binary in the scanner-service image is fetched that way)
may appear only as a Go binary with its modules, or not at all. Feed the
file to the vulnerability scanner you already use
(`grype sbom:tamga-image-proxy_${VERSION}.spdx.json`, for example).

Licences of the dependencies are in
[THIRD_PARTY_LICENSES.md](../THIRD_PARTY_LICENSES.md), which each archive
also contains.

## Rebuilding a binary

The binaries are built with `-trimpath`, with the version and the commit
linked in, and with nothing else that varies between two builds:

```bash
git clone https://github.com/yatuk/tamga.git && cd tamga
git checkout "$VERSION"
cd proxy     # use the Go version that `go.mod` names
PKG=github.com/yatuk/tamga/internal/buildinfo
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -buildvcs=true \
  -ldflags "-s -w -buildid= -X ${PKG}.Version=${VERSION} -X ${PKG}.Revision=$(git rev-parse HEAD)" \
  -o /tmp/tamga ./cmd/tamga
sha256sum /tmp/tamga     # compare with the tamga in the release archive
```

Build in a clean checkout: an untracked or changed file makes Go record
`vcs.modified=true` and the bytes differ. `go version -m tamga` on a release
binary shows the commit (`vcs.revision`), `vcs.modified=false` and
`-trimpath=true`.

What has been checked: on one machine, two builds of the same commit gave
the same bytes for both binaries on all five platforms, and the archive step
gave the same bytes twice. What has not: a rebuild on another machine
against a released binary, since there is none. The
container images are not reproducible: they install packages from Alpine,
Debian, PyPI and npm at build time.

## What verification tells you, and what it does not

A valid signature and provenance tell you that the artefact was built by
that workflow file, from that commit of `yatuk/tamga`, on a GitHub-hosted
runner, and has not been changed since. They do not tell you that the code
is free of defects, and they say nothing about what the build pulled in: the
base images are referenced by tag, and the Python and npm dependencies below
the direct ones are not pinned by hash. The trust placed in GitHub Actions,
in Sigstore's public instance (Fulcio, Rekor) and in those upstream
registries remains. See the [threat model](threat-model.md).

## If verification fails

Do not run the artefact. Check that the version in `IDENTITY` is the one you
downloaded and that cosign is v3 or later, and try the digest from
`image-digests_<version>.txt` rather than a tag. If it still fails, report it
as [SECURITY.md](../SECURITY.md) describes.
