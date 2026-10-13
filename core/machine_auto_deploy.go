package core

// machine_auto_deploy.go — SPEC 164: автообновление подписок удалённых машин
// и авто-деплой, когда изменились только узлы.
//
// Обход идёт из петли автообновления (auto_update.go) после локального
// обхода, последовательно по машинам с флагом AutoDeploySubscriptions. Сеть
// — только к ним: флаг и есть согласие на фоновые вызовы (правило heartbeat
// «без спроса к записям реестра не ходить»). Пересборка — тем же шагом, что
// Save Мастера машины (MachineConfigRebuildFunc), доставка — той же
// цепочкой, что кнопка Deploy (RemoteRegistry.Deploy).

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"strings"
	"time"

	"singbox-launcher/core/events"
	"singbox-launcher/core/services"
	"singbox-launcher/internal/constants"
	"singbox-launcher/internal/debuglog"
	"singbox-launcher/internal/locale"
	"singbox-launcher/internal/platform"
)

const (
	// autoDeployHealthTimeout — срок health-опроса машины в обходе: фоновый
	// тик не имеет права висеть на зависшей машине до минуты.
	autoDeployHealthTimeout = 10 * time.Second
	// autoDeployRefreshRetryDelay — задержка одного повтора фетча после
	// провала (как у локального автообновления). Повтор только освежает
	// файл состояния — деплоит исключительно тик обхода.
	autoDeployRefreshRetryDelay = 15 * time.Second
)

// runMachineSweep — §4.2 SPEC 164: обход машин с флагом авто-деплоя.
// Блокирующий, сеть; зовётся из горутины петли автообновления.
func (ac *AppController) runMachineSweep(trigger string) {
	if ac.FileService == nil || ac.ConfigService == nil {
		return
	}
	registry := services.NewRemoteRegistry(ac.FileService.Layout.Data)
	list, err := registry.List()
	if err != nil {
		debuglog.WarnLog("auto-deploy[%s]: read registry: %v", trigger, err)
		return
	}
	considered, deployed, skipped, failed := 0, 0, 0, 0
	for _, d := range list {
		if !d.AutoDeploySubscriptions {
			continue
		}
		if ac.IsStorageSwitching() {
			debuglog.DebugLog("auto-deploy[%s]: data move in progress, stopping sweep", trigger)
			return
		}
		considered++
		out := ac.AutoDeployMachine(d, trigger, false)
		switch out.Result {
		case services.AutoDeployApplied:
			deployed++
		case services.AutoDeploySkipped, services.AutoDeployNotConfigured:
			skipped++
		default:
			failed++
		}
	}
	if considered == 0 {
		return
	}
	debuglog.InfoLog("auto-deploy[%s]: %d machine(s): %d deployed, %d skipped, %d failed",
		trigger, considered, deployed, skipped, failed)
}

// AutoDeployMachine — §4.2.2 SPEC 164 для одной машины. force — фетчить все
// подписки машины, не только устаревшие (ручной запуск из Debug API).
// Блокирующий, сеть. Итог ложится в журнал машины (LastAutoDeploy), в лог и
// — кроме skipped/not_configured — в событие MachineAutoDeploy.
func (ac *AppController) AutoDeployMachine(d services.RemoteDaemon, trigger string, force bool) services.MachineAutoDeployOutcome {
	registry := services.NewRemoteRegistry(ac.FileService.Layout.Data)
	out := ac.autoDeployMachine(registry, d, trigger, force)
	if err := registry.SetLastAutoDeploy(d.ID, out.Record(time.Now())); err != nil {
		debuglog.WarnLog("auto-deploy[%s]: %s: record outcome: %v", trigger, d.ID, err)
	}
	switch out.Result {
	case services.AutoDeployApplied:
		debuglog.InfoLog("auto-deploy[%s]: %s: applied %s (%s)", trigger, d.Name, shortSHA(out.SHAAfter), out.Note)
	case services.AutoDeploySkipped, services.AutoDeployNotConfigured:
		debuglog.InfoLog("auto-deploy[%s]: %s: %s — %s", trigger, d.Name, out.Result, out.Note)
	default:
		debuglog.WarnLog("auto-deploy[%s]: %s: %s — %s", trigger, d.Name, out.Result, out.Note)
	}
	if ac.EventBus != nil && out.Result != services.AutoDeploySkipped && out.Result != services.AutoDeployNotConfigured {
		ac.EventBus.Publish(events.Event{
			Kind: events.MachineAutoDeploy,
			Payload: events.MachineAutoDeployPayload{
				ID: d.ID, Name: d.Name, Result: out.Result, SHA: out.SHAAfter, Note: out.Note,
			},
		})
	}
	return out
}

