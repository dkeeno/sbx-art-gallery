# art-gallery

Tiny single-binary Go website serving a fixed gallery of public-domain fine art works. Second workload on the sbx-02 GKE Autopilot cluster, and the **first consumer of `sbx-ci-templates/build-templates`** for shared CI.

## What it serves

| Route | Returns |
|---|---|
| `GET /art-gallery/` | HTML grid of all 6 works |
| `GET /art-gallery/works/{id}` | HTML detail page for one work |
| `GET /art-gallery/api/works` | JSON array |
| `GET /art-gallery/api/works/{id}` | JSON for one work |
| `GET /art-gallery/static/works/{slug}.svg` | the SVG asset |
| `GET /art-gallery/healthz` | `ok` (200) — Gateway liveness probe |

The 6 works are hardcoded in `main.go`. The 6 SVGs live in `static/works/` and are embedded into the binary at build time via `//go:embed`. No external CDN, no database, no state.

## How it's deployed

Standard sbx-02 GitOps loop:

```
git push to main
    ↓
GitLab CI runs the included `build-and-push` template (build + push :SHA to AR)
    ↓
ArgoCD Image Updater detects new tag in AR (~2 min)
    ↓
Image Updater commits the new tag to sbx-02-manifests
    ↓
ArgoCD's git poll picks up the commit (~3 min) → auto-sync
    ↓
Kubernetes rolling update → new pods serving the new image
```

End-to-end ~5–8 min from `git push` to live, hands-off.

## Local runs

Std-lib only, so:
```sh
go run .
# then
curl http://localhost:8080/art-gallery/healthz
open http://localhost:8080/art-gallery/
```

## In-cluster verification

```sh
kubectl exec -n default deploy/netshoot -- curl -fS http://10.3.11.202/art-gallery/healthz
kubectl exec -n default deploy/netshoot -- curl -fS http://10.3.11.202/art-gallery/api/works | jq .
```

## CI

Inherited from `sbx-ci-templates/build-templates@v1.0.0`. The local `.gitlab-ci.yml` is ~15 lines and only sets per-app concerns (workflow:rules with the file globs that trigger a rebuild, and `IMAGE_REPO` for AR). See [`.gitlab-ci.yml`](.gitlab-ci.yml) and the template's [README](https://gitlab.com/ice-team/emodernization/gcp-platform/test-gcp-dev-projects/sbx-ci-templates/build-templates).

## Manifests

The Kubernetes resources for this app live in the gitops repo at `sbx-02-manifests/manifests/dev/art-gallery/` — see that repo for the Deployment, Service, HTTPRoute, HealthCheckPolicy + the dev overlay. The ApplicationSet auto-generates the `art-gallery` ArgoCD Application from that directory; no manual ArgoCD config needed.
