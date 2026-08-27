# Repository Instructions

## Working Approach

- Use the current source, tests, executable configuration, dependency manifests, and authoritative component contracts to understand the repository. Summary documentation must be kept consistent with them.
- When sources disagree, resolve the conflict explicitly instead of choosing whichever is convenient. Use executable configuration and validation for toolchain facts, authoritative contracts for supported public behavior, and update stale summaries.
- When `.codegraph/` is present, use CodeGraph for initial navigation and relationship discovery, then verify conclusions against the current files. Fall back to text search when the index is incomplete or stale.
- Prefer the simplest correct solution. Reuse existing code and dependencies before adding abstractions or infrastructure.
- Make the smallest coherent change that fully addresses the request, preserve unrelated work, and avoid speculative or unrelated cleanup.

## Architecture and Evolution

- This repository currently combines a backend and CLI with a web interface. Treat the present directory layout, frameworks, and development tools as useful context, not permanent architectural constraints.
- Preserve clear separation between entry points, transport, application and domain behavior, persistence, and presentation. Change these boundaries deliberately and update affected callers, tests, contracts, and documentation together.
- Keep the web interface coupled to documented backend contracts rather than backend implementation details.
- Existing patterns are the default for routine changes. When a request intentionally upgrades or replaces a framework, toolchain, design system, dependency, or architectural pattern, apply the migration coherently and remove the superseded path when it is no longer required.
- Do not unintentionally weaken documented security, data-integrity, traceability, or artifact-verification guarantees. Changing those guarantees requires an explicit product decision and matching contract, test, and documentation updates; their implementation mechanisms may evolve.

## Contracts and Compatibility

- Use `docs/README.md` to locate component-specific authoritative references, and keep implementation, validation, and user-facing documentation consistent with them.
- Treat public APIs, persisted data, operator-facing configuration, and release formats as compatibility boundaries. Changes to them require an explicit compatibility or migration decision and coverage of the transition.
- Do not retain compatibility code for behavior that is not part of a required boundary. Remove obsolete implementations after a migration is complete.
- Produce generated or derived files through their designated workflow. If that workflow changes, update its source, outputs, validation, and documentation as one coherent change.

## Implementation and Dependencies

- Follow nearby conventions and the repository's current toolchain configuration unless the task intentionally changes them.
- Keep responsibilities focused and ownership explicit. Introduce shared abstractions or interfaces only for a current, concrete need.
- Keep interface work consistent, responsive, and accessible. Reuse established foundations for routine changes while allowing coherent interface or design-system migrations.
- Evaluate dependency changes for security, maintenance, compatibility, licensing, and fit with existing capabilities. Keep manifests, lockfiles, generated metadata, and required notices consistent.
- Add or update focused tests with behavioral changes, following the testing style of the affected area.
- For non-trivial upgrades, review the relevant upstream release, migration, compatibility, and security guidance before implementation.

## Working Tree and Git

- Preserve unrelated working-tree changes.
- Rely on Git for repository source recovery and history; do not create ad hoc backup copies, rollback scripts, archives, or duplicated source snapshots unless explicitly requested. This does not prohibit product-level data migration, backup, or rollback mechanisms.
- Do not stage, commit, amend, reset, rebase, switch branches, or otherwise change Git state or history unless the user explicitly requests that exact action.
- Leave completed changes in the working tree for review.

## Verification

- Scale verification to the changed behavior, dependencies, blast radius, and risk. Start with focused checks and broaden them whenever the change warrants it.
- Prefer repository-provided verification workflows over ad hoc substitutes.
- For toolchain, framework, or dependency upgrades, verify all affected build, test, generation, integration, and packaging paths, including supported targets when relevant.
- Run `git diff --check` after changes and report the exact checks and tests performed.

## Instruction Maintenance

- Keep repository instructions concise, in English, and limited to durable rules that apply across the project.
- Keep feature behavior, schema details, numeric interface specifications, state machines, and implementation details in source code, tests, or dedicated specifications rather than this file.
- Remove superseded guidance instead of accumulating history, duplicate rules, or compatibility notes.
- Do not record secrets, personal data, temporary debugging information, or task progress here.
