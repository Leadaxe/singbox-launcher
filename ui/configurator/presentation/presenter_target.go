package presentation

import (
	"runtime"
	"strings"

	wizardtemplate "singbox-launcher/core/template"
	"singbox-launcher/internal/constants"
	"singbox-launcher/internal/debuglog"
	wizardbusiness "singbox-launcher/ui/configurator/business"
	wizardmodels "singbox-launcher/ui/configurator/models"
)

// SPEC 097 — переключение таргета конфига (local ⇄ remote).
//
// Таргет — это не просто флаг в модели: он выбирает ФАЙЛ СОСТОЯНИЯ
// (bin/wizard_states/state.json против bin/wizard_states/remote/state.json),
// поэтому переключение = flush текущего state + чтение целевого + перерисовка.
//
// Флаш делается всегда и молча: пользователь, ткнувший в другой таргет, не
// ожидает потерять правки, а модальный вопрос «сохранить?» на каждом
// переключении раздражает сильнее, чем лишняя запись на диск.

// ConfigTarget — текущий таргет модели ("local" | "remote").
func (p *WizardPresenter) ConfigTarget() string {
	if p.model == nil {
		return constants.ConfigTargetLocal
	}
	if p.model.Target.Normalized().IsRemote() {
		return constants.ConfigTargetRemote
	}
	return constants.ConfigTargetLocal
}

// SwitchConfigTarget переключает таргет: сохраняет текущее состояние в его
// директории, затем читает состояние нового таргета и перезагружает модель.
//
// Для нового (ещё не существующего) remote-состояния LoadCurrentState вернёт
// ошибку — это не сбой, а первый заход: модель остаётся как есть, но уже
// помечена remote, и первый Save создаст bin/wizard_states/remote/state.json.
func (p *WizardPresenter) SwitchConfigTarget(next string) {
	if p.model == nil {
		return
	}
	next = strings.ToLower(strings.TrimSpace(next))
	if next != constants.ConfigTargetRemote {
		next = constants.ConfigTargetLocal
	}
	if next == p.ConfigTarget() {
		return
	}

	prev := p.ConfigTarget()

	// 1. Flush текущего таргета — store ещё смотрит на СТАРУЮ директорию,
	// потому что p.model.Target пока не переставлен.
	if err := p.SaveCurrentState(); err != nil {
		debuglog.WarnLog("SwitchConfigTarget: flush of %s state failed: %v", prev, err)
	}

	// 2. Переставляем таргет: с этого момента GetStateStore() отдаёт store
	// новой директории.
	nextSpec := targetSpecFor(next, p.model.Target)
	p.model.Target = nextSpec
	debuglog.InfoLog("SwitchConfigTarget: %s → %s (%s/%s)", prev, next,
		nextSpec.GOOS, nextSpec.GOARCH)

	// 3. Читаем состояние нового таргета.
	stateFile, err := p.GetStateStore().LoadCurrentState()
	if err != nil {
		debuglog.InfoLog("SwitchConfigTarget: no existing %s state (%v) — starting from current model", next, err)
		p.MarkAsChanged()
		p.invalidateParsedNodes()
		p.refreshAfterTargetChange()
		return
	}
	if err := p.LoadState(stateFile); err != nil {
		debuglog.ErrorLog("SwitchConfigTarget: restore %s state failed: %v", next, err)
		return
	}
	// LoadState восстанавливает Target из meta прочитанного файла. Если файл
	// legacy (без meta.target) — там окажется local, и следующий flush записал
	// бы remote-состояние в local/state.json, затирая его. Переставляем
	// обратно на тот таргет, чью директорию мы читали; расхождение с файлом
	// помечаем как несохранённое, чтобы meta дописалась.
	if !sameTarget(p.model.Target, nextSpec) {
		p.model.Target = nextSpec
		p.MarkAsChanged()
	}
	p.invalidateParsedNodes()
	p.refreshAfterTargetChange()
}

// sameTarget сравнивает таргеты по значимым полям (роль + платформа).
func sameTarget(a, b wizardtemplate.TargetSpec) bool {
	a, b = a.Normalized(), b.Normalized()
	return a.Target == b.Target && a.GOOS == b.GOOS && a.GOARCH == b.GOARCH
}