func (ac *AppController) autoDeployMachine(registry *services.RemoteRegistry, d services.RemoteDaemon,
	trigger string, force bool) services.MachineAutoDeployOutcome {
	dataDir := ac.FileService.Layout.Data
	configPath := platform.GetRemoteConfigPathFor(dataDir, d.ID)

	// 1. Машина должна отвечать до того, как мы тратим сеть на подписки.
	h := registry.HealthWithin(d.ID, autoDeployHealthTimeout)
	if h.Err != "" {
		return services.MachineAutoDeployOutcome{Result: services.AutoDeployUnreachable, Note: h.Err}
	}

	// 2. Конфиг до обхода: его sha и есть «что пользователь оставил».
	shaBefore := fileSHA256(configPath)
	if shaBefore == "" {
		return services.MachineAutoDeployOutcome{Result: services.AutoDeployNotConfigured,
			Note: "no built config — run Configure first"}
	}

	// 3. Подписки профиля машины.
	statePath := platform.GetWizardStatePathFor(dataDir, constants.ConfigTargetRemote, d.ID)
	settings := locale.LoadSettings(dataDir.Bin())
	res, err := ac.ConfigService.refreshStaleSubscriptionsAt(statePath, ac.MachineStateLock(d.ID), settings, force, time.Now().UTC())
	if err != nil {
		return services.MachineAutoDeployOutcome{Result: services.AutoDeployFailed, SHABefore: shaBefore,
			Note: "refresh subscriptions: " + err.Error()}
	}
	if len(res.Failed) > 0 {
		ids := make([]string, 0, len(res.Failed))
		for id, msg := range res.Failed {
			ids = append(ids, id+": "+msg)
		}
		ac.scheduleMachineRefreshRetry(d, trigger)
		return services.MachineAutoDeployOutcome{Result: services.AutoDeployFailed, SHABefore: shaBefore,
			Note: "subscription fetch failed (" + strings.Join(ids, "; ") + "), retry in " + autoDeployRefreshRetryDelay.String()}
	}
	if len(res.Changed) > 0 && ac.EventBus != nil {
		// Открытый Конфигуратор этой машины должен узнать о внешней записи
		// (SPEC 160 §E), иначе его Save перезапишет свежие узлы.
		ac.EventBus.Publish(events.Event{
			Kind: events.StateChanged,
			Payload: events.StateChangedPayload{
				Changed: []string{"sources"}, Source: events.StateSourceAutoUpdate,
				Target: constants.ConfigTargetRemote, MachineID: d.ID,
			},
		})
	}
	if len(res.Changed) == 0 {
		return services.MachineAutoDeployOutcome{Result: services.AutoDeploySkipped, SHABefore: shaBefore,
			SHAAfter: shaBefore, Note: "subscriptions unchanged"}
	}

	// 4. Пересборка тем же шагом, что Save Мастера машины.
	if ac.MachineConfigRebuildFunc == nil {
		return services.MachineAutoDeployOutcome{Result: services.AutoDeployBuildFailed, SHABefore: shaBefore,
			Note: "config builder is not available (headless)"}
	}
	if _, err := ac.MachineConfigRebuildFunc(d.ID); err != nil {
		return services.MachineAutoDeployOutcome{Result: services.AutoDeployBuildFailed, SHABefore: shaBefore,
			Note: "build: " + err.Error()}
	}

	// 5. Изменился ли конфиг от пересборки.
	shaAfter := fileSHA256(configPath)
	if shaAfter == "" || shaAfter == shaBefore {
		return services.MachineAutoDeployOutcome{Result: services.AutoDeploySkipped, SHABefore: shaBefore,
			SHAAfter: shaAfter, Note: "config unchanged"}
	}

	// 6. Решение по §3.3–3.4.
	if ok, reasons := autoDeployDecision(shaBefore, shaAfter, h, d.CoreWarnAck, constants.RequiredCoreVersion); !ok {
		return services.MachineAutoDeployOutcome{Result: services.AutoDeploySkipped, SHABefore: shaBefore,
			SHAAfter: shaAfter, Note: strings.Join(reasons, "; ")}
	}

	// 7. Доставка той же цепочкой, что кнопка Deploy.
	dres, err := registry.Deploy(d.ID, nil)
	if err != nil {
		return services.MachineAutoDeployOutcome{Result: services.AutoDeployFailed, SHABefore: shaBefore,
			SHAAfter: shaAfter, Note: "deploy: " + err.Error()}
	}
	return services.MachineAutoDeployOutcome{Result: services.AutoDeployApplied, SHABefore: shaBefore,
		SHAAfter: dres.ConfigSHA, Deployed: true,
		Note: fmt.Sprintf("%d subscription(s) changed: %s", len(res.Changed), strings.Join(res.Titles, ", "))}
}

