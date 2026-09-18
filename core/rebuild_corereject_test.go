package core

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"singbox-launcher/core/state"
)

// File rebuild_corereject_test.go — ОДИН комплексный тест цикла страховки
// (SPEC 132 волна 4) на подставном `check`, таблицей сценариев.
//
// Проверяется РЕШЕНИЕ цикла, а не тексты и не вёрстка: какой узел выключен,
// заменён ли config.json, на каком круге цикл остановился. Именно это решает,
// не выключит ли страховка чужой узел и не оставит ли человека без связи.

// fakeCheck — сценарий ядра: на каждом круге отдаёт свой вердикт.
type fakeCheck struct {
	verdicts []string // "" = принято; иначе текст отказа
	calls    int
	// seen — содержимое кандидата на каждом круге: тест проверяет, что
	// проверялся именно кандидат, а не config.json.
	seen []string
}

func (f *fakeCheck) fn(_ string, configPath string) error {
	body, _ := os.ReadFile(configPath)
	f.seen = append(f.seen, string(body))
	i := f.calls
	f.calls++
	if i >= len(f.verdicts) || f.verdicts[i] == "" {
		return nil
	}
	return fmt.Errorf("%s", f.verdicts[i])
}

// memDisabler — «выключатель» поверх карты, без state.json: цикл о носителе
// не знает, и тесту хватает шва.
type memDisabler struct {
	nodes map[string]*state.Node // ключ — "folder/tag"
	order []string
}

func linkKey(l state.NodeLink) string { return l.FolderID + "/" + l.Tag }

func (d *memDisabler) Disable(link state.NodeLink, reason string) bool {
	n, ok := d.nodes[linkKey(link)]
	if !ok {
		return false
	}
	if !n.SetCoreRejected(reason) {
		return false
	}
	d.order = append(d.order, linkKey(link))
	return true
}

func (d *memDisabler) Commit() error               { return nil }
func (d *memDisabler) Label(state.NodeLink) string { return "sub" }

// rejectLine — строка отказа ядра нормативной формы (CANON §9.1).
func rejectLine(i int, tag, text string) string {
	return fmt.Sprintf("initialize outbound[%d] vless[%s]: %s", i, tag, text)
}

