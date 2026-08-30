// File preview_node_ops.go — операции над ОДНИМ узлом контейнера из вкладки
// Preview окна источника (SPEC 116 этап 3, W5; требование П2, цель 3).
//
// # Почему здесь, а не в новом списке
//
// §O4 = вариант А: список узлов у контейнера один — тот, что уже есть на
// вкладке Preview. Там есть строка узла, чекбокс включённости, правый клик и
// главное — ИДЕНТИЧНОСТЬ узла (сырой тег, `identities[id]`). Второй список
// узлов гарантированно разъехался бы с первым (ловушка «пул и сборка — одни и
// те же вызовы»), поэтому меню расширяется на месте.
//
// # Почему операции применяются к модели СРАЗУ, а не буферизуются в scratch
//
// Окно источника правит value-snapshot (`cloneSource`) и пишет его в модель
// одной записью на Save. Move/Copy этот контракт держать НЕ МОГУТ: они
// затрагивают ДВА источника — исходный контейнер и целевую папку, — а scratch
// знает ровно один, свой. Буферизовать половину операции значило бы: узел уже
// лежит в чужой папке (её-то мы правим напрямую — она не в scratch'е), а из
// исходного контейнера ещё не ушёл; Cancel окна оставил бы два экземпляра
// узла с одним сырым тегом. Поэтому Move/Copy/Rename/Delete — немедленные
// мутации модели по образцу fetch'а (`ApplyFetchSnapshot`), с полным набором
// побочек `applySourceMutation`, а окно после них ОБЯЗАНО перечитать scratch
// из живой записи (reloadScratch), иначе следующий Save затрёт операцию
// снимком, снятым до неё.
//
// # Что делает каждая операция и чем зовётся
//
// Реестр переписи ссылок и предупреждения — существующие, своих не заводим:
//
//   - Move/Copy → business.MoveNodeToFolder / CopyNodeToFolder (W2), они же
//     переписывают NodeLink и возвращают имена задетых источников;
//   - смена финального тега → showStaleSelectionDialog (выбор в кэше ядра);
//   - сброшенные ссылки при переименовании → resetRefsAfterNodeRename +
//     showDetourRefsResetDialog;
//   - разыменование (Д5) → business.DereferenceNodeOrigin + ShowAutoHideInfo.
package tabs

import (
	"fmt"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"

	corestate "singbox-launcher/core/state"
	"singbox-launcher/internal/dialogs"
	"singbox-launcher/internal/locale"
	wizardbusiness "singbox-launcher/ui/configurator/business"
	wizardmodels "singbox-launcher/ui/configurator/models"
	wizardpresentation "singbox-launcher/ui/configurator/presentation"
)

// previewNodeOps — контекст операций над узлом для одного открытого окна
// источника.
//
// Собирается один раз при построении окна и передаётся в контекстное меню
// строки: меню знает, ЧТО можно сделать с узлом, но не знает, как устроена
// модель, — ровно как раньше оно знало только `*config.ParsedNode`.
type previewNodeOps struct {
	presenter *wizardpresentation.WizardPresenter
	guiState  *wizardpresentation.GUIState
	// win — окно источника: владелец диалогов операции. Диалог живёт ровно
	// столько, сколько окно, и закрытие окна его уносит.
	win fyne.Window
	// sourceIndex — позиция контейнера в m.Sources. Индекс, а не ULID:
	// вызывающая сторона (окно) уже живёт индексом, и вторая адресация
	// разъехалась бы с первой на любой перестановке списка.
	sourceIndex int
	// kind — вид контейнера на момент открытия окна. От него зависит, какие
	// пункты меню вообще существуют: у подписки узлы несвободны
	// (features/sources.md §«Свобода и несвобода узлов»).
	kind corestate.SourceKind
	// reloadScratch — перечитать рабочую копию окна из живой записи модели.
	// Обязателен после КАЖДОЙ немедленной мутации (см. шапку файла).
	reloadScratch func()
	// refreshPreview — перерисовать список узлов вкладки.
	refreshPreview func()
}

