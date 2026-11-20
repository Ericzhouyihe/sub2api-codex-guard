# Patch set for Sub2API v0.2.21

All patches are applied with `git apply --binary` in filename order. They were
normalized to LF line endings; do not convert them back to CRLF.

| Patch | Purpose | Target area | Upstream range |
| --- | --- | --- | --- |
| `0001-evaluation-core.patch` | Adds the evaluation engines (candy/codex/fingerprint/pelican/SIWC/model/state) to the Sub2API server. | `backend/internal/codexeval/*.go` | v0.2.20–v0.2.21 |
| `0002-evaluation-data.patch` | Adds embedded evaluation data: lm-detector enrollment suite, model table, detector binary and Codex request context. | `backend/internal/codexeval/data/` | v0.2.20–v0.2.21 |
| `0003-vendor-assets.patch` | Adds vendor JS/CSS used by the evaluation UI (markdown-it, Temml). | `backend/internal/codexeval/web/vendor/` | v0.2.20–v0.2.21 |
| `0004-admin-channel.patch` | Adds admin evaluation handler/service and the HTTP upstream interception hooks. | `backend/internal/handler/admin/`, `backend/internal/service/`, `backend/internal/repository/` | v0.2.20–v0.2.21 |
| `0005-inspection.patch` | Adds automated inspection: settings, scheduling, notifications and admin endpoints. | `backend/internal/codexeval/inspection*.go`, `backend/internal/handler/admin/` | v0.2.20–v0.2.21 |
| `0006-evaluation-ui.patch` | Adds the embedded evaluation web UI (`/codex-eval`) and the `CodexEvalView.vue` page. | `backend/internal/codexeval/web*`, `frontend/src/views/admin/` | v0.2.20–v0.2.21 |
| `0007-sub2api-integration.patch` | Wires the evaluation service into server startup, admin routes, sidebar navigation, i18n and embedded URL handling. | `backend/cmd/server/`, `backend/internal/server/`, `frontend/src/` | v0.2.20–v0.2.21 |

`SHA256SUMS` contains the exact LF-normalized hashes checked by
`codex-eval/scripts/check_patches.py`.
