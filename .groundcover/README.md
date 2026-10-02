# groundcover Grafana fork

`gc/<tag>` = upstream Grafana `<tag>` + the groundcover patch stack. One image serves two instances:
**mm** (monitors-manager, alert engine in every customer deployment) and **theatre** (embedded UI: BYOC UI, app.groundcover.com).

## Stack

| Commit prefix | Instance | Content |
|---|---|---|
| `gc(build)` | both | groundcover Docker target, plugin bundles, pr/release workflows, Go version; upstream workflows removed |
| `gc(mm)` fingerprints | mm | `_gc_fingerprint` on alerts (`sender/gc_fingerprint.go`), annotations in rule fingerprint |
| `gc(mm)` state history | mm | enriched entries, `log_all`, OTLP export, silence IDs, Jinja2 summary (`historian/gc_history.go`, `otel_loki_client.go`, `template/jinja2.go`, `notifier/gc_silences.go`) |
| `gc(mm)` threshold | mm | `_gc_configured_threshold` in state values (`state/threshold_extraction.go`) |
| `gc(mm)` runtime | mm | `disableInstanceStore` toggle, 5m alerting init timeout |
| `gc(theatre)` permissions | theatre | org admins without org/team write (`api/gc_accesscontrol.go`), editors get alert provisioning |
| `gc(theatre)` UI | theatre | hidden org switcher, closed menu, NoData/Error → OK defaults, `.*` All value |
| `gc(theatre)` backport | theatre | upstream #95387; dropped automatically once the target tag contains it |
| `gc(tools)` | — | this directory and `carry-upstream.yaml` |
| `gc(carry)` | — | regenerated on every carry: base tag, go.mod/go.sum from `deps.txt`, generated feature toggles |

Alerting library: `github.com/groundcover-com/alerting`, one commit (`Mutes`) on the upstream alerting version the Grafana tag pins; tag per Grafana line, referenced in `deps.txt`.

## Carrying to a new upstream tag

1. If the tag pins a new `grafana/alerting` version, cherry-pick the `Mutes` commit onto it in the alerting fork, tag it, and add an `alerting <upstream> <fork>` line to `deps.txt` (carry stops if the mapping is missing).
2. Run the **carry-upstream** workflow (`tag`, `from`), or locally: `.groundcover/carry.sh v12.4.12` from the current stack branch.
3. On conflicts it stops (exit 2, workflow pushes `gc/<tag>-wip`): resolve locally, `git cherry-pick --continue`, then `.groundcover/carry.sh --finish <tag>`. `rerere` remembers resolutions.
4. Finish regenerates the `gc(carry)` commit, builds `./pkg/...` and runs the groundcover tests.

The workflow pushes upstream history (which edits `.github/workflows`), so it needs a `CARRY_TOKEN` secret with `contents` and `workflows` write.

## Rules for new changes

- Put logic in new `gc_*.go` files; touch upstream files only with one-line hooks.
- One commit per feature, prefixed `gc(mm)`, `gc(theatre)` or `gc(build)`; amend the feature's commit rather than stacking fixes.
- Backports carry an `Upstream-Commit: <sha>` trailer.
- Dependency bumps go in `deps.txt`, never hand-edited go.mod/go.sum.
- This repository is public: no secrets, internal hostnames or customer data in commits or branches.
