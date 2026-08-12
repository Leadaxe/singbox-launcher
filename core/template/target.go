package template

import (
	"runtime"
	"strings"
)

// TargetSpec — для какой машины готовится config.json (SPEC 097).
//
// До SPEC 097 генерация неявно предполагала «конфиг для машины, на которой
// запущен лаунчер»: платформа бралась из runtime.GOOS/GOARCH прямо внутри
// пайплайна. TargetSpec делает эту связь явной и параметризуемой — лаунчер
// на macOS может собрать конфиг для linux-роутера.
//
// Две независимые оси:
//   - GOOS/GOARCH — платформа ЦЕЛЕВОЙ машины. Питает params.platforms,
//     default_value per-platform и @runtime.platform / @runtime.arch.
//   - Target — local | remote. Питает @runtime.target. Определяет,
//     применимы ли к конфигу локальные допущения (clash_api, find_process,
//     set_system_proxy): на remote управление идёт только через lxd.
//
// «Другой mac» — это Target=remote при GOOS=darwin: оси ортогональны,
// удалённость не выводится из платформы.
type TargetSpec struct {
	// GOOS — платформа целевой машины ("linux", "darwin", "windows").
	GOOS string

	// GOARCH — архитектура целевой машины. Влияет на win7-ветку
	// default_value (windows/386) и @runtime.arch.
	GOARCH string

	// Target — TargetLocal | TargetRemote. Пустая строка трактуется как
	// TargetLocal (nil-tolerance для legacy-вызовов).
	Target string
}

// Значения TargetSpec.Target. Совпадают со слагами: TargetRemote — имя
// подпапки состояний bin/wizard_states/remote/.
const (
	TargetLocal  = "local"
	TargetRemote = "remote"
)

// WizardUITarget — значение vars[].wizard_ui для переменных, которые
// рендерятся на шаге 0 визарда (вкладка Target), а не в Settings.
//
// Зачем отдельное размещение: от таких vars зависят default_value других
// полей (gateway_mode → proxy_in_listen), а дефолты резолвятся однопроходно
// в порядке объявления. Переключатель, стоящий ниже зависимых полей или на
// другой вкладке, давал бы пользователю значения, посчитанные до его выбора.
const WizardUITarget = "target"

// TargetTabVars отбирает vars шага 0 (wizard_ui:"target") с сохранением
// порядка объявления.
func TargetTabVars(vars []TemplateVar) []TemplateVar {
	out := make([]TemplateVar, 0, 4)
	for _, v := range vars {
		if v.Separator {
			continue
		}
		if strings.EqualFold(strings.TrimSpace(v.WizardUI), WizardUITarget) {
			out = append(out, v)
		}
	}
	return out
}

// LocalTarget — таргет «эта машина»: платформа из runtime, Target=local.
// Единственное санкционированное место, где генерация смотрит на runtime.GOOS.
// Все прочие слои принимают TargetSpec параметром.
func LocalTarget() TargetSpec {
	return TargetSpec{GOOS: runtime.GOOS, GOARCH: runtime.GOARCH, Target: TargetLocal}
}

// RemoteTarget — таргет «удалённая машина» на указанной платформе.
// goos/goarch пустые → подставляется runtime (лучше собрать под свою
// платформу, чем уронить генерацию на пустой строке).
func RemoteTarget(goos, goarch string) TargetSpec {
	return TargetSpec{GOOS: goos, GOARCH: goarch, Target: TargetRemote}.Normalized()
}

// Normalized заполняет пустые поля дефолтами: GOOS/GOARCH — из runtime,
// Target — local. Вызывается на входе в пайплайн, чтобы вглубь шёл уже
// валидный spec.
func (t TargetSpec) Normalized() TargetSpec {
	if strings.TrimSpace(t.GOOS) == "" {
		t.GOOS = runtime.GOOS
	}
	if strings.TrimSpace(t.GOARCH) == "" {
		t.GOARCH = runtime.GOARCH
	}
	if strings.TrimSpace(t.Target) == "" {
		t.Target = TargetLocal
	}
	return t
}

// IsRemote true для Target=remote.
func (t TargetSpec) IsRemote() bool {
	return strings.EqualFold(strings.TrimSpace(t.Target), TargetRemote)
}

// TargetOrLocal возвращает Target, подставляя local для пустой строки.
// Используется резолвером @runtime.target.
func (t TargetSpec) TargetOrLocal() string {
	if s := strings.TrimSpace(t.Target); s != "" {
		return strings.ToLower(s)
	}
	return TargetLocal
}
