package core

// srs_refresh.go — обновление наборов правил (.srs) лаунчером (SPEC 185).
//
// Набор в режиме local — такой же обновляемый ресурс, как подписка: общий
// цикл auto_update.go раз в час находит файлы с истёкшим периодом и качает
// их заново; сборка и старт ядра докачивают только ОТСУТСТВУЮЩИЕ файлы.
// Свежесть — время изменения файла, в state ничего не пишется. Работающее
// ядро перечитывает заменённый local-набор само, поэтому после обновления
// существующего файла конфиг не пересобирается.

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/muhammadmuzzammil1998/jsonc"

	"singbox-launcher/core/build"
	"singbox-launcher/core/services"
	"singbox-launcher/core/state"
	"singbox-launcher/core/template"
	"singbox-launcher/internal/constants"
	"singbox-launcher/internal/debuglog"
	"singbox-launcher/internal/platform"
)

const (
	// srsRefreshParallel — одновременных загрузок наборов (soft cap: правил с
	// наборами может быть много, а источник обычно один — GitHub).
	srsRefreshParallel = 2
	// srsFailureBackoff — пауза после неудачной загрузки набора. Без неё сборка
	// без сети ждала бы полный лимит при каждом вызове, а мёртвая ссылка (404)
	// запрашивалась бы каждый час.
	srsFailureBackoff = time.Hour
	// srsEnsureDeadline — общий лимит докачки отсутствующих файлов внутри
	// сборки конфига.
	srsEnsureDeadline = 20 * time.Second
	// srsSweepDeadline — общий лимит одного обхода цикла / обновления перед Deploy.
	srsSweepDeadline = 5 * time.Minute
)

// srsRefreshTracker — состояние обновления наборов в памяти процесса.
type srsRefreshTracker struct {
	mu        sync.Mutex
	lastFail  map[string]time.Time // путь файла → время неудачи
	retry     *time.Timer
	lastEvent time.Time
}

var srsTracker = &srsRefreshTracker{lastFail: make(map[string]time.Time)}

func (t *srsRefreshTracker) inBackoff(path string, now time.Time) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	at, ok := t.lastFail[path]
	return ok && now.Sub(at) < srsFailureBackoff
}

func (t *srsRefreshTracker) note(path string, failed bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if failed {
		t.lastFail[path] = time.Now()
	} else {
		delete(t.lastFail, path)
	}
}

func (t *srsRefreshTracker) hasFailures() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return len(t.lastFail) > 0
}

// ruleSetPlan — local-наборы включённых правил АКТИВНОГО состояния машины и
// каталог, где лаунчер держит её файлы. machineID пуст — локальная машина.
func (ac *AppController) ruleSetPlan(machineID string) ([]build.RuleSetFetch, string, error) {
	layout := ac.FileService.Layout
	statePath := platform.GetWizardStatePath(layout.Data)
	if machineID != "" {
		statePath = platform.GetWizardStatePathFor(layout.Data, constants.ConfigTargetRemote, machineID)
	}
	s, err := state.Load(statePath)
	if err != nil {
		return nil, "", err
	}
	td, err := template.LoadTemplateData(layout)
	if err != nil {
		return nil, "", err
	}
	return ac.ruleSetPlanFor(s, td, machineID)
}

func (ac *AppController) ruleSetPlanFor(s *state.State, td *template.TemplateData, machineID string) ([]build.RuleSetFetch, string, error) {
	ctx := ac.buildContextFromState(s, nil, td)
	if machineID != "" {
		ctx.Preset.Target.SrsLocalDir = platform.GetRuleSetsDirFor(ac.FileService.Layout.Data, constants.ConfigTargetRemote, machineID)
	}
	return ctx.Preset.RuleSetFetches(), ctx.Preset.RuleSetFileDir(), nil
}

// srsRefreshOpts — что и как обновлять.
type srsRefreshOpts struct {
	// onlyMissing — качать только отсутствующие файлы (сборка, старт ядра).
	onlyMissing bool
	// ignoreBackoff — не учитывать паузу после неудачи (повтор, VPN-событие).
	ignoreBackoff bool
}