func TestCoreRejectLoopScenarios(t *testing.T) {
	// Конфиг из трёх узлов подписки F1.
	tags := []string{"A", "B", "C"}

	cases := []struct {
		name     string
		verdicts []string
		// decide: nil = фоновый вход; иначе ответ на вопрос предела.
		decide *bool

		wantPromoted bool
		wantDisabled []string
		wantStopped  bool
		wantRounds   int
	}{
		{
			name:         "чисто с первого раза — ни одного круга выключения",
			verdicts:     []string{""},
			wantPromoted: true,
			wantRounds:   1,
		},
		{
			name: "один негодный: выключен, второй круг чист, замена один раз",
			verdicts: []string{
				rejectLine(1, "B", "parse encryption: unknown appearance"),
				"",
			},
			wantPromoted: true,
			wantDisabled: []string{"F1/B"},
			wantRounds:   2,
		},
		{
			name: "два негодных подряд",
			verdicts: []string{
				rejectLine(0, "A", "bad uuid"),
				rejectLine(1, "C", "bad flow"),
				"",
			},
			wantPromoted: true,
			wantDisabled: []string{"F1/A", "F1/C"},
			wantRounds:   3,
		},
		{
			name:         "ошибка НЕ про узел — config.json не заменён, state не тронут",
			verdicts:     []string{"initialize inbound[0] tun: permission denied"},
			wantPromoted: false,
			wantRounds:   1,
		},
		{
			name:         "тег не сопоставлен — автоматики нет",
			verdicts:     []string{rejectLine(4, "Ghost", "bad uuid")},
			wantPromoted: false,
			wantRounds:   1,
		},
		{
			name:         "форма без тега (ядра до lx.7) — автоматики нет",
			verdicts:     []string{"initialize outbound[26]: unknown uTLS fingerprint"},
			wantPromoted: false,
			wantRounds:   1,
		},
		{
			name: "тот же тег назван повторно — цикл прерван (CANON §9.5)",
			verdicts: []string{
				rejectLine(1, "B", "bad flow"),
				rejectLine(1, "B", "bad flow"),
			},
			wantPromoted: false,
			wantDisabled: []string{"F1/B"},
			wantRounds:   2,
		},
		{
			name: "тег с `]: ` внутри — берётся максимально длинный кандидат",
			verdicts: []string{
				"initialize outbound[0] vless[A]: B]: c",
				"",
			},
			wantPromoted: true,
			wantDisabled: []string{"F1/A]: B"},
			wantRounds:   2,
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			configPath := filepath.Join(dir, "config.json")
			if err := os.WriteFile(configPath, []byte(`{"previous":true}`), 0o644); err != nil {
				t.Fatal(err)
			}

			all := append(append([]string(nil), tags...), "A]: B")
			dis := &memDisabler{nodes: map[string]*state.Node{}}
			links := map[string]state.NodeLink{}
			for _, tag := range all {
				dis.nodes["F1/"+tag] = &state.Node{
					Kind: state.SourceKindServer, Tag: tag, Enabled: true,
					Body: []byte(`{"type":"vless"}`),
				}
				links[tag] = state.NodeLink{FolderID: "F1", Tag: tag}
			}

			chk := &fakeCheck{verdicts: tc.verdicts}
			var decide coreRejectDecider
			if tc.decide != nil {
				want := *tc.decide
				decide = func(int) bool { return want }
			}
			loop := &coreRejectLoop{
				check: chk.fn, configPath: configPath, disabler: dis, decide: decide,
			}

			round := 0
			mk := func() buildRound {
				round++
				return buildRound{
					ConfigJSON: []byte(fmt.Sprintf(`{"round":%d}`, round)),
					NodeLinks:  links,
				}
			}
			out, err := loop.run(mk(), func() (buildRound, error) { return mk(), nil })
			if err != nil {
				t.Fatalf("цикл вернул ошибку: %v", err)
			}

			if out.Promoted != tc.wantPromoted {
				t.Errorf("Promoted = %v, ожидалось %v", out.Promoted, tc.wantPromoted)
			}
			if out.Rounds != tc.wantRounds {
				t.Errorf("Rounds = %d, ожидалось %d", out.Rounds, tc.wantRounds)
			}
			if got := strings.Join(dis.order, ","); got != strings.Join(tc.wantDisabled, ",") {
				t.Errorf("выключены %q, ожидалось %q", got, tc.wantDisabled)
			}

			// config.json: заменён принятым конфигом либо остался прежним.
			body, rerr := os.ReadFile(configPath)
			if rerr != nil {
				t.Fatalf("config.json пропал: %v", rerr)
			}
			if tc.wantPromoted {
				if string(body) == `{"previous":true}` {
					t.Error("config.json не заменён, хотя ядро конфиг приняло")
				}
			} else if string(body) != `{"previous":true}` {
				t.Errorf("config.json заменён, хотя ядро конфиг НЕ приняло: %s", body)
			}

			// Кандидат убран за собой при любом исходе.
			if _, serr := os.Stat(candidatePath(configPath)); !os.IsNotExist(serr) {
				t.Error("файл-кандидат остался на диске")
			}

			// Причина на выключенном узле — дословный текст ядра, без
			// префикса с тегом.
			for _, key := range dis.order {
				n := dis.nodes[key]
				if n.Enabled {
					t.Errorf("узел %s остался включённым", key)
				}
				reason := n.CoreRejectedReason()
				if reason == "" {
					t.Errorf("у узла %s нет причины", key)
				}
				if strings.Contains(reason, "initialize ") || strings.Contains(reason, "]: ") {
					t.Errorf("причина узла %s несёт префикс ядра: %q", key, reason)
				}
			}
		})
	}
}