// nodeOpsAllowed — состав этого контейнера правится командами меню.
//
// Только ПАПКА, и ровно по двум причинам, разным:
//
//   - подписка: её состав принадлежит провайдеру (features/sources.md
//     §«Свобода и несвобода узлов») — следующий fetch вернул бы удалённый узел
//     и переименовал переименованный; показывать управление, которое отменит
//     первое же обновление, хуже, чем не показывать его вовсе;
//   - server/chain/auto: у такого источника нет состава — его единственный
//     узел и есть сам Source, он переименовывается полем формы и удаляется
//     кнопкой строки в списке Sources. Второй путь к тем же двум действиям
//     разъехался бы с первым (у формы есть сброс ссылок на Save, у меню его
//     не было бы).
//
// «Copy to folder…» при этом есть ВЕЗДЕ: копия ничего в источнике не меняет —
// это ровно требование П2 «забрать узел провайдера себе».
func (o *previewNodeOps) nodeOpsAllowed() bool {
	return o != nil && o.kind == corestate.SourceKindFolder
}

// reorderAllowed — порядок узлов этого контейнера принадлежит пользователю.
//
// Только папка: порядок узлов подписки задаёт тело провайдера (merge кладёт
// свежие узлы в порядке тела — features/sources.md §«Merge-заливка»), и
// перестановка потерялась бы на первом же обновлении. Показать захват там
// значило бы пообещать порядок, который мы не удержим.
// Совпадает с nodeOpsAllowed по условию, но не по причине: там речь о правке
// состава, здесь — о порядке. Разъединены намеренно — если у какого-то вида
// контейнера появится одно без другого, поменяется одна функция, а не обе.
func (o *previewNodeOps) reorderAllowed() bool {
	return o != nil && o.kind == corestate.SourceKindFolder
}

// applyReorder переставляет узел папки с позиции from на позицию to (П5).
//
// Позиции приходят от списка UI, а узлы адресуются СЫРЫМИ тегами (identities):
// сопоставление «слот → узел» делается по тегу, а не по индексу в Nodes.
// Индексы совпадать не обязаны — список превью показывает эмитированные узлы,
// и выключенный/деградировавший узел мог в него не попасть.
//
// Перестановка не меняет ни одного тега, поэтому ни реестра переписи, ни
// предупреждения о протухшем выборе тут нет: адрес узла (контейнер, сырой тег)
// прежний, меняется только порядок эмиссии.
//
// Одна оговорка про {$num}: переменная тега нумерует узлы по порядку, и
// перестановка сдвинет номера — но это ровно то, чего пользователь и добивался
// перетаскиванием, и предупреждать о собственном действии незачем.
func (o *previewNodeOps) applyReorder(identities []string, from, to int) {
	if from == to || from < 0 || to < 0 || from >= len(identities) || to >= len(identities) {
		return
	}
	m := o.presenter.Model()
	if m == nil || o.sourceIndex < 0 || o.sourceIndex >= len(m.Sources) {
		return
	}
	src := &m.Sources[o.sourceIndex]

	fromIdx := nodeIndexByTag(src.Nodes, identities[from])
	toIdx := nodeIndexByTag(src.Nodes, identities[to])
	if fromIdx < 0 || toIdx < 0 || fromIdx == toIdx {
		return
	}

	src.Nodes = moveNodeWithinSlice(src.Nodes, fromIdx, toIdx)

	applySourceMutation(o.presenter, o.guiState)
	o.afterModelMutation()
}

// moveNodeWithinSlice переставляет элемент from на позицию to.
//
// Вырезать и вставить, а не поменять местами: пользователь тащил строку СКВОЗЬ
// соседей, и swap переставил бы двоих вместо одного (строка «через одного»
// уехала бы туда, откуда тащили).
//
// СЕМАНТИКА `to` — та же, что у chainForm.moveHop (source_chain_tab.go:669),
// единственного другого потребителя DragReorderGroup со списком-моделью:
// индекс отсчитывается по слайсу УЖЕ БЕЗ вырезанного элемента, поправки на
// сдвиг тут нет. Расхождение двух реализаций одной перестановки было бы хуже
// любой из двух семантик по отдельности: захват один, а вести себя стал бы
// по-разному в цепочке и в папке.
//
// Отдельной функцией — арифметика вставки после выреза единственное, что здесь
// можно перепутать, и она обязана проверяться тестом без Fyne.
//
// go1.20-гард: без slices.Insert/Delete — ручная сборка нового слайса.
func moveNodeWithinSlice(nodes []corestate.Node, from, to int) []corestate.Node {
	if from == to || from < 0 || to < 0 || from >= len(nodes) || to >= len(nodes) {
		return nodes
	}
	node := nodes[from]
	rest := append(nodes[:from:from], nodes[from+1:]...)
	out := make([]corestate.Node, 0, len(rest)+1)
	out = append(out, rest[:to]...)
	out = append(out, node)
	out = append(out, rest[to:]...)
	return out
}

