# Release Checklist

Use this checklist before publishing a new release and announcing it publicly.

## 1) Repository Health

- [ ] Default branch is `main`
- [ ] Repository visibility is set as intended (public/private)
- [ ] README badges resolve successfully
- [ ] `README.md`, `CONTRIBUTING.md`, `CODE_OF_CONDUCT.md`, `LICENSE`, `SECURITY.md` are present

## 2) CI Verification

- [ ] CI pipeline passes on `main`
- [ ] `go vet ./...` passes
- [ ] `go test ./...` passes
- [ ] End-to-end smoke job passes in CI

## 3) Local Validation

- [ ] `go build -o entropy ./cmd/entropy`
- [ ] `./entropy --help` works
- [ ] `./scripts/e2e-smoke.sh` passes locally
- [ ] `./scripts/e2e-smoke.sh --with-demo-compose` passes locally

## 4) Documentation Accuracy

- [ ] Commands in README match actual CLI behavior
- [ ] `chaos.example.yaml` is current
- [ ] `chaos-scenario.example.yaml` is current
- [ ] Docs under `docs/` reflect latest command set and features

## 5) Release Preparation

- [ ] `CHANGELOG.md` updated under `[Unreleased]`
- [ ] Version tag selected (for example `v0.1.0`)
- [ ] Release notes drafted from changelog entries

## 6) Publish

- [ ] Create and push tag:
  - `git tag vX.Y.Z`
  - `git push origin vX.Y.Z`
- [ ] Verify GitHub Release workflow completed
- [ ] Verify release assets exist for all target platforms

## 7) Post-Release

- [ ] Validate installation from release assets
- [ ] Announce release (LinkedIn/Reddit/GitHub)
- [ ] Open follow-up issues for known gaps or roadmap items