// srsRefreshResult — итог обхода.
type srsRefreshResult struct {
	appeared int // файлов не было — появились (конфиг стоит пересобрать)
	updated  int // существующие файлы заменены
	failed   int
}

// refreshRuleSetFiles качает наборы плана: отсутствующие всегда, существующие —
// когда истёк их период (если не onlyMissing). Не больше srsRefreshParallel
// загрузок сразу. Неудача оставляет прежний файл на месте.
func refreshRuleSetFiles(ctx context.Context, fetches []build.RuleSetFetch, dir string, opts srsRefreshOpts) srsRefreshResult {
	type job struct {
		fetch  build.RuleSetFetch
		path   string
		exists bool
	}
	now := time.Now()
	var jobs []job
	for _, f := range fetches {
		path := filepath.Join(dir, f.FileTag+".srs")
		mod, exists := services.SRSFileModTime(path)
		stale := exists && !opts.onlyMissing && f.IntervalHours > 0 &&
			now.Sub(mod) >= time.Duration(f.IntervalHours)*time.Hour
		if exists && !stale {
			continue
		}
		if !opts.ignoreBackoff && srsTracker.inBackoff(path, now) {
			continue
		}
		jobs = append(jobs, job{fetch: f, path: path, exists: exists})
	}
	var res srsRefreshResult
	if len(jobs) == 0 {
		return res
	}
	debuglog.DebugLog("srs refresh: start, %d rule-set(s) to fetch (onlyMissing=%v)", len(jobs), opts.onlyMissing)

	var mu sync.Mutex
	var wg sync.WaitGroup
	sem := make(chan struct{}, srsRefreshParallel)
	for _, j := range jobs {
		if ctx.Err() != nil {
			break
		}
		wg.Add(1)
		sem <- struct{}{}
		go func(j job) {
			defer wg.Done()
			defer func() { <-sem }()
			changed, err := services.RefreshSRS(ctx, j.fetch.URL, j.path)
			srsTracker.note(j.path, err != nil)
			mu.Lock()
			defer mu.Unlock()
			switch {
			case err != nil:
				res.failed++
				debuglog.WarnLog("srs refresh: %s: %v (previous file kept)", j.fetch.URL, err)
			case !j.exists:
				res.appeared++
			case changed:
				res.updated++
			}
		}(j)
	}
	wg.Wait()
	debuglog.InfoLog("srs refresh: done, %d new, %d updated, %d failed", res.appeared, res.updated, res.failed)
	return res
}

// runScheduledSRSRefresh — обход цикла автообновления: наборы локальной машины
// с истёкшим периодом и отсутствующие. Зовётся рядом с runScheduledRefresh.
func (ac *AppController) runScheduledSRSRefresh(trigger string, ignoreBackoff bool) {
	if ac == nil || ac.FileService == nil || ac.ctx == nil || ac.IsStorageSwitching() {
		return
	}
	fetches, dir, err := ac.ruleSetPlan("")
	if err != nil {
		debuglog.DebugLog("srs refresh (%s): no plan: %v", trigger, err)
		return
	}
	ctx, cancel := context.WithTimeout(ac.ctx, srsSweepDeadline)
	defer cancel()
	res := refreshRuleSetFiles(ctx, fetches, dir, srsRefreshOpts{ignoreBackoff: ignoreBackoff})
	if res.appeared > 0 && ac.StateService != nil {
		// Файла не было — правило было выпавшим. Как после обновления подписки:
		// конфиг помечается устаревшим и пересоберётся при ближайшем старте.
		ac.StateService.MarkConfigStale()
	}
	if res.failed > 0 && !ignoreBackoff {
		ac.scheduleSRSRetry()
	}
}

// scheduleSRSRetry — один повтор через autoUpdateRetryDelay, как у подписки.
func (ac *AppController) scheduleSRSRetry() {
	srsTracker.mu.Lock()
	defer srsTracker.mu.Unlock()
	if srsTracker.retry != nil {
		srsTracker.retry.Stop()
	}
	srsTracker.retry = time.AfterFunc(autoUpdateRetryDelay, func() {
		if ac.ctx.Err() != nil {
			return
		}
		ac.runScheduledSRSRefresh("retry", true)
	})
}