// nodeIndexByTag — позиция узла с этим сырым тегом; -1, если такого нет.
func nodeIndexByTag(nodes []corestate.Node, tag string) int {
	if tag == "" {
		return -1
	}
	for i := range nodes {
		if nodes[i].Tag == tag {
			return i
		}
	}
	return -1
}

// folderTargets — папки, доступные целью move/copy, кроме самой себя.
//
// Возвращает параллельные срезы (подписи для селекта, ULID'ы) — Fyne'овский
// widget.Select работает строками, а адресовать папку строкой имени нельзя:
// имена не уникальны, а ULID уникален по построению.
func (o *previewNodeOps) folderTargets() (labels []string, ids []string) {
	m := o.presenter.Model()
	if m == nil {
		return nil, nil
	}
	selfID := ""
	if o.sourceIndex >= 0 && o.sourceIndex < len(m.Sources) {
		selfID = strings.TrimSpace(m.Sources[o.sourceIndex].ID)
	}
	seen := map[string]int{}
	for i := range m.Sources {
		s := &m.Sources[i]
		if s.Kind != corestate.SourceKindFolder {
			continue
		}
		id := strings.TrimSpace(s.ID)
		if id == "" || id == selfID {
			continue
		}
		label := wizardbusiness.SourceDisplayName(*s)
		// Две папки с одинаковым именем — законная ситуация (имя папки ничем
		// не занято и ссылок на него нет). В селекте они обязаны различаться,
		// иначе пользователь выберет не ту: дописываем порядковый номер.
		seen[label]++
		if n := seen[label]; n > 1 {
			label = fmt.Sprintf("%s (%d)", label, n)
		}
		labels = append(labels, label)
		ids = append(ids, id)
	}
	return labels, ids
}

// showMoveOrCopyDialog — выбор целевой папки для move/copy.
//
// Один диалог на обе операции: они отличаются ровно вызовом бизнес-функции и
// заголовком, а разводить два почти одинаковых окна значило бы получить два
// расходящихся набора проверок.
//
// Ловушка Fyne (fyne-label-minwidth-trap): подпись обязана быть Wrapping —
// без него длинный тег узла в одну строку задаёт диалогу min-width.
func (o *previewNodeOps) showMoveOrCopyDialog(rawTag string, move bool) {
	labels, ids := o.folderTargets()
	if len(labels) == 0 {
		// Папок нет вовсе — операция бессмысленна, но молчать нельзя: иначе
		// пункт меню выглядит сломанным.
		dialog.ShowInformation(
			locale.T("No folders yet"),
			locale.T("Create a folder first: Sources → ⋮ → Add folder."),
			o.win)
		return
	}

	title := locale.T("Copy node to folder")
	if move {
		title = locale.T("Move node to folder")
	}

	body := widget.NewLabel(locale.Tf("Node %q — pick the destination folder:", rawTag))
	body.Wrapping = fyne.TextWrapWord

	sel := widget.NewSelect(labels, nil)
	sel.SetSelectedIndex(0)

	content := container.NewVBox(body, sel)

	d := dialog.NewCustomConfirm(title, locale.T("OK"), locale.T("Cancel"), content,
		func(ok bool) {
			if !ok {
				return
			}
			idx := sel.SelectedIndex()
			if idx < 0 || idx >= len(ids) {
				return
			}
			o.applyMoveOrCopy(rawTag, ids[idx], move)
		}, o.win)
	d.Resize(fyne.NewSize(520, 220))
	d.Show()
}

