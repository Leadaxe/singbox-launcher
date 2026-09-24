package core

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"singbox-launcher/core/build"
	"singbox-launcher/core/config"
	"singbox-launcher/core/events"
	"singbox-launcher/core/services"
	"singbox-launcher/core/state"
	"singbox-launcher/core/template"
	"singbox-launcher/internal/constants"
	"singbox-launcher/internal/debuglog"
	"singbox-launcher/internal/dialogs"
	"singbox-launcher/internal/locale"
	"singbox-launcher/internal/paths"
	"singbox-launcher/internal/platform"
)

// validateConfigViaSingBox runs `sing-box check -c <configPath>` with a short
// timeout and returns nil if config is valid, error with sing-box stderr
// otherwise. If sing-box binary не существует / нечитаем → nil (graceful skip,
// чтобы старые установки без bundled binary не падали).
func validateConfigViaSingBox(singboxPath, configPath string) error {
	if singboxPath == "" {
		return nil
	}
	if _, err := os.Stat(singboxPath); err != nil {
		return nil // graceful skip
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, singboxPath, "check", "-c", configPath)
	platform.PrepareCommand(cmd)
	out, err := cmd.CombinedOutput()
	if err == nil {
		return nil
	}
	// Strip ANSI color codes from sing-box output.
	msg := stripANSI(string(out))
	msg = strings.TrimSpace(msg)
	if msg == "" {
		msg = err.Error()
	}
	// Cap output length для popup display.
	if len(msg) > 1500 {
		msg = msg[:1500] + "\n... (truncated, see sing-box.log)"
	}
	return fmt.Errorf("%s", msg)
}

// stripANSI removes ANSI escape sequences (ESC [ ... m) from output.
func stripANSI(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	i := 0
	for i < len(s) {
		if s[i] == 0x1b && i+1 < len(s) && s[i+1] == '[' {
			// Skip until terminator letter.
			i += 2
			for i < len(s) && (s[i] < 0x40 || s[i] > 0x7e) {
				i++
			}
			if i < len(s) {
				i++
			}
			continue
		}
		b.WriteByte(s[i])
		i++
	}
	return b.String()
}

