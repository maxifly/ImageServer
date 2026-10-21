
## Task Plans

Use repository-root `tasks/` Markdown files for implementation plans and working checklists. Treat `tasks/` as local working state that is intentionally ignored by Git.
Write task-plan documents in Russian.

- When the user asks to continue or implement planned work, read `AGENTS.md` and the referenced `tasks/` plan before changing code.
- If the user does not name a plan and exactly one `tasks/` plan clearly matches the request, use that plan. Otherwise, ask which plan to use.
- Treat the plan as the source of truth for agreed scope, requirements, technical decisions, task order, acceptance criteria, and progress. A direct user instruction overrides the plan; record the resulting decision or scope change in the plan.
- Keep user requirements and acceptance criteria separate from implementation decisions. Do not weaken, reinterpret, or change a requirement merely to fit the chosen implementation.
- Before starting implementation of a new plan, verify that:
  - all agreed requirements are represented by acceptance criteria;
  - the planned subtasks collectively cover those acceptance criteria;
  - task ordering and dependencies are consistent;
  - technical decisions do not contradict requirements;
  - there are no material gaps or unresolved contradictions.
    Record any material gap, contradiction, assumption, or required decision in the plan before changing code.

- By default, work on the first unchecked subtask. Do not start later dependent subtasks unless the plan or the user explicitly allows it.
- Before starting a subtask, set the plan's current-subtask field and mark that subtask as in progress without checking it off.
- While implementing a subtask, keep changes within its agreed scope. If newly discovered work is required:
  - add it to the current subtask if it is necessary to satisfy that subtask's acceptance criteria and does not materially change scope;
  - otherwise add a new unchecked subtask in the appropriate dependency order;
  - record material scope or design changes in the plan before proceeding.

- After finishing a subtask, update the same plan in the same turn:
  - check off the subtask only when its acceptance criteria are satisfied;
  - update the overall status and last-updated date;
  - append a concise journal entry with changed files, validation performed, skipped validation or required approvals, relevant decisions, discovered follow-up work, and the next subtask.

- Do not mark a subtask complete when required work remains, validation failed, a required confirmation was declined, or acceptance criteria have not been demonstrated. Record the exact remaining work or blocker instead.
- Preserve completed plan history and user decisions. Do not delete, replace, or substantially rewrite an existing plan unless the user asks for that change.
- When all subtasks are checked, perform a final plan-level convergence review before marking the plan complete:
  - compare the resulting implementation against the complete agreed scope, requirements, and acceptance criteria;
  - verify that no requirement was lost or unintentionally narrowed during implementation;
  - verify that all material technical decisions recorded in the plan are reflected consistently in the implementation;
  - check for unfinished dependent work, compatibility changes, migrations, documentation updates, or other required follow-up work identified by the plan or implementation.
    If anything remains, add or reopen the necessary unchecked subtasks instead of marking the plan complete.

- Mark the overall plan complete only after the final convergence review finds no remaining required work.
- A task-plan checklist entry never grants permission to create tests, run builds or tests, launch simulators, access external systems, or take any other action that still requires explicit user approval.


<!-- graft:start -->
## Graft — repo context graph

This repo is indexed in `graft/`: small linked markdown nodes that explain each
system and carry exact file:line spans, kept in sync with the code through git.

For ANY task here — understanding how something works, finding where code lives,
or scoping a change — get context from the graph before grepping or opening
source files. Re-ask freely (it's cheap) and reuse literal identifiers you
already have (symbol, error string, file name) as the query. New to this repo?
Run `graft map` first — a token-budgeted orientation (dir clusters, hubs,
hotspots), no LLM, no key.

- Run `graft ask "<your question>" --source` → ranked nodes with the relevant
  code spans inlined (each hit's ≤8-line crux by default; `--full` for whole
  definitions when the crux isn't enough). Match the tool to the task shape:
  for understanding or editing, the top node IS the answer — cite its
  `covers:` file:line spans and edit straight from `--source`. For
  exhaustive tasks ("every occurrence / every caller of this pattern"), ranked
  results are top-N, not complete — run `graft grep "<literal>"` instead
  (exhaustive over indexed files, grouped by enclosing symbol), falling back
  to raw `grep -rn` only for unindexed files.
- `graft skeleton <file>` → every definition's signature + span, ~10× cheaper
  than reading the file; use it to skim an API surface.
- `graft callers <symbol>` gives precomputed, exact edges — who calls this.
  Add `--direction out` for what it calls, or `--depth N` to walk
  transitively for the full blast radius. For structural questions, skip
  ranking and use this directly.
- Or browse: `graft/INDEX.md` lists every node; follow the links.
- Monorepos and folders of multiple repos rank fairly across sub-projects —
  hits carry `[scope/]` labels naming which one they're from. Narrow with
  `graft ask "<task>" --in <scope>/` once you know where you're working.

If a returned span is truncated ("+N more lines"), open the file at that exact
range before finalizing. Only open source files when a node genuinely lacks a
needed detail, and then at the exact file:line the node points to — never
re-read whole files.

After big code changes, refresh the graph with `graft build` (deterministic,
no API key, $0).
<!-- graft:end -->

## Ответы и оценка экономии токенов

В конце каждого ответа в этом чате указывай приблизительную оценку сэкономленных
токенов благодаря использованию контекста Graft. Используй формат:

`🌿 Сэкономлено примерно: ~N токенов`

Оценка приблизительная; если экономия не поддаётся разумной оценке, укажи это
явно вместо выдуманного точного значения.

Если Graft сам добавляет англоязычную строку с оценкой экономии, не дублируй её в ответе. Объединяй информацию в одну русскоязычную строку с символом `🌿`.