// applyMoveOrCopy выполняет перенос/копирование и разгребает последствия.
//
// Порядок обязателен: сначала мутация модели (W2), потом побочки
// (applySourceMutation), потом перечитывание scratch'а окна, и только затем
// диалоги — они модальные, и показать их до перечитывания значило бы дать
// пользователю нажать Save по устаревшему снимку.
func (o *previewNodeOps) applyMoveOrCopy(rawTag, dstFolderID string, move bool) {
	m := o.presenter.Model()
	if m == nil {
		return
	}
	var (
		affected []string
		err      error
	)
	if move {
		affected, err = wizardbusiness.MoveNodeToFolder(m, o.sourceIndex, rawTag, dstFolderID)
	} else {
		affected, err = wizardbusiness.CopyNodeToFolder(m, o.sourceIndex, rawTag, dstFolderID)
	}
	if err != nil {
		dialog.ShowError(err, o.win)
		return
	}

	applySourceMutation(o.presenter, o.guiState)
	o.afterModelMutation()

	// Предупреждение о протухшем выборе — ТОЛЬКО у переноса: там финальный тег
	// узла сменился (у целевой папки своя тег-политика, у исходного контейнера
	// была своя), и ручной выбор в селекторах живого ядра адресован СТАРЫМ
	// тегом — переписать его лаунчер не может (SPEC 118 Т8). Копия же ничего
	// не переименовывает: оригинал на месте под прежним тегом, и пугать
	// пользователя сбросом выбора было бы враньём.
	if move {
		showStaleSelectionDialog(o.win, staleSelectionScope{NodesRenamed: true})
	}

	// Критерий A3: всякая ссылка, чья цель сменила финальный тег, либо
	// переписана реестром, либо НАЗВАНА. Реестр W2 вернул имена — показываем
	// их тем же окном, что и сброс ссылок при переименовании.
	if len(affected) > 0 {
		showDetourRefsResetDialog(o.win, rawTag, affected)
	}
}

// applyRename переименовывает узел контейнера и переписывает ссылки на него.
//
// Единственный вызывающий — кнопка Rename в окне узла
// (preview_node_edit_window.go). Своего диалога переименования у меню больше
// нет (обкатка заход 3): он вёл к тому же действию, что поле окна, и два пути
// к одному только путали. Сырой тег — ИДЕНТИЧНОСТЬ узла в рамках контейнера
// (SPEC 112): смена тега = появление другого узла, поэтому ссылки на прежний
// адрес переписываются реестром W2, а не гасятся вслепую.
func (o *previewNodeOps) applyRename(oldTag, newTag string) {
	if newTag == "" {
		dialog.ShowError(fmt.Errorf("%s", locale.T("Node tag cannot be empty.")), o.win)
		return
	}
	if newTag == oldTag {
		return
	}
	m := o.presenter.Model()
	if m == nil || o.sourceIndex < 0 || o.sourceIndex >= len(m.Sources) {
		return
	}
	src := &m.Sources[o.sourceIndex]
	var target *corestate.Node
	for i := range src.Nodes {
		if src.Nodes[i].Tag == newTag {
			dialog.ShowError(fmt.Errorf("%s", locale.Tf(
				"Tag %q is already taken in this container.", newTag)), o.win)
			return
		}
		if src.Nodes[i].Tag == oldTag {
			target = &src.Nodes[i]
		}
	}
	if target == nil {
		dialog.ShowError(fmt.Errorf("%s", locale.Tf("Node %q is gone.", oldTag)), o.win)
		return
	}

	// Д5, критерий A4: ручная правка тега — это «ручной чих» по копии узла
	// подписки, и она разыменовывает её НЕМЕДЛЕННО. Иначе следующая заливка
	// той же подписки нашла бы узел по прежнему сырому тегу... точнее, НЕ
	// нашла бы — и добавила рядом второй экземпляр, а переименованный сочла
	// исчезнувшим у провайдера.
	dereferenced := wizardbusiness.DereferenceNodeOrigin(target)

	folderID := strings.TrimSpace(src.ID)
	target.Tag = newTag
	affected := wizardbusiness.RepointContainerNodeLinks(m, folderID, oldTag, newTag)

	applySourceMutation(o.presenter, o.guiState)
	o.afterModelMutation()

	if dereferenced {
		o.notifyDereferenced(newTag)
	}
	showStaleSelectionDialog(o.win, staleSelectionScope{NodesRenamed: true})
	if len(affected) > 0 {
		showDetourRefsResetDialog(o.win, oldTag, affected)
	}
}

