// File node_build_gate.go — полевой гейт сборки по версии ядра и платформе
// (SPEC 131 §3.4, W2c).
//
// Тело узла в state ЗАМОРОЖЕНО и от запущенного ядра не зависит: узел,
// добавленный на ядре lx.4, остаётся тем же телом после отката на lx.3. Но
// ключ, которого это ядро не знает, — не «поле похуже», а отказ ВСЕГО
// config.json («unknown field»), то есть человек остаётся без VPN целиком.
// Поэтому на сборке тело проходит табличную проверку по реестру: ключ с
// `min_core` выше версии ядра или с `platform`, не совпадающей с целевой ОС,
// опускается, и это пишется в лог сборки WARN-строкой.
//
// ⚠ на узле от этого гейта НЕ ставится (§3.4): тело узла верное, ограничен
// рантайм. Узловой гейт — другой класс: он выбрасывает узел целиком
// (nodeflow.NodeCoreRefusal по `on_core_unsupported` реестра, SPEC 142
// волна 5; цепочки — отдельно, ChainSupportProbe).
//
// До W2c ту же работу делала ОДНА частная проба на одно поле
// (RealityKeyShareSupportProbe → tls.reality.key_share), зашитая внутрь
// эмиттера TLS. Каждый следующий пин ядра требовал новой пробы, нового хука
// и новой ветки в эмиттере; теперь достаточно записи `min_core` в реестре.
package config

import (
	"encoding/json"
	"runtime"
	"strings"
	"sync"

	"singbox-launcher/core/config/nodeflow"
	"singbox-launcher/internal/debuglog"
)

// CoreVersionProbe — версия ядра, под которое собирается config.json
// ("1.14.1-lx.4"). Ставится слоем приложения (core.AppController) тем же
// приёмом, что и пробы возможностей.
//
// nil-хук или пустая строка = «версия неизвестна»: гейт по версии не
// применяется вовсе. Деградировать по догадке нельзя — это та же политика,
// что у соседних проб, и последним рубежом остаётся `sing-box check`.
var CoreVersionProbe func() string

// CoreBuildTagsProbe — теги сборки ядра и теги, которые в сборке есть, но
// возможности не дают (тег → причина), для узлового гейта сборки
// (nodeflow.NodeCoreRefusal). Ставится слоем приложения тем же приёмом.
//
// nil-хук или nil-теги = «теги неизвестны»: гейт по тегу не применяется.
var CoreBuildTagsProbe func() (tags []string, issues map[string]string)

// coreInfoForBuild — ядро и ОС текущей сборки.
func coreInfoForBuild() nodeflow.CoreInfo {
	version := ""
	if CoreVersionProbe != nil {
		version = strings.TrimSpace(CoreVersionProbe())
	}
	return nodeflow.CoreInfo{Version: version, GOOS: runtime.GOOS}
}

// coreCapabilitiesForBuild — coreInfoForBuild плюс теги сборки ядра: для
// узлового гейта, один раз на прогон сборки (полевому гейту теги не нужны,
// и спрашивать их на каждый узел незачем).
func coreCapabilitiesForBuild() nodeflow.CoreInfo {
	info := coreInfoForBuild()
	if CoreBuildTagsProbe != nil {
		info.Tags, info.TagIssues = CoreBuildTagsProbe()
	}
	return info
}

// gateLoggedOnce снимает повтор WARN-строки: одна подписка приносит сотни
// узлов одного провайдера, и снятое у всех поле дало бы сотни одинаковых
// строк в логе сборки. Ключ — пара (тег поля, версия ядра), не тег узла:
// человеку важно, ЧТО снято и почему, а не у скольких узлов.
var gateLoggedOnce sync.Map

// gateBodyForCore — тело узла, пропущенное через полевой гейт. Второе
// значение — снял ли гейт хоть что-нибудь: вызывающему это нужно, чтобы не
// переформатировать тело, которого гейт не касался.
//
// Ошибка гейта не отменяет узел: тело возвращается как есть. Реестр здесь
// последний рубеж перед `sing-box check`, а не первый, и потеря узла из-за
// сбоя разбора собственного тела была бы хуже, чем ключ, который ядро и так
// отвергнет с внятным сообщением.
func gateBodyForCore(scheme, tag string, body []byte) ([]byte, bool) {
	if len(body) == 0 {
		return body, false
	}
	core := coreInfoForBuild()
	out, dropped, err := nodeflow.GateForCore(scheme, body, core)
	if err != nil {
		debuglog.WarnLog("Build gate: узел %q (%s): тело не разобралось (%v) — пропускаем как есть", tag, scheme, err)
		return body, false
	}
	if len(dropped) == 0 {
		return body, false
	}
	for _, path := range dropped {
		key := scheme + "." + path + "@" + core.Version
		if _, seen := gateLoggedOnce.LoadOrStore(key, true); seen {
			continue
		}
		debuglog.WarnLog(
			"Build gate: %s.%s снято — ядро %s его не знает (min_core/platform реестра); узел остаётся, ограничен рантайм",
			scheme, path, coreVersionLabel(core.Version))
	}
	return out, true
}

func coreVersionLabel(v string) string {
	if v == "" {
		return "неизвестной версии"
	}
	return v
}

// repairBodyForBuild — правила-починки реестра (`requires … set`,
// `coerce_when`, контракт 1.1.61) поверх замороженного тела состояния: тело
// записано при материализации, и правило, появившееся позже (REALITY без
// uTLS, random под REALITY), иначе доехало бы до ядра только с обновлением
// подписки. Правок нет — тело возвращается байт-в-байт.
func repairBodyForBuild(scheme, tag string, body []byte) []byte {
	if len(body) == 0 {
		return body
	}
	var m map[string]interface{}
	if err := json.Unmarshal(body, &m); err != nil {
		return body
	}
	fixed, notes := nodeflow.Repairs(scheme, m)
	if len(notes) == 0 {
		return body
	}
	out, err := json.Marshal(fixed)
	if err != nil {
		return body
	}
	logBuildRepairs(scheme, tag, notes)
	return out
}

// logBuildRepairs — коды починок, сделанных на сборке: у замороженного тела
// кодов на узле нет до следующей материализации, поэтому код уходит в лог.
func logBuildRepairs(scheme, tag string, notes []nodeflow.Warning) {
	for _, w := range notes {
		debuglog.InfoLog("Build: node %q (%s): %s at %s", tag, scheme, w.Code, w.Path)
	}
}
