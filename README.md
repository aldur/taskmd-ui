# taskmd-ui

A full-screen terminal browser for [taskmd](https://github.com/driangle/taskmd)
projects: a task list you filter and re-sort a keypress at a time, next to the
detail of whatever is under the cursor.

taskmd ships a CLI and a web dashboard. It had a TUI once and
[removed it](https://github.com/driangle/taskmd/blob/main/tasks/archive/cli/074-remove-tui-feature.md)
in v0.3.0 as redundant with those two. This is the terminal view that left
behind.

```
taskmd /home/you/project/tasks                                      9/29 tasks
/status!=done priority>=high  o:open f:status p:prio t:tag h:phase   sort:priority↑
╭──────────────────────────────────────────────╮╭──────────────────────────────╮
│▸ ○ 050  v0.3 release checklist      critical ││050  v0.3 release checklist   │
│  ◐ 002  Segment compaction runs be… critical ││──────────────────────────────│
│  ○ 003  Checksum mismatch panics i… critical ││status    pending             │
│  ⊘ 032  Nightly job silently stopp… critical ││priority  critical            │
│  ◐ 011  orbit query: filters, proj… high     ││owner     aldur               │
╰──────────────────────────────────────────────╯╰──────────────────────────────╯
/ filter · o open · s sort · space advance · e edit · c clear · ? help · q quit
```

## Install

```sh
go install github.com/aldur/taskmd-ui@latest
```

`taskmd` itself needs to be on `PATH`: status changes are written by
`taskmd set` rather than by editing frontmatter here, so its rules about what a
transition does stay in one place.

## Use

Run it inside a taskmd project. The task directory is discovered the way the
CLI discovers it — the `dir` key of the nearest `.taskmd.yaml` above the
working directory — or pass `-d DIR` to point it somewhere else, including
into a project you are not standing in.

The list reloads every two seconds, so edits made anywhere else — `taskmd set`
in another pane, an editor, an agent — show up on their own. The cursor stays
on the same task across a reload, and `r` forces one.

Below 120 columns, the list and details each use the full width. Press `tab`
to switch views, and `esc` to return to the list. Details and help
wrap to the available width; smaller screens keep active filters, task counts,
the sort order and navigation hints ahead of optional columns and shortcuts.

Press `enter` to read the selected task full screen at any terminal width.
This view hides the browser header, filters and borders. It is read-only:
`e` and `1`–`6` are disabled, and `space` scrolls down instead of changing
status. Use `j`/`k` to scroll, `g`/`G` for the top or bottom, and `esc` to
return to the same task and preview position. External edits still reload.

### Filtering

Single keys cycle a facet: `o` toggles open-only, and `f p t h w n y` cycle
status, priority, tag, phase, owner, group and type (uppercase cycles
backwards). Every facet is spelled out in the header, so the keys are
discoverable without the help screen.

`/` opens a filter expression, applied live as you type:

| Term | Matches |
| --- | --- |
| `parser` | free text over id, title, tags and body |
| `-parser` | exclude tasks with this text in any searchable field |
| `"record format"` | quoted, so the space is part of the text |
| `status:blocked` | field is this value |
| `status!=completed` | field is not (same as `-status:completed`) |
| `status:open` | `open`, `active`, `done` and `resolved` name a group of statuses |
| `tag:a11y,ux` | either value |
| `priority>=high` | at least this urgent (`effort` orders by size) |
| `created<2026-06-01` | dates compare as dates |
| `owner:none` | field is unset (`owner:any` for the opposite) |

Terms are ANDed, and combine with the facet keys. Fields: `status`,
`priority`, `effort`, `type`, `owner`, `phase`, `group`, `tag`, `dep`,
`touches`, `parent`, `id`, `title`, `file`, `body`, `created`, with short
aliases for the common ones.

Use `-status:completed` to exclude completed tasks. `-completed` excludes
any task that contains that text, including in its title or body.

Press `v` to select statuses. Use `j`/`k` or the arrow keys and space to change the
selection, then press enter to update the query or escape to cancel.

A value a field cannot take is reported rather than silently matching nothing,
and a half-typed expression keeps filtering on the terms that did parse.

### Keys

`?` has the full list. The short version: `j`/`k` move, `tab` switches panes,
`enter` reads the task full screen,
`s` cycles the sort and `S` reverses it, `space` advances a task along the
workflow, `1`–`6` set a status outright, `e` opens the task in `$EDITOR`, `q`
quits.

## Notes

Tasks are read from the markdown directly rather than through `taskmd list
--format json`, which drops `phase` and `owner` — both filter axes here. Only
writes go through the CLI.
