# Upgrading the fork to a new Grafana version

Runbook for moving the patch stack (`gc/<old>`) onto a new upstream tag (`gc/<new>`). The 11.3.7 → 12.4.12 carry is the reference: about 15 files needed hand resolution, and all the hotspots it hit are listed below.

Run builds and tests on a large Linux machine. The frontend suite and the multi-arch image are slow on a laptop.

## 1. Choose the tag and check the alerting pin

Prefer the latest patch release of a supported line. Then check which `grafana/alerting` version it pins:

```bash
git fetch --no-tags https://github.com/grafana/grafana.git "+refs/tags/<new>:refs/tags/<new>"
git show <new>:go.mod | grep 'grafana/alerting '
```

If `.groundcover/deps.txt` already has an `alerting <that version> …` line, skip to step 3.

## 2. Release the alerting fork for the new pin

`groundcover-com/alerting` carries one patch, `Mutes()` (active silence IDs for a label set), on top of each upstream alerting version.

1. Create a base branch `grafana-v<line>.x` at the upstream alerting commit the tag pins.
2. Cherry-pick the `Mutes` commit onto a branch `gc/grafana-v<line>.x` and open a PR into the base branch, never into `main` (`main` is a stale upstream mirror). The diff must be `Mutes` only.
3. Merge, then tag the merge commit with the next `v0.0.N` and push the tag. Never move or re-create a released tag: the Go checksum database caches it.
4. Add `alerting <upstream pseudo-version> v0.0.N` to `deps.txt`. The carry stops if the mapping is missing.

## 3. Run the carry

From the current stack branch (`gc/<old>`):

```bash
.groundcover/carry.sh <new>
```

or run the **carry-upstream** workflow (`tag`, `from`). The workflow pushes upstream history, which touches `.github/workflows`, so it needs a `CARRY_TOKEN` secret with `contents` and `workflows` write.

The script cherry-picks the stack onto the tag as `gc/<new>`. It drops backports whose `Upstream-Commit:` trailer the tag already contains, and auto-resolves conflicts in deleted upstream workflows and generated toggle files. `finish` then regenerates the `gc(carry)` commit (upstream workflows removed, `deps.txt` applied, `go mod tidy` + `go work sync`, generated feature toggles), and builds and tests. Go commands run with the toolchain from `go.mod` (`GOTOOLCHAIN`), so a newer local Go does not change the result.

On a conflict it stops with exit 2 (the workflow pushes `gc/<new>-wip`). Resolve, `git add`, `git cherry-pick --continue`, then:

```bash
.groundcover/carry.sh --finish <new>
```

`rerere` is enabled, so resolutions are remembered across attempts.

## 4. Known conflict hotspots

Keep the stack rules while resolving: logic stays in `gc_*` files, upstream files get one-line hooks, and fixes are amended into their feature commit.

| Area | What moved in 12.4 | Where the hook lives now |
|---|---|---|
| State history Loki client | moved to `grafana/alerting/notify/historian/lokiclient`; settings moved to `LokiSettings` | `historian.ApplyGCSettings(backend, cfg, met)` after `NewRemoteLokiBackend` in `ngalert.go` |
| Silence lookup | — | `historian.AttachMuteChecker(history, ng.MultiOrgAlertmanager)` in `ngalert.go` |
| Instance store | state persister init was restructured | `disableInstanceStore` check before `initStatePersister`; the toggle needs `Expression: "false"` |
| Org switcher | `TopBar/TopSearchBar.tsx` → `MegaMenu/MegaMenuHeader.tsx` | same one-line hide |
| New-rule defaults (NoData/Error → OK) | `utils/rule-form.ts` → `rule-editor/formDefaults.ts` | same two values |
| Historian tests | `TestMerge` expectations became raw JSON lines | update expectations, not behaviour |

When a hook target disappears, find the new home of the upstream logic first, then re-attach the hook there. Don't re-create upstream code inside the hook.

## 5. Dockerfile

The `gc(build)` commit takes upstream's Dockerfile and adds the `groundcover` stage. After a carry, check:

- `ARG GF_VERSION=<new>` is set, and `groundcover` is the **last** stage, so a build without `--target` produces it.
- `--platform=${JS_PLATFORM}` is on `go-builder-base` itself. `--platform` on a `FROM` that names another stage is ignored, and the arm64 Go build then runs under QEMU (hours instead of minutes).
- The Go build sets `GOARCH=${TARGETARCH}` and copies `bin/linux/${TARGETARCH}/grafana*` to `bin/`.
- The `groundcover` stage declares `ARG GF_UID="472"`. Without it, `USER "$GF_UID"` expands to nothing and the image runs as root.
- The plugin zips for both architectures are present at the repo root.

## 6. Tests

`finish` covers the Go side. For the frontend, run what CI runs, with Node from `.nvmrc`:

```bash
yarn install --immutable
base=$(git log --format=%H --grep='^gc(build):' -1)
npx jest --ci --passWithNoTests --changedSince="${base}^"
```

- Never pass `--testPathIgnorePatterns` on the command line. It replaces the config's ignore list and pulls the decoupled datasource plugins into the root run.
- The NoData/Error → OK defaults change alerting snapshots and `getPayloadToExport.test.ts`. Update them (`-u`) in the UI-defaults commit after checking that the diff is only `exec_err_state` / `no_data_state`.
- Some upstream suites time out under heavy parallel load (`RuleEditorCloudRules`, `SimplifiedRuleEditor`, `ProvisioningWizard`). Rerun them alone before treating a failure as real.

## 7. Review PR

`gc/<new>` shares no history with `groundcover-main`, so it can't be PR'd directly. After pushing `gc/<new>`:

```bash
.groundcover/carry.sh --review
```

This builds `gc/<new>-review`: `groundcover-main`, then one commit with the upstream tag's tree, then the stack replayed. It has the same tree as `gc/<new>` and prints a compare link with only the fork changes. Open a PR from it into `groundcover-main`, and rerun after every push to `gc/<new>`. Never commit on `-review`. Merge with a merge or rebase merge, not squash, so the `gc(*)` commits stay separate.

## 8. Validate on a dev environment

Build the multi-arch image (`--target groundcover`, `linux/amd64,linux/arm64`), deploy it to a dev install for both monitors-manager and the embedded Grafana, and check:

- **State history:** after the rollout, every row has a summary, eval duration, threshold values and the `_gc_query` label, at the same rates as before. `log_all` gives one row per evaluation.
- **Silences:** create a silence, then check that matching state rows carry its ID. This exercises the alerting fork end to end.
- **No data:** a monitor whose query returns nothing produces `Normal` / reason `NoData` with empty values.
- **Fingerprints:** the "Investigate" link from a workflow notification opens the right issue, and label keys for a monitor are unchanged from the previous version.
- **Theatre:** org switcher hidden, profile shows no orgs, side menu closed, new rules default to Normal, variable "All" is `.*`. An org Admin can't create teams or edit the org; an Editor can provision alert rules.
- **Instance store:** monitors-manager writes nothing to `alert_instance`.
- **arm64:** the image boots and reports healthy on an arm64 node.

## 9. Release

1. Merge the review PR.
2. Tag `groundcover-main` with the next `v0.0.N`. `release.yaml` publishes `grafana-groundcover:v0.0.N-grafana<new>` for both architectures.
3. Bump both image tags in the groundcover Helm values. Remove feature toggles the new version no longer has (12.4 dropped the `alertStateHistoryLoki*` toggles).
4. After the production rollout, check that an issue that was open before the upgrade keeps its ID.