// autoDeployDecision — чистое решение §3.3–3.4 SPEC 164: деплоить ли
// пересобранный конфиг машины без участия пользователя. Возвращает ok и
// причины отказа словами журнала (англ., не локализуются).
func autoDeployDecision(shaBefore, shaAfter string, h services.RemoteHealth,
	coreWarnAck, requiredCore string) (bool, []string) {
	var reasons []string
	if shaBefore == "" || shaAfter == "" || shaBefore == shaAfter {
		reasons = append(reasons, "config unchanged")
	}
	if shaBefore != h.ActiveSHA {
		reasons = append(reasons, "built config differed from active before refresh (manual Deploy expected)")
	}
	if h.CoreStatus != "started" {
		reasons = append(reasons, "core status "+emptyAs(h.CoreStatus, "unknown"))
	}
	if h.ActiveSHA != h.LastGoodSHA {
		reasons = append(reasons, "active config is not last-good")
	}
	if h.InterruptedApply {
		reasons = append(reasons, "previous apply was interrupted")
	}
	if CompareCoreVersion(h.Version, requiredCore) == CoreVersionOlder && coreWarnAck != h.Version {
		reasons = append(reasons, "core "+h.Version+" older than required "+requiredCore)
	}
	return len(reasons) == 0, reasons
}

// scheduleMachineRefreshRetry — один повтор фетча подписок машины через 15 с
// после провала. Повтор только освежает файл состояния (деплой — дело
// следующего тика), поэтому исход не публикуется.
func (ac *AppController) scheduleMachineRefreshRetry(d services.RemoteDaemon, trigger string) {
	time.AfterFunc(autoDeployRefreshRetryDelay, func() {
		if ac.ConfigService == nil || ac.FileService == nil || ac.IsStorageSwitching() {
			return
		}
		dataDir := ac.FileService.Layout.Data
		statePath := platform.GetWizardStatePathFor(dataDir, constants.ConfigTargetRemote, d.ID)
		settings := locale.LoadSettings(dataDir.Bin())
		res, err := ac.ConfigService.refreshStaleSubscriptionsAt(statePath, ac.MachineStateLock(d.ID), settings, false, time.Now().UTC())
		if err != nil || len(res.Failed) > 0 {
			debuglog.WarnLog("auto-deploy[%s-retry]: %s: subscriptions still failing (next attempt at next heartbeat)", trigger, d.Name)
			return
		}
		debuglog.InfoLog("auto-deploy[%s-retry]: %s: subscriptions refreshed, %d changed (deploy at next heartbeat)", trigger, d.Name, len(res.Changed))
	})
}

// fileSHA256 — sha256 файла ("" — нет файла или не читается).
func fileSHA256(path string) string {
	raw, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

func emptyAs(s, fallback string) string {
	if s == "" {
		return fallback
	}
	return s
}
