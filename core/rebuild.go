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
	"singbox-launcher/core/events"
	"singbox-launcher/core/services"
	"singbox-launcher/core/state"
	"singbox-launcher/core/template"
	"singbox-launcher/internal/debuglog"
	"singbox-launcher/internal/dialogs"
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
	execDir := ac.FileService.ExecDir

	// One-time legacy cleanup: bin/outbounds.cache.json больше не используется.
	cleanupLegacyOutboundsCache(execDir)

	// Step 1: load state.
	statePath := platform.GetWizardStatePath(execDir)
	s, err := state.Load(statePath)
	if err != nil {
		return fmt.Errorf("load state: %w", err)
	}

	// Step 1.5: load template — нужен раньше (SPEC 056) для preset.outbounds
	// pre-patch внутри buildSnapshotFromRawCache. Это лёгкая операция (file
	// read + JSON parse), переиспользуется в Step 4 для BuildConfig.
	td, err := template.LoadTemplateData(execDir)
	if err != nil {
		return fmt.Errorf("load template: %w", err)
	}

	// Step 2: попытаться построить snapshot из raw cache.
	cacheSnap, snapErr := buildSnapshotFromRawCache(s, execDir, nil, td)
	cacheMissing := errors.Is(snapErr, ErrRawCacheIncomplete)
	if snapErr != nil && !cacheMissing {
		return fmt.Errorf("build snapshot from raw cache: %w", snapErr)
	}

	if cacheMissing {
		debuglog.InfoLog("RebuildConfigIfDirty: raw cache incomplete — triggering Update first")
		if ac.ConfigService == nil {
			return fmt.Errorf("raw cache incomplete and ConfigService not initialized")
		}
		if _, updErr := ac.ConfigService.UpdateConfigFromSubscriptions(); updErr != nil {
			return fmt.Errorf("auto-update for empty raw cache failed: %w", updErr)
		}
		// Перечитываем state (Update сохраняет meta) и снова строим snapshot.
		s, err = state.Load(statePath)
		if err != nil {
			return fmt.Errorf("reload state after auto-update: %w", err)
		}
		cacheSnap, snapErr = buildSnapshotFromRawCache(s, execDir, nil, td)
		if snapErr != nil {
			return fmt.Errorf("rebuild snapshot after auto-update: %w", snapErr)
		}
	}

	// Step 3: noop fast-path (skipped when forced=true — user explicitly
	// pressed Rebuild button и ожидает полный rebuild + sing-box check
	// даже если dirty markers чистые).
	if !isForced && !cacheMissing && !ac.StateService.IsCacheStale() && !ac.StateService.IsConfigStale() {
		return nil
	}

	debuglog.InfoLog("RebuildConfigIfDirty: rebuilding config.json (forced=%v update_dirty=%v restart_dirty=%v cache_missing_initially=%v)",
		isForced, ac.StateService.IsCacheStale(), ac.StateService.IsConfigStale(), cacheMissing)

	// Step 4: build.
	parserCfg := s.ParserConfig
	ctx := ac.buildContextFromState(s, cacheSnap, td, &parserCfg)
	res, err := build.BuildConfig(ctx)
	if err != nil {
		return fmt.Errorf("build: %w", err)
	}

	// Step 5: atomic write.
	if err := atomicWriteConfig(ac.FileService.ConfigPath, res.ConfigJSON); err != nil {
		return fmt.Errorf("write config: %w", err)
	}

	// Step 5.4: sing-box check — валидация только что записанного config.json
	// через сам sing-box (`sing-box check -c config.json`). Catches schema
	// violations (unknown fields, legacy DNS format, type mismatches) ДО того
	// как юзер нажмёт Connect и получит non-obvious "FATAL: ..." в логе.
	// Ошибка → ErrorLog + popup через UIService.
	configValid := true
	if checkErr := validateConfigViaSingBox(ac.FileService.SingboxPath, ac.FileService.ConfigPath); checkErr != nil {
		debuglog.ErrorLog("RebuildConfigIfDirty: sing-box check failed: %v", checkErr)
		if ac.UIService != nil && ac.UIService.MainWindow != nil {
			dialogs.ShowErrorText(ac.UIService.MainWindow,
				"Config validation failed",
				fmt.Sprintf("sing-box rejected the generated config.json:\n\n%v\n\nConnect won't work until this is fixed. See logs for details.", checkErr))
		}
		if ac.EventBus != nil {
			ac.EventBus.Publish(events.Event{
				Kind: events.ConfigBuilt,
				Payload: events.ConfigBuiltPayload{
					OK:       false,
					Warnings: []string{fmt.Sprintf("sing-box check: %v", checkErr)},
				},
			})
		}
		// Config записан, но sing-box его ОТВЕРГ: оставляем ConfigStale,
		// чтобы следующий rebuild перепроверил, а не доверял заведомо битому
		// config.json (и пропускаем ConfigBuilt{OK:true} ниже). Без return —
		// поток продолжается; popup + ConfigBuilt{OK:false} уже отправлены.
		configValid = false
	}

	// Step 5.5: orphan GC для bin/rule-sets/. Параллельно тому что
	// refreshSubscriptionsMetaAndCache делает для bin/subscriptions/.
	// Live tags = union из всех stages (multi-stage safety). Удаляем
	// .srs файлы которые уже не упоминаются ни одним stage'ом.
	knownTags := collectAllStageRuleSetTags(execDir, td)
	if deleted, gcErr := services.DeleteOrphanRuleSets(execDir, knownTags); gcErr != nil {
		debuglog.WarnLog("RebuildConfigIfDirty: DeleteOrphanRuleSets: %v", gcErr)
	} else if len(deleted) > 0 {
		debuglog.InfoLog("RebuildConfigIfDirty: GC removed %d orphan rule-set file(s): %v", len(deleted), deleted)
	}

	// Step 6: clear ConfigStale ТОЛЬКО если sing-box принял config (свеж И
	// валиден). Если rejected — ConfigStale остаётся, чтобы следующий rebuild
	// перепроверил. CacheStale НЕ трогаем: rebuild не делал network fetch (при
	// cacheMissing Update уже его сбросил; иначе CacheStale остаётся как был).
	if configValid {
		ac.StateService.ClearConfigStale()
		if ac.EventBus != nil {
			ac.EventBus.Publish(events.Event{
				Kind:    events.ConfigBuilt,
				Payload: events.ConfigBuiltPayload{OK: true, Warnings: res.Validation.Warnings},
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

	debuglog.InfoLog("RebuildConfigIfDirty: config.json written (%d bytes)", len(res.ConfigJSON))
	return nil
}

// CleanOrphanRuleSets removes bin/rule-sets/*.srs files not referenced by ANY
// saved wizard state — the same multi-stage live-set the rebuild GC (Step 5.5)
// uses. Returns the removed filenames.
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
	execDir := ac.FileService.ExecDir
	td, err := template.LoadTemplateData(execDir)
	if err != nil {
		return nil, fmt.Errorf("CleanOrphanRuleSets: load template: %w", err)
	}
	known := collectAllStageRuleSetTags(execDir, td)
	return services.DeleteOrphanRuleSets(execDir, known)
}

// cleanupLegacyOutboundsCache удаляет `bin/outbounds.cache.json`, если он
// существует (legacy SPEC 045 cache, выпиленный в SPEC 052). One-shot:
// файл не пересоздаётся новым кодом, поэтому достаточно удалить однажды
// и забыть. Best-effort — ошибки не критичны.
func cleanupLegacyOutboundsCache(execDir string) {
	path := platform.GetOutboundsCachePath(execDir)
	if _, err := os.Stat(path); err == nil {
		if remErr := os.Remove(path); remErr == nil {
			debuglog.InfoLog("cleanupLegacyOutboundsCache: removed legacy %s", path)
		} else {
			debuglog.WarnLog("cleanupLegacyOutboundsCache: failed to remove %s: %v", path, remErr)
		}
	}
}
