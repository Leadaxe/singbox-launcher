package core

import (
	"errors"
	"fmt"
	"os"

	"singbox-launcher/core/build"
	"singbox-launcher/core/events"
	"singbox-launcher/core/state"
	"singbox-launcher/core/template"
	"singbox-launcher/internal/debuglog"
	"singbox-launcher/internal/platform"
)

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
// No-op условие: оба dirty-маркера чисты И полный raw cache на диске.
//
// Возвращает:
//   - nil — успех (или nothing-to-do);
//   - error — fatal на этапе сборки/записи.
func (ac *AppController) RebuildConfigIfDirty() error {
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

	// Step 2: попытаться построить snapshot из raw cache.
	cacheSnap, snapErr := buildSnapshotFromRawCache(s, execDir, nil)
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
		cacheSnap, snapErr = buildSnapshotFromRawCache(s, execDir, nil)
		if snapErr != nil {
			return fmt.Errorf("rebuild snapshot after auto-update: %w", snapErr)
		}
	}

	// Step 3: noop fast-path.
	if !cacheMissing && !ac.StateService.IsCacheStale() && !ac.StateService.IsConfigStale() {
		return nil
	}

	debuglog.InfoLog("RebuildConfigIfDirty: rebuilding config.json (update_dirty=%v restart_dirty=%v cache_missing_initially=%v)",
		ac.StateService.IsCacheStale(), ac.StateService.IsConfigStale(), cacheMissing)

	td, err := template.LoadTemplateData(execDir)
	if err != nil {
		return fmt.Errorf("load template: %w", err)
	}

	// Step 4: build.
	parserCfg := s.ParserConfig
	ctx := buildContextFromState(s, cacheSnap, td, &parserCfg)
	res, err := build.BuildConfig(ctx)
	if err != nil {
		return fmt.Errorf("build: %w", err)
	}

	// Step 5: atomic write.
	if err := atomicWriteConfig(ac.FileService.ConfigPath, res.ConfigJSON); err != nil {
		return fmt.Errorf("write config: %w", err)
	}

	// Step 6: clear ConfigStale (config теперь свеж относительно state+cache).
	// CacheStale НЕ трогаем: rebuild не делал network fetch (если только не
	// auto-Update сработал выше — в этом случае Update сам уже его сбросил).
	// Поэтому если cacheMissing=true → CacheStale уже cleared изнутри Update;
	// если cacheMissing=false → CacheStale остаётся как был (rebuild не
	// обновлял подписки и не имеет права отчитываться за это).
	ac.StateService.ClearConfigStale()

	if ac.EventBus != nil {
		ac.EventBus.Publish(events.Event{
			Kind:    events.ConfigBuilt,
			Payload: events.ConfigBuiltPayload{OK: true, Warnings: res.Validation.Warnings},
		})
	}

	// Step 7: refresh UI markers (coarse — будем заменять на typed events).
	if ac.UIService != nil {
		if ac.UIService.UpdateConfigStatusFunc != nil {
			ac.UIService.UpdateConfigStatusFunc()
		}
		if ac.UIService.UpdateCoreStatusFunc != nil {
			ac.UIService.UpdateCoreStatusFunc()
		}
	}

	debuglog.InfoLog("RebuildConfigIfDirty: config.json written (%d bytes)", len(res.ConfigJSON))
	return nil
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
