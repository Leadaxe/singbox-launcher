// File draft_reject.go — цикл страховки над черновиком Конфигуратора
// (Final и remote-Save). Кандидат проверяет локальный бинарь; выключения
// идут в model.Sources, не в сохранённый state.
package presentation

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

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
	res, err := p.runDraftRejectLoop(first, "", true, progress)
	return res.Accepted, res.Disabled, err
}

// draftRejectResult — итог прохода цикла над черновиком.
type draftRejectResult struct {
	// Accepted — конфиг последнего круга (принятый либо последний кандидат).
	Accepted string
	Disabled []core.CoreRejectedNode
	// Promoted — цикл сам заменил promotePath принятым конфигом. false = файл
	// на диске остался прежним (отказ не про узел, Stop, проверять нечем), и
	// записать последний кандидат должен вызывающий. Судить об этом по
	// наличию файла нельзя: от прошлого Save он лежит там всегда.
	Promoted bool
	// CheckErr — последний отказ ядра, если цикл кончился отказом. Молчать о
	// нём нельзя: конфиг, который ядро не приняло, машина не примет тоже.
	CheckErr       error
	StoppedByHuman bool
}

// runDraftRejectLoop гоняет кандидат → check → выключение в черновике.
//
// promotePath — куда писать принятый конфиг (remote config.json). Путь
// пустой + noPromote: только превью Final, боевой файл не трогаем.
func (p *WizardPresenter) runDraftRejectLoop(first string, promotePath string, noPromote bool, progress func(int)) (draftRejectResult, error) {
	res := draftRejectResult{Accepted: first}
	if p == nil || p.model == nil {
		return res, nil
	}
	ac := core.GetController()
	if ac == nil || ac.FileService == nil {
		return res, nil
	}
	configPath := promotePath
	if configPath == "" {
		configPath = ac.FileService.ConfigPath
	}
	if configPath == "" {
		return res, nil
	}
	if err := os.MkdirAll(filepath.Dir(configPath), platform.DefaultDirMode); err != nil {
		return res, err
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
		ForCheck:    localSrsPathsForCheck(p.model.ResourceDir, p.model.Target.SrsLocalDir),
	}
	out, err := core.RunRejectLoop(in)
	if err != nil {
		return res, err
	}
	if err := in.Disabler.Commit(); err != nil {
		debuglog.ErrorLog("draft reject: commit: %v", err)
	}
	publishDraftReject(ac, out)
	if len(out.AcceptedJSON) > 0 {
		res.Accepted = string(out.AcceptedJSON)
	}
	res.Disabled = out.Disabled
	res.Promoted = out.Promoted
	res.CheckErr = out.CheckErr
	res.StoppedByHuman = out.StoppedByHuman
	return res, nil
}

// localSrsPathsForCheck — вид конфига удалённой машины для локального check.
//
// rule_set[].path такого конфига ведёт в каталог ресурсов НА МАШИНЕ
// (resourceDir + "/" + имя, см. core/build), и локальное ядро падает на
// «no such file» раньше, чем доходит до узлов. Те же файлы под теми же
// именами лежат у нас в srsLocalDir — на время проверки пути ведут туда.
// nil для local-цели: там пути и так наши.
func localSrsPathsForCheck(resourceDir, srsLocalDir string) func([]byte) []byte {
	if resourceDir == "" || srsLocalDir == "" {
		return nil
	}
	// Пути сравниваются в том виде, в каком лежат в JSON: у локального
	// каталога на Windows обратные слэши экранированы.
	from := `"` + jsonStringBody(resourceDir+"/")
	to := `"` + jsonStringBody(srsLocalDir+string(filepath.Separator))
	return func(cfg []byte) []byte {
		return []byte(strings.ReplaceAll(string(cfg), from, to))
	}
}

// jsonStringBody — строка в JSON-экранировании, без обрамляющих кавычек.
func jsonStringBody(s string) string {
	b, err := json.Marshal(s)
	if err != nil || len(b) < 2 {
		return s
	}
	return string(b[1 : len(b)-1])
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
