package linkmap

// Раннер-страж обратного хода (SPEC 133).
//
// Два судьи, и оба нужны:
//
//  1. TestEngineEmitVsSnapshot — ссылка движка = снимок РУКОПИСНОГО эмита
//     байт в байт по всем схемам, кроме поимённого списка разрешённых
//     отличий. Список судится В ОБЕ СТОРОНЫ: кейс, объявленный отличием,
//     обязан ОТЛИЧАТЬСЯ. Односторонняя пометка протухает и с этого момента
//     прикрывает регрессию (MAPPER_ENGINE.md «Рекомендации» §6).
//
//  2. TestEngineEmitRoundTrip — круг `parse(emit(body)) == body` на всех
//     фикстурах корпуса. Снимок сторожит ВИД ссылки (чужие клиенты), круг —
//     её СОДЕРЖАНИЕ (наш разбор). Зелёный снимок при красном круге означает,
//     что рукописный эмиттер терял поле и движок повторил потерю.
//
// Запуск:
//
//	go test ./core/config/linkmap -run TestEngineEmitVsSnapshot
//	go test ./core/config/linkmap -run TestEngineEmitRoundTrip

import (
	"encoding/json"
	"os"
	"sort"
	"strings"
	"testing"

	"singbox-launcher/core/config/nodeflow"
	"singbox-launcher/core/config/registry"
)

const (
	emitSnapshotPath = "testdata/emit_snapshot.json"
	emitDeltasPath   = "testdata/emit_deltas_allowed.json"
	// Отличия от КРУГА живут отдельным списком: «ссылка выглядит иначе» и
	// «круг вернул другое тело» — разные факты об одном кейсе, и кейс бывает
	// ровно одним из них (у vmess ссылка совпадает БАЙТ В БАЙТ, а круг
	// добавляет заголовок). Один список судил бы их вместе и требовал бы от
	// такого кейса отличаться там, где он совпадает.
	emitRTDeltasPath = "testdata/emit_round_trip_deltas_allowed.json"
)

type emitSnapshotEntry struct {
	Body  map[string]interface{} `json:"body"`
	Label string                 `json:"label,omitempty"`
	URI   string                 `json:"uri,omitempty"`
	Err   string                 `json:"err,omitempty"`
}

func loadEmitSnapshot(t *testing.T) map[string]emitSnapshotEntry {
	t.Helper()
	data, err := os.ReadFile(emitSnapshotPath)
	if err != nil {
		t.Skipf("снимка рукописного эмита нет (%v): снять `go test ./core/config/subscription -run TestShareURISnapshot -update`", err)
	}
	var out map[string]emitSnapshotEntry
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatalf("%s: %v", emitSnapshotPath, err)
	}
	return out
}

// loadEmitDeltas — поимённый список разрешённых отличий с ПРИЧИНОЙ. Парный к
// DELTAS.md.
func loadEmitDeltas(t *testing.T) map[string]string {
	return loadDeltaFile(t, emitDeltasPath)
}

// loadEmitRTDeltas — тот же список для круга.
func loadEmitRTDeltas(t *testing.T) map[string]string {
	return loadDeltaFile(t, emitRTDeltasPath)
}

func loadDeltaFile(t *testing.T, path string) map[string]string {
	t.Helper()
	out := map[string]string{}
	data, err := os.ReadFile(path)
	if err != nil {
		return out
	}
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatalf("%s: %v", path, err)
	}
	return out
}

// emitHarness — общий для обоих раннеров набор: планы и реестр.
type emitHarness struct {
	plans *PlanSet
	reg   *registry.Registry
	set   *registry.MapperSet
}

func newEmitHarness(t *testing.T) *emitHarness {
	t.Helper()
	set, err := registry.LoadMappers()
	if err != nil {
		t.Fatalf("LoadMappers: %v", err)
	}
	reg, err := registry.Get()
	if err != nil {
		t.Fatalf("registry.Get: %v", err)
	}
	plans, err := BuildPlans(set, reg.Order)
	if err != nil {
		t.Fatalf("BuildPlans: %v", err)
	}
	return &emitHarness{plans: plans, reg: reg, set: set}
}

// planForDir — план секции uri по имени каталога корпуса.
//
// Каталог назван по ФАЙЛУ протокола, а не по схеме (у shadowsocks схема
// зовётся `ss`), поэтому соответствие берётся из реестра, а не из таблицы в
// тесте.
func (h *emitHarness) planForDir(dir string) (string, *Plan, bool) {
	for _, scheme := range h.set.Schemes() {
		if protocolFileFor(scheme) != dir {
			continue
		}
		if plan, ok := h.plans.Plan(scheme, "uri"); ok {
			return scheme, plan, true
		}
	}
	return "", nil, false
}

// emitOne прогоняет тело через движок обратного хода.
func (h *emitHarness) emitOne(dir string, body map[string]interface{}, label string) (string, error) {
	scheme, plan, ok := h.planForDir(dir)
	if !ok {
		return "", ErrEmitNoSection
	}
	clean := map[string]interface{}{}
	for k, v := range body {
		if k == "tag" || k == "type" {
			continue
		}
		clean[k] = v
	}
	return Emit(plan, EmitInput{
		Body:     clean,
		Label:    label,
		BodyType: h.reg.SingboxType(scheme),
	})
}