// RebuildConfigIfDirty — **единственный writer `config.json`** (SPEC 045 invariant).
//
// Pipeline (SPEC 052):
//
//	state.json + bin/subscriptions/<id>.raw (per-source) + template
//	  → in-memory parse → outboundscache.Snapshot
//	  → core/build.BuildConfig
//	  → atomic write config.json
//	  → ClearConfigStale + ConfigBuilt event
//
// Auto-Update fallback: если хоть одна enabled subscription без `.raw` —
// сначала зовёт `ConfigService.UpdateConfigFromSubscriptions` (network),
// затем продолжает.
//
// No-op условие: оба dirty-маркера чисты И полный raw cache на диске
// (skipped when forced=true — UI кнопка Rebuild всегда полностью пересобирает).
//
// Возвращает:
//   - nil — успех (или nothing-to-do);
//   - error — fatal на этапе сборки/записи.
func (ac *AppController) RebuildConfigIfDirty(forced ...bool) error {
	isForced := len(forced) > 0 && forced[0]
	if ac == nil || ac.StateService == nil {
		return nil
	}
	if ac.FileService == nil {
		return fmt.Errorf("FileService not initialized")
	}
	layout := ac.FileService.Layout

	// Шаблон после апгрейда докачивается в фоне (StartTemplateRefresh). Сборка
	// из старого шаблона, пока новый в пути, дала бы ровно тот config.json,
	// ради замены которого шаблон и качается, — ждём.
	ac.awaitTemplateRefresh()

	// One-time legacy cleanup: bin/outbounds.cache.json больше не используется.
	cleanupLegacyOutboundsCache(layout.Data)

	// Step 1: load state.
	statePath := platform.GetWizardStatePath(layout.Data)
	s, err := state.Load(statePath)
	if err != nil {
		return fmt.Errorf("load state: %w", err)
	}

	// Step 1.5: load template — нужен раньше (SPEC 056) для preset.outbounds
	// pre-patch внутри buildSnapshotFromState. Это лёгкая операция (file
	// read + JSON parse), переиспользуется в Step 4 для BuildConfig.
	// Отсутствующий файл сначала скачивается (loadTemplateForBuild).
	td, templateFetched, err := ac.loadTemplateForBuild(layout)
	if err != nil {
		return fmt.Errorf("load template: %w", err)
	}

	// Step 2: попытаться построить snapshot из материализованных узлов.
	cacheSnap, parserRes, snapErr := buildSnapshotFromState(s, layout, nil, td)
	cacheMissing := errors.Is(snapErr, ErrNoMaterializedNodes)
	if snapErr != nil && !cacheMissing {
		// Сборка не состоялась — но если разбор успел объяснить, почему
		// (все источники пусты, провайдер ответил отказом), это объяснение
		// обязано доехать до списка источников. Молчаливый выход оставлял бы
		// сломанную подписку с виду здоровой.
		feedParserDiagnosticsOnFailure(parserRes)
		return fmt.Errorf("build snapshot from materialized nodes: %w", snapErr)
	}

	if cacheMissing {
		debuglog.InfoLog("RebuildConfigIfDirty: subscriptions have no materialized nodes — run Update first")
		if ac.ConfigService == nil {
			return fmt.Errorf("no materialized nodes and ConfigService not initialized")
		}
		// triggerRebuild=false: мы УЖЕ внутри Rebuild — хвостовой rebuild
		// из Update замыкал бы взаимную рекурсию (см. updateConfigFromSubscriptions).
		if _, updErr := ac.ConfigService.updateConfigFromSubscriptions(false); updErr != nil {
			return fmt.Errorf("auto-update for empty node set failed: %w", updErr)
		}
		// Перечитываем state (Update сохраняет meta) и снова строим snapshot.
		s, err = state.Load(statePath)
		if err != nil {
			return fmt.Errorf("reload state after auto-update: %w", err)
		}
		cacheSnap, parserRes, snapErr = buildSnapshotFromState(s, layout, nil, td)
		if snapErr != nil {
			feedParserDiagnosticsOnFailure(parserRes)
			return fmt.Errorf("rebuild snapshot after auto-update: %w", snapErr)
		}
	}

	// Step 3: noop fast-path (skipped when forced=true — user explicitly
	// pressed Rebuild button и ожидает полный rebuild + sing-box check
	// даже если dirty markers чистые). Только что скачанный шаблон — тоже
	// повод собрать: config.json на диске собран без него.
	if !isForced && !cacheMissing && !templateFetched && !ac.StateService.IsCacheStale() && !ac.StateService.IsConfigStale() {
		return nil
	}

	debuglog.InfoLog("RebuildConfigIfDirty: rebuilding config.json (forced=%v update_dirty=%v restart_dirty=%v cache_missing_initially=%v)",
		isForced, ac.StateService.IsCacheStale(), ac.StateService.IsConfigStale(), cacheMissing)

	// SPEC 112-B часть B / SPEC 115: попытка сборки открывается ЗДЕСЬ, за
	// noop-развилкой, а не в эмиссии выше. Разбор идёт до развилки, и
	// открытая там попытка на холостом вызове (dirty-маркеры чисты, кэш на
	// месте) осталась бы без санитайзерных записей и без Finish: холостой
	// Rebuild стирал бы пометки «снято N» прошлой полной сборки, не дав взамен
	// ничего. Начиная попытку тут, мы начинаем её ровно тогда, когда сборка
	// действительно идёт.
	//
	// Записи парсерной стадии кладутся сразу, включая ПУСТОЙ итог: чистая
	// сборка обязана снять прежние ⚠, иначе пометка переживёт свою причину.
	gen := config.StartBuildReport()
	FeedBuildReportFromParser(gen, parserRes)
	// SPEC 118 W4: деградации последнего fetch — из состояния, не из разбора
	// (тела сборка не читает). Кладутся в ту же попытку: пользователю нужен
	// один список причин, а не два по стадиям конвейера.
	FeedBuildReportFromFetchStatus(gen, s.Sources)

	// Step 4: build.
	ctx := ac.buildContextFromState(s, cacheSnap, td)
	res, err := build.BuildConfig(ctx)
	if err != nil {
		return fmt.Errorf("build: %w", err)
	}

	// SPEC 113-B: последний рубеж мог выбросить узлы источника за недоступную
	// цель detour (исчез селектор шаблона — выключили мульти-VPN). Парсер про
	// это не знал, поэтому записи доливаются в отчёт поверх его собственных.
	FeedBuildReportFromSanitizer(gen, res.ExcludedSources)

	// Parser-stage warnings (e.g. naive nodes degraded on a core without
	// naive support, SPEC 044 feature-probe) ride along with the build
	// validation warnings into the ConfigBuilt event.
	if cacheSnap != nil && len(cacheSnap.Warnings) > 0 {
		for _, w := range cacheSnap.Warnings {
			debuglog.WarnLog("RebuildConfigIfDirty: %s", w)
		}
		res.Validation.Warnings = append(res.Validation.Warnings, cacheSnap.Warnings...)
	}

	// Step 5: кандидат → check → замена (SPEC 132 §5А).
	//
	// Инвариант: в config.json попадает ТОЛЬКО конфиг, принятый ядром.
	// Раньше файл писался первым, а проверялся после, и битый конфиг
	// оставался на диске — ядро могло стартовать на заведомо отвергнутом.
	//
	// Круг отказа, назвавшего НАШ узел, выключает этот узел в состоянии и
	// пересобирает конфиг — и так до чистого прохода либо до стоп-условия
	// (CANON §9.5). Всё это живёт в ОБЩЕЙ функции сборки, поэтому цикл
	// достаётся всем входам сразу: pre-start обоих движков, кнопка Rebuild,
	// автообновление подписок, API /action/rebuild-config.
	disabler := &savedStateDisabler{s: s, path: statePath}
	loop := &coreRejectLoop{
		check:      coreRejectCheck,
		singbox:    ac.FileService.SingboxPath,
		configPath: ac.FileService.ConfigPath,
		disabler:   disabler,
		decide:     ac.coreRejectDecider(),
		progress:   ac.coreRejectProgress(),
	}
	first := buildRound{ConfigJSON: res.ConfigJSON, NodeLinks: stateNodeLinks(parserRes)}
	// Круг цикла: конфиг пересобирается из ТОГО ЖЕ состояния в памяти, в
	// котором страховка уже выключила узел. Отчёт сборки не переоткрывается —
	// записи круга те же, меняется только состав.
	rebuildRound := func() (buildRound, error) {
		snap, pres, rerr := buildSnapshotFromState(s, layout, nil, td)
		if rerr != nil {
			return buildRound{}, fmt.Errorf("rebuild after disabling a node: %w", rerr)
		}
		rctx := ac.buildContextFromState(s, snap, td)
		rres, rerr := build.BuildConfig(rctx)
		if rerr != nil {
			return buildRound{}, fmt.Errorf("rebuild after disabling a node: %w", rerr)
		}
		res = rres // итог последней сборки — он и уедет на диск
		return buildRound{ConfigJSON: rres.ConfigJSON, NodeLinks: stateNodeLinks(pres)}, nil
	}
	outcome, loopErr := loop.run(first, rebuildRound)
	if loopErr != nil {
		return fmt.Errorf("write config: %w", loopErr)
	}
	// Выключения закрепляются на диске ОДИН раз, независимо от исхода:
	// нажатый Stop и оборвавшийся цикл выключенные узлы не возвращают
	// (решение владельца, §6.2 SPEC 132).
	if err := disabler.Commit(); err != nil {
		debuglog.ErrorLog("RebuildConfigIfDirty: disabled nodes not saved to state.json: %v", err)
	}
	// Проекции состояния (индекс предупреждений узлов, разбор состава)
	// кэшируются по mtime/размеру state.json — Commit выше их и снял.

	configValid := outcome.Promoted
	if !configValid {
		checkErr := outcome.CheckErr
		debuglog.ErrorLog("RebuildConfigIfDirty: sing-box check failed: %v", checkErr)
		if ac.UIService != nil && ac.UIService.MainWindow != nil {
			dialogs.ShowErrorText(ac.UIService.MainWindow,
				locale.T("Config validation failed"),
				// Текст переписан вместе с §5А: «Connect won't work until
				// this is fixed» стало бы прямой неправдой — config.json НЕ
				// заменён, на диске лежит предыдущий рабочий конфиг, и
				// Connect как раз будет работать, на нём.
				fmt.Sprintf("sing-box rejected the newly built config:\n\n%v\n\nconfig.json was NOT replaced — the previous working config is still on disk. See logs for details.", checkErr))
		}
		if ac.EventBus != nil {
			ac.EventBus.Publish(events.Event{
				Kind: events.ConfigBuilt,
				Payload: events.ConfigBuiltPayload{
					OK:            false,
					Warnings:      []string{fmt.Sprintf("sing-box check: %v", checkErr)},
					DisabledNodes: coreRejectedPayload(outcome.Disabled),
				},
			})
		}
		// config.json не заменён: ConfigStale остаётся, чтобы следующий
		// rebuild перепроверил. Без return — поток продолжается, как и раньше.
	} else {
		// Имя собственного TUN могло смениться этой пересборкой. Реестр
		// netiface обязан догнать её сразу: по нему пикер аплинков прячет наш
		// TUN, а всё прочее туннельное — теперь законный выбор (SPEC 113-F).
		//
		// СТРОГО после реальной замены (SPEC 132 §5Б): по конфигу, который
		// ядро ещё не приняло, реестр начал бы прятать из пикера имя, которого
		// не существует.
		ac.refreshOwnTunNames()

		// SPEC 135 §3.5: config.json на диске теперь собран от этого DataDir
		// (абсолютные пути .srs и tailscale). Единственная точка успешной
		// записи локального config.json — здесь.
		stampConfigDataRoot(layout)
	}

	// Step 5.5: orphan GC для bin/rule-sets/. Параллельно тому что
	// refreshSubscriptionsMetaAndCache делает для bin/subscriptions/.
	// Live tags = union из всех stages ЛОКАЛЬНОЙ машины (multi-stage safety).
	// Удаляем .srs файлы которые уже не упоминаются ни одним её stage'ом.
	//
	// SPEC 098: этот путь — только про локальную машину. Каталоги .srs
	// удалённых машин лежат в их директориях и чистятся своим GC; трогать их
	// отсюда значило бы удалить файл, живой для другой машины.
	//
	// СТРОГО после реальной замены (SPEC 132 §5Б): GC по кандидату удалил бы
	// `.srs`, на которые ссылается ещё живой ПРЕДЫДУЩИЙ config.json, и откат
	// на него оставил бы битые ссылки.
	if configValid {
		knownTags := collectAllStageRuleSetTags(layout.Data, constants.ConfigTargetLocal, "", td)
		if deleted, gcErr := services.DeleteOrphanRuleSets(layout.Data, knownTags); gcErr != nil {
			debuglog.WarnLog("RebuildConfigIfDirty: DeleteOrphanRuleSets: %v", gcErr)
		} else if len(deleted) > 0 {
			debuglog.InfoLog("RebuildConfigIfDirty: GC removed %d orphan rule-set file(s): %v", len(deleted), deleted)
		}
	}

	// Step 6: clear ConfigStale ТОЛЬКО если sing-box принял config (свеж И
	// валиден). Если rejected — ConfigStale остаётся, чтобы следующий rebuild
	// перепроверил. CacheStale НЕ трогаем: rebuild не делал network fetch (при
	// cacheMissing Update уже его сбросил; иначе CacheStale остаётся как был).
	if configValid {
		// SPEC 115 (фикс-раунд): «отчёт готов» ставится ТОЛЬКО здесь — после
		// того как конфиг записан и принят sing-box'ом. Раньше признак стоял
		// сразу за BuildConfig, и сборка, отвергнутая валидацией, объявляла свой
		// отчёт готовым: гейт Save открывался на конфиге, который ядро не
		// возьмёт. Упавшая сборка Finish не зовёт вовсе, и попытка остаётся
		// незавершённой — записи в отчёте есть (их видно), но готовым он не
		// считается.
		config.FinishBuildReport(gen)
		ac.StateService.ClearConfigStale()
		if ac.EventBus != nil {
			ac.EventBus.Publish(events.Event{
				Kind: events.ConfigBuilt,
				Payload: events.ConfigBuiltPayload{
					OK:       true,
					Warnings: res.Validation.Warnings,
					// SPEC 132: список выключенных страховкой едет в событии,
					// чтобы плашка главного экрана (волна UI) показала его
					// после успешного старта.
					DisabledNodes: coreRejectedPayload(outcome.Disabled),
				},
			})
		}
	}

	// Step 7: refresh UI markers.
	// SPEC 047 phase 6 (SPEC 070): config-status refresh теперь приходит через
	// events.ConfigBuilt (опубликован в Step 5.4 OK:false / Step 6 OK:true) —
	// dashboard-подписчик зовёт updateConfigInfo. Поэтому прямой вызов
	// UpdateConfigStatusFunc здесь убран. UpdateCoreStatusFunc оставлен
	// (VpnState-канал, вне scope этого шага).
	if ac.UIService != nil {
		if ac.UIService.UpdateCoreStatusFunc != nil {
			ac.UIService.UpdateCoreStatusFunc()
		}
	}

	// SPEC 115 §3: тоста об исключённых источниках здесь больше НЕТ (решение
	// пользователя). Тост исчезает через несколько секунд, а список
	// исключений на конфиге с десятком зависимых подписок в одну строку не
	// помещался — то есть ровно тот случай, ради которого сообщение и
	// показывали, пользователь дочитать не успевал. Роль забрали отчёт
	// вкладки «Итог» (полный, со скроллом и копированием) и стойкие пометки в
	// строках Wizard → Sources, которые живут, пока живёт причина.

	debuglog.InfoLog("RebuildConfigIfDirty: config.json written (%d bytes)", len(res.ConfigJSON))
	return nil
}