// refreshAfterTargetChange — перерисовка после смены таргета/платформы.
//
// SyncModelToGUI обновляет ЗНАЧЕНИЯ существующих виджетов, но вкладки, чей
// СОСТАВ зависит от таргета (Target — поля платформы; Settings — набор vars
// и их дефолты), должны перестроиться целиком. Без этого выбор "Remote"
// оставлял вкладку с local-подсказкой и без выбора платформы.
func (p *WizardPresenter) refreshAfterTargetChange() {
	p.SyncModelToGUI()
	p.UpdateUI(func() {
		// Вкладка Target: состав зависит от таргета (поля платформы) и от
		// её собственных vars. Регистрируется через GUIState — один путь,
		// чтобы вкладка не перестраивалась дважды.
		if p.guiState != nil && p.guiState.RefreshTargetTabFromModel != nil {
			p.guiState.RefreshTargetTabFromModel()
		}
		// Settings держит набор vars и их дефолты — они зависят от таргета
		// (per-platform default_value). Виджеты там создаются по составу,
		// поэтому нужна полная перестройка, а не SetText.
		if p.guiState != nil && p.guiState.RefreshSettingsFromModel != nil {
			p.guiState.RefreshSettingsFromModel()
		}
	})
}

// SetTargetPlatform меняет платформу целевой машины (значимо только для
// remote: у local платформа всегда runtime'а). Файл состояния не меняется —
// это тот же таргет, поэтому ни flush, ни перечитывание не нужны.
func (p *WizardPresenter) SetTargetPlatform(goos, goarch string) {
	if p.model == nil || !p.model.Target.Normalized().IsRemote() {
		return
	}
	cur := p.model.Target.Normalized()
	if goos == "" {
		goos = cur.GOOS
	}
	if goarch == "" {
		goarch = cur.GOARCH
	}
	if goos == cur.GOOS && goarch == cur.GOARCH {
		return
	}
	p.model.Target = wizardtemplate.TargetSpec{
		GOOS: goos, GOARCH: goarch, Target: constants.ConfigTargetRemote,
	}.Normalized()
	p.MarkAsChanged()
	debuglog.InfoLog("SetTargetPlatform: remote target platform → %s/%s", goos, goarch)
	// Платформа меняет видимость и дефолты vars — перестраиваем вкладки.
	p.refreshAfterTargetChange()
}

// targetSpecFor собирает TargetSpec для таргета, сохраняя ранее выбранную
// платформу remote. Для local платформа всегда runtime'а: собирать конфиг
// «для этой машины, но под другую ОС» бессмысленно.
func targetSpecFor(target string, prev wizardtemplate.TargetSpec) wizardtemplate.TargetSpec {
	if target != constants.ConfigTargetRemote {
		return wizardtemplate.LocalTarget()
	}
	goos, goarch := prev.GOOS, prev.GOARCH
	// Свежий remote: платформа хоста — плохой дефолт (лаунчер на mac, роутер
	// на linux), поэтому начинаем с linux как самого частого случая.
	if !prev.Normalized().IsRemote() || strings.TrimSpace(goos) == "" {
		goos = "linux"
	}
	if strings.TrimSpace(goarch) == "" {
		goarch = runtime.GOARCH
	}
	return wizardtemplate.TargetSpec{GOOS: goos, GOARCH: goarch, Target: constants.ConfigTargetRemote}.Normalized()
}

// targetSpecFromStateMeta восстанавливает TargetSpec из meta-полей state'а.
// Файлы, записанные до SPEC 097, не имеют meta.target — они нормализуются в
// local-таргет текущей машины, то есть читаются ровно как раньше.
func targetSpecFromStateMeta(sf *wizardmodels.WizardStateFile) wizardtemplate.TargetSpec {
	if sf == nil {
		return wizardtemplate.LocalTarget()
	}
	return wizardtemplate.TargetSpec{
		GOOS:   sf.TargetPlatform,
		GOARCH: sf.TargetArch,
		Target: sf.Target,
	}.Normalized()
}

// invalidateParsedNodes сбрасывает разобранные ноды после смены таргета.
//
// У каждого таргета свой набор источников и outbound-групп, поэтому ноды,
// разобранные для предыдущего, к новому отношения не имеют. Парсер и так
// выбрасывает их при расхождении ParserConfigJSON, но флаг «нужен разбор»
// при этом не взводился — и Save экспортировал конфиг с ПУСТЫМИ секциями
// между парсер-маркерами (2 статических outbound вместо 43 нод).
//
// Ставим флаг явно: превью и Save увидят, что данные надо перечитать.
func (p *WizardPresenter) invalidateParsedNodes() {
	if p.model == nil {
		return
	}
	p.model.GeneratedOutbounds = nil
	p.model.GeneratedEndpoints = nil
	p.model.OutboundStats = wizardmodels.OutboundStats{}
	p.model.ParserConfig = nil
	p.model.PreviewNeedsParse = true
	wizardbusiness.InvalidatePreviewCache(p.model)
}