func TestEngineEmitVsSnapshot(t *testing.T) {
	snap := loadEmitSnapshot(t)
	deltas := loadEmitDeltas(t)
	h := newEmitHarness(t)

	names := make([]string, 0, len(snap))
	for n := range snap {
		names = append(names, n)
	}
	sort.Strings(names)

	used := map[string]bool{}
	for _, name := range names {
		want := snap[name]
		dir := name[:strings.Index(name, "/")]
		t.Run(name, func(t *testing.T) {
			got, err := h.emitOne(dir, want.Body, want.Label)
			reason, allowed := deltas[name]
			if allowed {
				used[name] = true
			}
			if want.Err != "" {
				// Рукописный эмиттер отказал. Движок вправе отказать тоже
				// либо, будучи объявленным отличием, собрать ссылку.
				if err != nil {
					return
				}
				if allowed {
					t.Logf("разрешённое отличие: %s\n движок собрал: %s", reason, got)
					return
				}
				t.Errorf("рукописный эмиттер отказал (%s), движок собрал %q", want.Err, got)
				return
			}
			if err != nil {
				if allowed {
					t.Logf("разрешённое отличие: %s\n движок отказал: %v", reason, err)
					return
				}
				t.Errorf("движок отказал: %v\nснимок: %s", err, want.URI)
				return
			}
			if got == want.URI {
				if allowed {
					// Двусторонняя пометка: объявленное отличие ОБЯЗАНО
					// отличаться, иначе оно протухло и прикрывает регрессию.
					t.Errorf("кейс объявлен отличием (%s), но ссылки совпали — снять запись из %s",
						reason, emitDeltasPath)
				}
				return
			}
			if allowed {
				t.Logf("разрешённое отличие: %s\n got: %s\nwant: %s", reason, got, want.URI)
				return
			}
			t.Errorf("ссылка поехала\n got: %s\nwant: %s", got, want.URI)
		})
	}
	for name := range deltas {
		if !used[name] {
			t.Errorf("%s: кейса %q в снимке нет — запись протухла", emitDeltasPath, name)
		}
	}
}

// TestEngineEmitRoundTrip — круг parse(emit(body)) == body.
//
// Судит СОДЕРЖАНИЕ ссылки: ушедшее в неё обязано вернуться тем же телом.
// Отличия судятся СВОИМ списком (`emit_round_trip_deltas_allowed.json`): кейс
// бывает ровно одним из двух — ссылка совпала, а круг разошёлся, или
// наоборот, — и общий список требовал бы от такого кейса отличаться там, где
// он совпадает. Список судится в обе стороны так же, как список снимка.
func TestEngineEmitRoundTrip(t *testing.T) {
	snap := loadEmitSnapshot(t)
	deltas := loadEmitRTDeltas(t)
	h := newEmitHarness(t)

	names := make([]string, 0, len(snap))
	for n := range snap {
		names = append(names, n)
	}
	sort.Strings(names)

	used := map[string]bool{}
	for _, name := range names {
		entry := snap[name]
		dir := name[:strings.Index(name, "/")]
		t.Run(name, func(t *testing.T) {
			uri, err := h.emitOne(dir, entry.Body, entry.Label)
			if err != nil {
				t.Skipf("обратного хода нет: %v", err)
			}
			scheme, plan, ok := selectPlanFor(h.plans, uri)
			if !ok {
				t.Fatalf("собранную ссылку не опознала ни одна секция: %s", uri)
			}
			res, err := ParseURI(plan, uri, h.reg.SingboxType(scheme), nil)
			if err != nil {
				t.Fatalf("круг не разобрался: %v\nссылка: %s", err, uri)
			}
			sr := nodeflow.SanitizeFrom(scheme, plan.Mapper.BodySource, res.Body)
			if sr.Drop != nil {
				t.Fatalf("санитайзер отверг круг: %s\nссылка: %s", sr.Drop.Code, uri)
			}
			// Эталон проходит ТОТ ЖЕ санитайзер с тем же входом, что и круг:
			// снимок заморожен до правил реестра, которые приводят значение
			// (`coerce_when`), и сырое тело сравнивалось бы с уже приведённым.
			// Потерю поля это не прячет: санитайзер годное поле не убирает, и
			// выпавшее из ссылки останется только в эталоне.
			wr := nodeflow.SanitizeFrom(scheme, plan.Mapper.BodySource, entry.Body)
			if wr.Drop != nil {
				t.Fatalf("санитайзер отверг эталон: %s", wr.Drop.Code)
			}
			want := map[string]interface{}{}
			for k, v := range wr.Clean {
				if k == "tag" || k == "type" {
					continue
				}
				want[k] = v
			}
			order := h.reg.Order(scheme)
			g := canonString(sr.Clean, order)
			w := canonString(want, order)
			if g == w {
				return
			}
			if reason, ok := deltas[name]; ok {
				used[name] = true
				t.Logf("разрешённое отличие: %s\n got: %s\nwant: %s", reason, g, w)
				return
			}
			t.Errorf("круг потерял\n got: %s\nwant: %s\nссылка: %s", g, w, uri)
		})
	}
	for name := range deltas {
		if !used[name] {
			t.Errorf("%s: кейс %q на круге больше не расходится — запись протухла", emitRTDeltasPath, name)
		}
	}
}

// О СНИМКЕ, когда рукописного эмиттера больше нет.
//
// `testdata/emit_snapshot.json` снят ПРОГОНОМ рукописных эмиттеров
// (`shareuri_*.go`) до их удаления и с этого момента — ЗАМОРОЖЕННЫЙ ЭТАЛОН, а
// не то, что можно переснять. Генератор снимка удалён вместе с эмиттерами:
// пересъёмка теперь означала бы «запишем то, что движок и так выдаёт», то
// есть сверку с самим собой.
//
// Отсюда следствие для новых кейсов корпуса: тело, которого в снимке нет,
// раннер назовёт вслух (`в снимке нет кейса`). Это не поломка — это вопрос
// «а как эту ссылку писал прежний код?», и отвечать на него надо руками:
// либо кейс попадает в снимок вместе с ожидаемой ссылкой, либо в список
// отличий с причиной.