// cancelSRSRetry — остановка таймера повтора на завершении приложения.
func cancelSRSRetry() {
	srsTracker.mu.Lock()
	defer srsTracker.mu.Unlock()
	if srsTracker.retry != nil {
		srsTracker.retry.Stop()
		srsTracker.retry = nil
	}
}

// retryFailedRuleSets — досрочная попытка по VPN-событию для наборов, которые
// не скачались (без VPN GitHub может быть недоступен).
func (ac *AppController) retryFailedRuleSets(trigger string) {
	if ac.StateService == nil || !ac.StateService.IsAutoUpdateEnabled() || !srsTracker.hasFailures() {
		return
	}
	srsTracker.mu.Lock()
	now := time.Now()
	if now.Sub(srsTracker.lastEvent) < autoUpdateEventCooldown {
		srsTracker.mu.Unlock()
		return
	}
	srsTracker.lastEvent = now
	srsTracker.mu.Unlock()
	go ac.runScheduledSRSRefresh(trigger, true)
}

// ensureMissingRuleSets — докачка ОТСУТСТВУЮЩИХ файлов внутри сборки конфига
// (как сборка докачивает подписку без узлов). Истёкший период сборку не
// задерживает. Ошибки не возвращаются: правило без файла выпадет из конфига.
func (ac *AppController) ensureMissingRuleSets(s *state.State, td *template.TemplateData) {
	// ac.ctx == nil — контроллер собран без жизненного цикла (тесты): сети нет.
	if ac == nil || ac.FileService == nil || ac.ctx == nil || s == nil || td == nil {
		return
	}
	fetches, dir, err := ac.ruleSetPlanFor(s, td, "")
	if err != nil || len(fetches) == 0 {
		return
	}
	ctx, cancel := context.WithTimeout(ac.ctx, srsEnsureDeadline)
	defer cancel()
	refreshRuleSetFiles(ctx, fetches, dir, srsRefreshOpts{onlyMissing: true})
}

// RefreshMachineRuleSets — обновление наборов удалённой машины перед Deploy
// (SPEC 185 §2.3): отсутствующие и с истёкшим периодом. Залить изменившиеся
// файлы — дело Deploy (сравнение по SHA-256).
func (ac *AppController) RefreshMachineRuleSets(machineID string) {
	if ac == nil || ac.FileService == nil || ac.ctx == nil || machineID == "" {
		return
	}
	fetches, dir, err := ac.ruleSetPlan(machineID)
	if err != nil {
		debuglog.DebugLog("srs refresh (deploy %s): no plan: %v", machineID, err)
		return
	}
	ctx, cancel := context.WithTimeout(ac.ctx, srsSweepDeadline)
	defer cancel()
	refreshRuleSetFiles(ctx, fetches, dir, srsRefreshOpts{})
}

// configReferencesMissingRuleSet — config.json ссылается на local-набор,
// которого нет на диске: ядро на таком конфиге не стартует (FATAL initialize
// rule_set). Старт тогда принудительно пересобирает конфиг (SPEC 185 §2.2).
func (ac *AppController) configReferencesMissingRuleSet() bool {
	if ac == nil || ac.FileService == nil {
		return false
	}
	raw, err := os.ReadFile(ac.FileService.ConfigPath)
	if err != nil {
		return false
	}
	var parsed struct {
		Route struct {
			RuleSet []struct {
				Type string `json:"type"`
				Path string `json:"path"`
			} `json:"rule_set"`
		} `json:"route"`
	}
	if err := json.Unmarshal(jsonc.ToJSON(raw), &parsed); err != nil {
		return false
	}
	for _, rs := range parsed.Route.RuleSet {
		if rs.Type != "local" || rs.Path == "" || !filepath.IsAbs(rs.Path) {
			continue
		}
		if _, err := os.Stat(rs.Path); os.IsNotExist(err) {
			debuglog.WarnLog("pre-start: config.json references a missing rule-set file %s — rebuilding", rs.Path)
			return true
		}
	}
	return false
}
