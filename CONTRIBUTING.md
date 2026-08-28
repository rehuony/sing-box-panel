# Contributing

Thank you for helping improve sing-box-panel. Contributions should be focused,
reviewable, and consistent with the project's existing contracts and security
boundaries.

Participation is governed by the [Code of Conduct](CODE_OF_CONDUCT.md). Report
security vulnerabilities privately according to the [Security Policy](SECURITY.md),
not through a public issue or pull request.

## Before Opening an Issue

Search existing issues first. Use the Bug report, Feature request, or Question
form and remove management tokens, subscription tokens, configuration secrets,
private paths, and personal data from all examples and logs.

Open an issue before a large behavioral, compatibility, dependency, or
architectural change so the intended contract can be agreed before substantial
implementation work begins.

## Development Setup

The repository currently requires:

- Go 1.26, as declared by `go.mod`;
- Node.js 22.12 or newer;
- Corepack with the pnpm version pinned by `web/package.json`; and
- Bash and the ordinary platform tools used by the project scripts.

Install locked Go and Web dependencies from the repository root:

```sh
make bootstrap
```

Use a supported Linux environment when changing Linux-only installer, systemd,
runtime identity, or release behavior.

## Making Changes

- Understand the owning package, contract, tests, and documentation before
  changing behavior.
- Keep transport, application, domain, persistence, and presentation
  responsibilities separate.
- Add or update focused tests with behavioral changes.
- Keep `api/openapi.yaml`, CLI help, Web clients, and user documentation aligned
  with public contract changes.
- Run `make notices` when production dependencies change and include the
  resulting `THIRD_PARTY_NOTICES` update.
- Follow the version-onboarding and release guides for adapter or packaging
  changes instead of adding compatibility fallbacks.
- Do not commit settings, databases, generated binaries, `web/dist`, release
  private keys, credentials, tokens, or secret-bearing diagnostics.

## Validation

Run the repository's ordinary checks before opening a pull request:

```sh
make check
```

For changes to concurrency-sensitive Go behavior or parser boundaries, also
run:

```sh
make test-race fuzz-smoke
```

For release packaging, signing, installer integration, or release smoke-test
changes, also run:

```sh
make release-verify
```

Record every command run and any intentionally omitted check in the pull
request. Do not weaken checks merely to make a change pass.

## Pull Requests

Create a focused branch and keep each pull request to one coherent change.
Explain the problem, the chosen behavior, compatibility or migration impact,
security implications, and validation evidence. Link the relevant issue when
one exists and respond to review feedback with code, tests, or documented
reasoning.

Contributions are accepted under the project's
[GPL-3.0-or-later license](LICENSE).