// showDeleteDialog — удаление узла из контейнера, с подтверждением.
//
// Подтверждение обязательно: узел, добавленный руками, восстановить неоткуда
// (у папки нет URL и нет «обновить»), а узел, залитый из подписки, вернётся
// только повторной заливкой.
func (o *previewNodeOps) showDeleteDialog(rawTag string) {
	dialog.ShowConfirm(
		locale.T("Delete node"),
		locale.Tf("Delete node %q from this folder?", rawTag),
		func(ok bool) {
			if ok {
				o.applyDelete(rawTag)
			}
		}, o.win)
}

// applyDelete убирает узел из контейнера.
//
// Ссылки на него НЕ переписываются — переписывать не на что: узла больше нет.
// Они гаснут штатным fail-closed резолвом сборки, но узнать об этом
// пользователь обязан здесь, а не из отчёта следующей сборки, — поэтому
// задетые источники называются тем же диалогом, что и при переименовании.
func (o *previewNodeOps) applyDelete(rawTag string) {
	m := o.presenter.Model()
	if m == nil || o.sourceIndex < 0 || o.sourceIndex >= len(m.Sources) {
		return
	}
	src := &m.Sources[o.sourceIndex]
	folderID := strings.TrimSpace(src.ID)
	found := -1
	for i := range src.Nodes {
		if src.Nodes[i].Tag == rawTag {
			found = i
			break
		}
	}
	if found < 0 {
		return
	}
	src.Nodes = append(src.Nodes[:found], src.Nodes[found+1:]...)
	// Тег "" целью переписи означает «цели больше нет»: реестр W2 гасит
	// ссылку, а не уводит её на чужой узел.
	affected := wizardbusiness.ClearContainerNodeLinks(m, folderID, rawTag)

	applySourceMutation(o.presenter, o.guiState)
	o.afterModelMutation()

	if len(affected) > 0 {
		showDetourRefsResetDialog(o.win, rawTag, affected)
	}
}

// afterModelMutation — общий хвост немедленной операции: окно перечитывает
// рабочую копию из живой записи и перерисовывает список узлов.
//
// Без перечитывания следующий Save окна записал бы снимок, снятый ДО
// операции, и молча откатил её (см. шапку файла).
func (o *previewNodeOps) afterModelMutation() {
	if o.reloadScratch != nil {
		o.reloadScratch()
	}
	if o.refreshPreview != nil {
		o.refreshPreview()
	}
}

// notifyDereferenced — уведомление об авторазыменовании (Д5, критерий A4).
func (o *previewNodeOps) notifyDereferenced(rawTag string) {
	notifyNodeDereferenced(o.win, rawTag)
}

// notifyNodeDereferenced — единственный текст об авторазыменовании на все его
// поводы (правка тега, правка тела, Regen).
//
// Существующий ShowAutoHideInfo, а не модальный диалог: разыменование —
// следствие действия, которое пользователь только что сделал сам, и
// останавливать его подтверждением незачем; сказать о последствии — нужно.
func notifyNodeDereferenced(win fyne.Window, rawTag string) {
	if win == nil {
		return
	}
	dialogs.ShowAutoHideInfo(fyne.CurrentApp(), win,
		locale.T("Node unlinked from its subscription"),
		locale.Tf("Node %q was edited, so it no longer follows the subscription: the next fill will neither overwrite nor remove it.", rawTag))
}

// dereferenceEditedSourceNode — авторазыменование при правке ТЕЛА верхнего
// узла-источника (Apply JSON / Regen from raw на вкладке JSON окна).
//
// Второй повод разыменования из тех же Д5/A4: правка body или пересборка из
// raw — такой же «ручной чих», как правка тега. Верхний узел живёт в scratch'е
// окна, поэтому здесь мутируется рабочая копия, а не модель: снимок доедет до
// модели штатным Save, и разыменование доедет вместе с ним.
//
// Возвращает true, если связь была снята — вызывающий показывает уведомление
// ровно один раз и только по делу.
func dereferenceEditedSourceNode(src *wizardmodels.Source) bool {
	if src == nil {
		return false
	}
	return wizardbusiness.DereferenceNodeOrigin(&src.Node)
}
