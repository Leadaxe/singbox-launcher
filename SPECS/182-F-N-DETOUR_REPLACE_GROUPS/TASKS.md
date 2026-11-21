# TASKS — SPEC 182

- [x] `ui/configurator/business/replace_group_targets.go` (новый) — `ReplaceGroupTargets`, `LinkHolder`, поиск кольца по рёбрам модели.
- [x] `ui/configurator/business/detour.go` — `DetourKindReplaceGroup`, группы в опциях, `DetourChoice.Loop`, ссылка на двойник не «пропавшая».
- [x] `ui/configurator/tabs/detour_tree_picker.go` — ветка «Replace groups», строка с кольцом приглушена и не выбирается.
- [x] `ui/configurator/tabs/source_chain_hops.go`, `source_chain_tab.go` — группы из общего списка, двойник не предлагается, кольцо с ⚠ и отказом.
- [x] `bin/locale/ru.json` — три новых ключа.
- [x] Тест `TestDetourPickerOffersReplaceGroups` (`ui/configurator/business`); правка ожидания `TestChainHopCandidatesFoldedFolderOffersOnlyReplaceTags`.
- [x] `docs/release_notes/upcoming.md`.