// loadTemplateForBuild reads the template for a build. A MISSING file is
// downloaded first — the same template.EnsureTemplate the wizard uses: after a
// startup refresh that failed (launched offline, network up by now) a start
// would otherwise stop at "no such file" although one request fixes it.
//
// A file that exists but does not parse is NOT replaced: it may be the user's
// own edit, and the returned error names the problem.
//
// fetched=true means the template was just downloaded, so config.json on disk
// was built without it.
func (ac *AppController) loadTemplateForBuild(l paths.Layout) (td *template.TemplateData, fetched bool, err error) {
	td, err = template.LoadTemplateData(l)
	if err == nil || !errors.Is(err, os.ErrNotExist) {
		return td, false, err
	}
	debuglog.WarnLog("RebuildConfigIfDirty: %s is missing — downloading it before the build", template.GetTemplateFileName())
	ctx, cancel := context.WithTimeout(context.Background(), template.DownloadTimeout)
	defer cancel()
	td, _, err = template.EnsureTemplate(ctx, l, ac.GetURLBytes)
	if err != nil {
		return nil, false, err
	}
	// Вкладка Local прячет Configurator и показывает Download, пока файла
	// нет, — после докачки ей надо перечитать.
	if ac.UIService != nil && ac.UIService.UpdateConfigStatusFunc != nil {
		ac.UIService.UpdateConfigStatusFunc()
	}
	return td, true, nil
}