// TestCoreRejectLoopLimitDialog — предел 10 кругов: Stop останавливает с
// выключенными, Keep checking доводит до конца.
func TestCoreRejectLoopLimitDialog(t *testing.T) {
	// 12 негодных узлов подряд, затем чисто.
	var tags []string
	var verdicts []string
	for i := 0; i < 12; i++ {
		tag := fmt.Sprintf("N%d", i)
		tags = append(tags, tag)
		verdicts = append(verdicts, rejectLine(i, tag, "bad uuid"))
	}
	verdicts = append(verdicts, "")

	run := func(keepChecking bool) (coreRejectOutcome, *memDisabler) {
		dir := t.TempDir()
		configPath := filepath.Join(dir, "config.json")
		if err := os.WriteFile(configPath, []byte(`{"previous":true}`), 0o644); err != nil {
			t.Fatal(err)
		}
		dis := &memDisabler{nodes: map[string]*state.Node{}}
		links := map[string]state.NodeLink{}
		for _, tag := range tags {
			dis.nodes["F1/"+tag] = &state.Node{
				Kind: state.SourceKindServer, Tag: tag, Enabled: true,
				Body: []byte(`{"type":"vless"}`),
			}
			links[tag] = state.NodeLink{FolderID: "F1", Tag: tag}
		}
		chk := &fakeCheck{verdicts: verdicts}
		asked := 0
		loop := &coreRejectLoop{
			check: chk.fn, configPath: configPath, disabler: dis,
			decide: func(disabled int) bool {
				asked++
				if disabled != coreRejectAskAfter {
					t.Errorf("спросили при %d выключенных, ожидалось %d", disabled, coreRejectAskAfter)
				}
				return keepChecking
			},
		}
		mk := func() buildRound {
			return buildRound{ConfigJSON: []byte(`{}`), NodeLinks: links}
		}
		out, err := loop.run(mk(), func() (buildRound, error) { return mk(), nil })
		if err != nil {
			t.Fatalf("цикл вернул ошибку: %v", err)
		}
		if asked != 1 {
			t.Errorf("вопрос задан %d раз(а), ожидался ровно один", asked)
		}
		return out, dis
	}

	t.Run("Stop — цикл встал, выключенные остаются выключенными", func(t *testing.T) {
		out, dis := run(false)
		if !out.StoppedByHuman {
			t.Error("StoppedByHuman = false")
		}
		if out.Promoted {
			t.Error("config.json заменён, хотя человек нажал Stop")
		}
		if len(out.Disabled) != coreRejectAskAfter {
			t.Errorf("выключено %d, ожидалось %d", len(out.Disabled), coreRejectAskAfter)
		}
		for _, key := range dis.order {
			if dis.nodes[key].Enabled {
				t.Errorf("узел %s вернулся во включённые после Stop", key)
			}
		}
	})

	t.Run("Keep checking — предел снят, цикл доходит до чистого прохода", func(t *testing.T) {
		out, _ := run(true)
		if out.StoppedByHuman {
			t.Error("StoppedByHuman = true, хотя человек выбрал Keep checking")
		}
		if !out.Promoted {
			t.Error("config.json не заменён, хотя цикл дошёл до чистого прохода")
		}
		if len(out.Disabled) != 12 {
			t.Errorf("выключено %d, ожидалось 12", len(out.Disabled))
		}
	})
}

// TestCoreRejectLoopBackgroundHardCap — фоновый вход (decide == nil) вопросов
// не задаёт и идёт до жёсткого потолка, а не бесконечно.
func TestCoreRejectLoopBackgroundHardCap(t *testing.T) {
	total := coreRejectHardCap + 5
	var tags []string
	var verdicts []string
	for i := 0; i < total; i++ {
		tag := fmt.Sprintf("N%d", i)
		tags = append(tags, tag)
		verdicts = append(verdicts, rejectLine(i, tag, "bad uuid"))
	}

	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.json")
	if err := os.WriteFile(configPath, []byte(`{"previous":true}`), 0o644); err != nil {
		t.Fatal(err)
	}
	dis := &memDisabler{nodes: map[string]*state.Node{}}
	links := map[string]state.NodeLink{}
	for _, tag := range tags {
		dis.nodes["F1/"+tag] = &state.Node{
			Kind: state.SourceKindServer, Tag: tag, Enabled: true,
			Body: []byte(`{"type":"vless"}`),
		}
		links[tag] = state.NodeLink{FolderID: "F1", Tag: tag}
	}
	chk := &fakeCheck{verdicts: verdicts}
	loop := &coreRejectLoop{check: chk.fn, configPath: configPath, disabler: dis}
	mk := func() buildRound { return buildRound{ConfigJSON: []byte(`{}`), NodeLinks: links} }

	out, err := loop.run(mk(), func() (buildRound, error) { return mk(), nil })
	if err != nil {
		t.Fatalf("цикл вернул ошибку: %v", err)
	}
	if out.StoppedByHuman {
		t.Error("фоновый вход не должен объявлять «остановил человек»")
	}
	if out.Promoted {
		t.Error("config.json заменён, хотя чистого прохода не было")
	}
	if len(out.Disabled) != coreRejectHardCap {
		t.Errorf("выключено %d, ожидался жёсткий потолок %d", len(out.Disabled), coreRejectHardCap)
	}
}

