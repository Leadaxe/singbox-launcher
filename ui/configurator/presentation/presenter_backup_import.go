// File presenter_backup_import.go — импорт LX Backup в модель визарда
// (вкладка «Итог», ui/configurator/tabs/settings_backup.go).
//
// Слияние живёт в core/backup (BACKUP.md §9) — одно на UI и Debug API. Здесь
// только то, чем UI-вход отличается: во что сливать (модель или пустое
// состояние), что приёмник знает о целях, и загрузка результата в модель.
package presentation

import (
	"fmt"

	"singbox-launcher/core/backup"
	corestate "singbox-launcher/core/state"
	wizardtemplate "singbox-launcher/core/template"
	wizardbusiness "singbox-launcher/ui/configurator/business"
)

// BackupImportStage — шаг, на котором импорт не удался (текст ошибки UI).
type BackupImportStage int

const (
	// BackupImportStageRead — модель не отдала текущее состояние.
	BackupImportStageRead BackupImportStage = iota + 1
	// BackupImportStageMerge — слияние файла с состоянием.
	BackupImportStageMerge
	// BackupImportStageLoad — загрузка результата в модель.
	BackupImportStageLoad
)

// ImportBackupFile сливает файл бэкапа с состоянием визарда и загружает
// результат в модель.
//
// fresh — у визарда ещё нет своей настройки: state.json этой цели не
// сохранялся, и несохранённых правок нет. Модель тогда — сид шаблона
// (Направления parser_config, DNS-серверы dns_options с умолчаниями шаблона),
// а не выбор пользователя, и файл сливается в ПУСТОЕ состояние — так же, как
// POST /backup/import на свежей установке (core/debugapi/backup_endpoints.go).
// Сливать в сид значило бы отдать победу умолчаниям шаблона по правилу
// «своё сильнее» (§9 п. 4, 5): DNS-серверы, включённые в файле, оставались
// выключенными — финальный DNS уезжал на системный резолвер мимо VPN, — а
// Направления шаблона вытесняли настроенные Направления файла
// (backup_direction_exists на новой машине).
func (p *WizardPresenter) ImportBackupFile(file *backup.File, fresh bool) (*backup.ImportResult, BackupImportStage, error) {
	st := p.CreateStateFromModel("", "")
	if st == nil {
		return nil, BackupImportStageRead, fmt.Errorf("cannot read the current state")
	}
	if fresh {
		empty := corestate.New()
		// Цель — не настройка, а адрес состояния (SPEC 097/098): пустое
		// состояние удалённой машины обязано остаться состоянием этой машины.
		empty.Target, empty.TargetPlatform, empty.TargetArch = st.Target, st.TargetPlatform, st.TargetArch
		st = empty
	}

	res, err := backup.ImportFile(st, file, p.backupImportOptions())
	if err != nil {
		return nil, BackupImportStageMerge, err
	}
	// Import заменил Rules[] мимо диска, а LoadState читает inline/srs-правила
	// из legacy-вида CustomRules — без пересборки они терялись (issue #111).
	corestate.RebuildLegacyRuleView(st)

	if err := p.LoadState(st); err != nil {
		return res, BackupImportStageLoad, err
	}
	return res, 0, nil
}

// backupImportOptions — что приёмник знает о целях и пресетах.
//
// Известные цели — из модели (правило, метящее в никуда, приедет выключенным,
// а не уронит конфиг ядра); тег блокировки и системные теги шаблона — те же,
// что у POST /backup/import: ими становится `include_block`, и они известны
// импорту как цели правил и route.final, даже когда в модели ничего нет.
func (p *WizardPresenter) backupImportOptions() backup.ImportOptions {
	opts := backup.ImportOptions{
		KnownOutbounds: wizardbusiness.GetAvailableOutbounds(p.model),
	}
	if p.model == nil || p.model.TemplateData == nil {
		return opts
	}
	td := p.model.TemplateData
	for _, preset := range td.Presets {
		if preset.ID != "" {
			opts.KnownPresets = append(opts.KnownPresets, preset.ID)
		}
	}
	opts.BlockTag = td.DirectionBlockTag()
	opts.SystemTags = td.SystemOutboundTags()
	// SPEC 129: объявления шаблона приёмника — перенос корневых
	// `dns_<tag>_<var>` файла в записи, нормы записи, типы каналов для Н9.
	opts.RecordVars = wizardtemplate.RecordVarDeclsFor(td, p.model.SettingsVars, p.model.Target)
	return opts
}