// rebuildConfigBeforeStart — pre-start hook of both engines (classic
// ProcessService.Start, daemon applyCurrentConfig). A non-nil error means the
// core must NOT be started, and the caller shows it (ShowRebuildError).
//
// Before, the error went to the log and the core came up on whatever
// config.json an earlier build had left — settings silently not applied, the
// way a launcher upgrade that lost its template went unnoticed.
//
// No state.json is not an error: config.json is then managed by hand and there
// is nothing to rebuild it from.
func (ac *AppController) rebuildConfigBeforeStart(forced bool) error {
	err := ac.RebuildConfigIfDirty(forced)
	if errors.Is(err, state.ErrNotFound) {
		debuglog.InfoLog("pre-start rebuild: no state.json — config.json is used as is")
		return nil
	}
	return err
}

// CleanOrphanRuleSets removes bin/rule-sets/*.srs files not referenced by any
// saved LOCAL wizard state — the same multi-stage live-set the rebuild GC
// (Step 5.5) uses. Returns the removed filenames.
//
// SPEC 098: local-scoped. Remote machines keep their .srs under their own
// directory and garbage-collect within it, so this call can neither delete
// nor retain their files.
//
// Used by the manual "clean unused rule-sets" action and the state-delete path
// (deleting a saved state frees the .srs only that state referenced). Multi-stage
// semantics are intentional: an .srs stays while ANY saved state still uses it.
//
// Conservative on template-load failure: returns the error WITHOUT deleting, so a
// transient template read can never wipe still-referenced preset .srs files.
func (ac *AppController) CleanOrphanRuleSets() ([]string, error) {
	if ac == nil || ac.FileService == nil {
		return nil, fmt.Errorf("CleanOrphanRuleSets: controller not initialized")
	}
	layout := ac.FileService.Layout
	td, err := template.LoadTemplateData(layout)
	if err != nil {
		return nil, fmt.Errorf("CleanOrphanRuleSets: load template: %w", err)
	}
	known := collectAllStageRuleSetTags(layout.Data, constants.ConfigTargetLocal, "", td)
	return services.DeleteOrphanRuleSets(layout.Data, known)
}

// cleanupLegacyOutboundsCache удаляет `bin/outbounds.cache.json`, если он
// существует (legacy SPEC 045 cache, выпиленный в SPEC 052). One-shot:
// файл не пересоздаётся новым кодом, поэтому достаточно удалить однажды
// и забыть. Best-effort — ошибки не критичны.
func cleanupLegacyOutboundsCache(d paths.DataDir) {
	path := platform.GetOutboundsCachePath(d)
	if _, err := os.Stat(path); err == nil {
		if remErr := os.Remove(path); remErr == nil {
			debuglog.InfoLog("cleanupLegacyOutboundsCache: removed legacy %s", path)
		} else {
			debuglog.WarnLog("cleanupLegacyOutboundsCache: failed to remove %s: %v", path, remErr)
		}
	}
}