// TestCoreRejectLoopChecksCandidateNotConfig — проверяется КАНДИДАТ, а не
// config.json: инвариант «в config.json попадает только принятый конфиг»
// держится именно на этом.
func TestCoreRejectLoopChecksCandidate(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.json")
	if err := os.WriteFile(configPath, []byte(`{"previous":true}`), 0o644); err != nil {
		t.Fatal(err)
	}
	chk := &fakeCheck{verdicts: []string{""}}
	loop := &coreRejectLoop{check: chk.fn, configPath: configPath, disabler: &memDisabler{}}
	fresh := []byte(`{"fresh":true}`)
	out, err := loop.run(buildRound{ConfigJSON: fresh}, nil)
	if err != nil {
		t.Fatalf("цикл вернул ошибку: %v", err)
	}
	if !out.Promoted {
		t.Fatal("конфиг не заменён")
	}
	if len(chk.seen) != 1 || chk.seen[0] != string(fresh) {
		t.Errorf("проверялся не кандидат: %q", chk.seen)
	}
	body, _ := os.ReadFile(configPath)
	if string(body) != string(fresh) {
		t.Errorf("config.json = %q, ожидался свежий конфиг", body)
	}
}

// TestSavedStateDisablerAddressesNodes — адресация «ссылка → узел состояния»:
// узел контейнера, корневой узел (адресуется своим именем в конфиге),
// несуществующая ссылка.
func TestSavedStateDisablerAddressesNodes(t *testing.T) {
	s := &state.State{Sources: []state.Source{
		{
			ID:    "F1",
			Node:  state.Node{Kind: state.SourceKindSubscription, Tag: "sub", Enabled: true},
			Nodes: []state.Node{{Kind: state.SourceKindServer, Tag: "A", Enabled: true, Body: []byte(`{"type":"vless"}`)}},
		},
		{
			ID:    "S1",
			Label: "My server",
			Node:  state.Node{Kind: state.SourceKindServer, Tag: "root-1", Enabled: true, Body: []byte(`{"type":"vless"}`)},
		},
	}}
	d := &savedStateDisabler{s: s, path: filepath.Join(t.TempDir(), "state.json")}

	if !d.Disable(state.NodeLink{FolderID: "F1", Tag: "A"}, "bad uuid") {
		t.Fatal("узел контейнера не выключен")
	}
	if s.Sources[0].Nodes[0].Enabled {
		t.Error("узел контейнера остался включённым")
	}
	if got := s.Sources[0].Nodes[0].CoreRejectedReason(); got != "bad uuid" {
		t.Errorf("причина = %q", got)
	}
	if !d.Disable(state.NodeLink{FolderID: "", Tag: "root-1"}, "bad flow") {
		t.Fatal("корневой узел не выключен")
	}
	if s.Sources[1].Node.Enabled {
		t.Error("корневой узел остался включённым")
	}
	if d.Disable(state.NodeLink{FolderID: "F9", Tag: "nope"}, "x") {
		t.Error("выключен узел по ссылке в никуда")
	}
	// Повторное выключение той же причиной — «ничего не изменилось»: именно
	// по этому ответу цикл останавливается.
	if d.Disable(state.NodeLink{FolderID: "F1", Tag: "A"}, "bad uuid") {
		t.Error("повторное выключение той же причиной объявлено изменением")
	}
}
