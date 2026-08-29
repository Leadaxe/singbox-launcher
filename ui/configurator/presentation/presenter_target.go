package presentation

import (
	"fmt"
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

// ConfigMachineID — ID машины, чей профиль сейчас редактируется (SPEC 098).
// Пусто для local и для remote-состояний, оставшихся от singleton-раскладки.
func (p *WizardPresenter) ConfigMachineID() string {
	if p.model == nil {
		return ""
	}
	return p.model.Target.MachineIDOrEmpty()
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
	// Меняется ТОЛЬКО платформа: машина та же, поэтому id и каталоги едут из
	// текущего таргета. Собрать спек заново «по двум полям» значило бы увести
	// Save в singleton-папку через смену выпадающего списка архитектуры.
	p.model.Target = wizardtemplate.TargetSpec{
		GOOS:        goos,
		GOARCH:      goarch,
		Target:      constants.ConfigTargetRemote,
		MachineID:   cur.MachineIDOrEmpty(),
		ResourceDir: cur.ResourceDir,
		SrsLocalDir: cur.SrsLocalDir,
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
	// MachineID и каталоги переезжают из prev: переключатель local⇄remote
	// меняет ТАРГЕТ, а не машину — окно как было открыто на своей строке, так
	// на ней и остаётся. Потерять их здесь значит увести следующий Save в
	// singleton-папку вместо профиля этой машины.
	return wizardtemplate.TargetSpec{
		GOOS:        goos,
		GOARCH:      goarch,
		Target:      constants.ConfigTargetRemote,
		MachineID:   prev.MachineIDOrEmpty(),
		ResourceDir: prev.ResourceDir,
		SrsLocalDir: prev.SrsLocalDir,
	}.Normalized()
}

// targetSpecFromStateMeta восстанавливает TargetSpec из meta-полей state'а.
// Файлы, записанные до SPEC 097, не имеют meta.target — они нормализуются в
// local-таргет текущей машины, то есть читаются ровно как раньше.
//
// open — таргет ОТКРЫТОГО визарда, то есть машина, на строке которой нажали
// Configure (ShowConfigWizardForMachine кладёт туда её ID). Не «прошлая
// машина»: другой она за жизнь окна не станет, визард открывается на одной
// строке и не переключается между машинами.
//
// Из него берётся то, чего в файле нет и быть не может: MachineID и два
// каталога (ResourceDir на машине, SrsLocalDir у нас). Читаемый файл эти поля
// не несёт, поэтому собранный «по meta» таргет обнулял их — и первый же Load
// уводил Save в singleton-папку bin/wizard_states/remote/ мимо профиля машины.
//
// В файл они не пишутся намеренно: id и пути — свойство ЗАПИСИ РЕЕСТРА, а не
// содержимого настроек. Ровно поэтому копирование профиля между машинами
// (RemoteRegistry.CopyProfileFrom) безопасно: скопированный state не тащит за
// собой id донора, и Save из визарда приёмника уходит в ЕГО папку. Продублируй
// мы id внутрь state — копия несла бы чужой id и затирала настройки донора.
func targetSpecFromStateMeta(sf *wizardmodels.WizardStateFile,
	open wizardtemplate.TargetSpec) wizardtemplate.TargetSpec {
	if sf == nil {
		return wizardtemplate.LocalTarget()
	}
	next := wizardtemplate.TargetSpec{
		GOOS:   sf.TargetPlatform,
		GOARCH: sf.TargetArch,
		Target: sf.Target,
	}.Normalized()
	// Только для remote: у local ни id, ни чужих каталогов нет по построению.
	if next.IsRemote() {
		next.MachineID = open.MachineIDOrEmpty()
		next.ResourceDir = open.ResourceDir
		next.SrsLocalDir = open.SrsLocalDir
	}
	return next
}

// invalidateParsedNodes сбрасывает разобранные ноды после смены таргета.
//
// У каждого таргета свой набор источников и outbound-групп, поэтому ноды,
// разобранные для предыдущего, к новому отношения не имеют. Парсер и так
// выбрасывает результат при уходе ревизии модели вперёд, но флаг «нужен
// разбор» при этом не взводился — и Save экспортировал конфиг с ПУСТЫМИ
// секциями между парсер-маркерами (2 статических outbound вместо 43 нод).
//
// Ставим флаг явно: превью и Save увидят, что данные надо перечитать.
func (p *WizardPresenter) invalidateParsedNodes() {
	if p.model == nil {
		return
	}
	p.model.GeneratedOutbounds = nil
	p.model.GeneratedEndpoints = nil
	p.model.OutboundStats = wizardmodels.OutboundStats{}
	p.model.BumpRevision()
	p.model.PreviewNeedsParse = true
	wizardbusiness.InvalidateNodePool(p.model)
}

// ApplyClonedState применяет состояние, склонированное с ДРУГОЙ машины.
//
// Отличается от LoadState ровно одним, но критическим пунктом: у клона снята
// идентичность донора, в том числе meta.target. LoadState читает таргет из
// файла (targetSpecFromStateMeta), пустой meta.target нормализуется в local —
// и remote-машина, на которую клонировали, молча стала бы local. Следующий
// Save ушёл бы в bin/wizard_states/state.json, затирая ЛОКАЛЬНЫЙ конфиг
// чужим. Поэтому таргет приёмника снимается ДО загрузки и ставится обратно
// после — файл описывает настройки, а машину описывает открытый визард.
//
// Состояние помечается изменённым, а не сохранённым: клон — это правка,
// которую пользователь ещё может отменить, закрыв визард без Save.
func (p *WizardPresenter) ApplyClonedState(stateFile *wizardmodels.WizardStateFile) error {
	if p.model == nil {
		return fmt.Errorf("model is not initialized")
	}
	// Таргет приёмника — со всеми путевыми полями машины (MachineID,
	// ResourceDir, SrsLocalDir): они свойство записи реестра, а не файла.
	own := p.model.Target

	if err := p.LoadState(stateFile); err != nil {
		return err
	}

	p.model.Target = own
	// Ноды, разобранные для прежнего набора источников, к новому отношения
	// не имеют — иначе превью и Save показали бы состав ДО клона.
	p.invalidateParsedNodes()
	p.MarkAsChanged()

	debuglog.InfoLog("ApplyClonedState: applied, target restored to %s/%s (machine %q)",
		own.Normalized().Target, own.Normalized().GOARCH, own.MachineIDOrEmpty())

	// Перерисовка того же объёма, что и после смены таргета: клон меняет
	// СОСТАВ вкладок (набор источников, правил, vars), а не только значения.
	p.refreshAfterTargetChange()
	p.RefreshRulesTabAfterLoadState()
	return nil
}
