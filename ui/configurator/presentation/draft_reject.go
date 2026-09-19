// File draft_reject.go — цикл страховки над черновиком Конфигуратора
// (Final и remote-Save). Кандидат проверяет локальный бинарь; выключения
// идут в model.Sources, не в сохранённый state.
package presentation

import (
	"os"
	"path/filepath"

	"singbox-launcher/core"
	"singbox-launcher/core/events"
	"singbox-launcher/core/state"
	"singbox-launcher/internal/debuglog"
	"singbox-launcher/internal/platform"
	wizardbusiness "singbox-launcher/ui/configurator/business"
)

// RunDraftRejectPreview — цикл на Final: кандидат проверяется, боевой
// config.json не заменяется. Save остаётся доступной и после Stop.
func (p *WizardPresenter) RunDraftRejectPreview(first string, progress func(int)) (string, []core.CoreRejectedNode, error) {
	return p.runDraftRejectLoop(first, "", true, progress)
}

// runDraftRejectLoop гоняет кандидат → check → выключение в черновике.
//
// promotePath — куда писать принятый конфиг (remote config.json). Путь
// пустой + noPromote: только превью Final, боевой файл не трогаем.
func (p *WizardPresenter) runDraftRejectLoop(first string, promotePath string, noPromote bool, progress func(int)) (string, []core.CoreRejectedNode, error) {
	if p == nil || p.model == nil {
		return first, nil, nil
	}
	ac := core.GetController()
	if ac == nil || ac.FileService == nil {
		return first, nil, nil
	}
	configPath := promotePath
	if configPath == "" {
		configPath = ac.FileService.ConfigPath
	}
	if configPath == "" {
		return first, nil, nil
	}
	if err := os.MkdirAll(filepath.Dir(configPath), platform.DefaultDirMode); err != nil {
		return first, nil, err
	}

	rebuild := func() ([]byte, map[string]state.NodeLink, error) {
		configService := &wizardbusiness.ConfigServiceAdapter{CoreConfigService: ac.ConfigService}
		if err := wizardbusiness.ParseAndPreview(p, configService); err != nil {
			return nil, nil, err
		}
		var text string
		var err error
		if noPromote {
			text, _, err = wizardbusiness.BuildFinalReportConfig(p.model)
		} else {
			text, err = wizardbusiness.BuildRemoteConfig(p.model)
		}
		if err != nil {
			return nil, nil, err
		}
		return []byte(text), p.model.NodeLinks, nil
	}

	in := core.RejectLoopInput{
		SingboxPath: ac.FileService.SingboxPath,
		ConfigPath:  configPath,
		NoPromote:   noPromote,
		FirstJSON:   []byte(first),
		FirstLinks:  p.model.NodeLinks,
		Rebuild:     rebuild,
		Disabler:    newDraftDisabler(p.model),
		Decide:      ac.CoreRejectDecideFn(),
		Progress:    composeRejectProgress(ac, progress),
	}
	out, err := core.RunRejectLoop(in)
	if err != nil {
		return first, nil, err
	}
	if err := in.Disabler.Commit(); err != nil {
		debuglog.ErrorLog("draft reject: commit: %v", err)
	}
	publishDraftReject(ac, out)
	accepted := first
	if len(out.AcceptedJSON) > 0 {
		accepted = string(out.AcceptedJSON)
	}
	return accepted, out.Disabled, nil
}

func composeRejectProgress(ac *core.AppController, extra func(int)) func(int) {
	hook := ac.CoreRejectProgressFn()
	if hook == nil && extra == nil {
		return nil
	}
	return func(n int) {
		if hook != nil {
			hook(n)
		}
		if extra != nil {
			extra(n)
		}
	}
}

func publishDraftReject(ac *core.AppController, out core.RejectLoopResult) {
	if ac == nil || ac.EventBus == nil || len(out.Disabled) == 0 {
		return
	}
	nodes := make([]events.DisabledNode, 0, len(out.Disabled))
	for _, n := range out.Disabled {
		nodes = append(nodes, events.DisabledNode{
			SourceLabel: n.SourceLabel, Tag: n.Tag, Reason: n.Reason,
		})
	}
	ac.EventBus.Publish(events.Event{
		Kind: events.ConfigBuilt,
		Payload: events.ConfigBuiltPayload{
			OK:            out.Promoted,
			DisabledNodes: nodes,
		},
	})
}

